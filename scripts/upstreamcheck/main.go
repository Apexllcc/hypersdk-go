// Command upstreamcheck verifies the checked-in Hyperliquid upstream contract lock.
//
// Without -network it is deterministic and offline: it validates only the lock
// file's schema and internal digests. With -network it performs read-only checks
// against the official GitBook pages and the official Python SDK, and exits
// non-zero on any upstream drift. It never modifies the lock file.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	defaultLockPath = "upstream.lock.json"
	sha256Prefix    = "sha256:"
)

type lockFile struct {
	Version   int            `json:"version"`
	Documents []documentLock `json:"documents"`
	PythonSDK pythonSDKLock  `json:"python_sdk"`
}

type documentLock struct {
	Name            string         `json:"name"`
	URL             string         `json:"url"`
	Revision        string         `json:"revision"`
	SHA256          string         `json:"sha256"`
	Summary         string         `json:"summary"`
	RequiredMarkers []string       `json:"required_markers"`
	Semantics       []semanticLock `json:"semantics,omitempty"`
}

type semanticLock struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

type pythonSDKLock struct {
	Repository string           `json:"repository"`
	HeadAPIURL string           `json:"head_api_url"`
	RawBaseURL string           `json:"raw_base_url"`
	Revision   string           `json:"revision"`
	Files      []pythonFileLock `json:"files"`
}

type pythonFileLock struct {
	Path            string   `json:"path"`
	SHA256          string   `json:"sha256"`
	RequiredMarkers []string `json:"required_markers"`
}

type dependencies struct {
	fetch      func(string) ([]byte, error)
	pythonHead func(string) (string, error)
}

func main() {
	lockPath := flag.String("lock", defaultLockPath, "path to the upstream lock JSON")
	network := flag.Bool("network", false, "perform read-only checks against official upstreams")
	totalTimeout := flag.Duration("total-timeout", 30*time.Second, "total timeout for all -network checks")
	flag.Parse()

	lock, err := readLock(*lockPath)
	if err != nil {
		fatal(err)
	}
	if err := validateLock(lock); err != nil {
		fatal(fmt.Errorf("invalid lock %q: %w", *lockPath, err))
	}
	if !*network {
		fmt.Printf("UPSTREAM_LOCK_OK path=%s version=%d mode=offline\n", *lockPath, lock.Version)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), *totalTimeout)
	defer cancel()
	deps := newDependencies(ctx)
	report, err := checkNetwork(lock, deps)
	if report != "" {
		fmt.Print(report)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "UPSTREAM_DRIFT: %v\nReview the report, update code/tests if needed, then intentionally regenerate %s.\n", err, *lockPath)
		os.Exit(1)
	}
	fmt.Printf("UPSTREAM_NETWORK_OK python_revision=%s\n", lock.PythonSDK.Revision)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func readLock(path string) (lockFile, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return lockFile{}, err
	}
	var lock lockFile
	if err := json.Unmarshal(contents, &lock); err != nil {
		return lockFile{}, err
	}
	return lock, nil
}

func validateLock(lock lockFile) error {
	if lock.Version != 2 {
		return fmt.Errorf("unsupported version %d", lock.Version)
	}
	if len(lock.Documents) == 0 {
		return errors.New("documents must not be empty")
	}
	seenDocuments := map[string]struct{}{}
	for _, document := range lock.Documents {
		if document.Name == "" {
			return errors.New("document name must not be empty")
		}
		if _, duplicate := seenDocuments[document.Name]; duplicate {
			return fmt.Errorf("duplicate document %q", document.Name)
		}
		seenDocuments[document.Name] = struct{}{}
		required, knownDocument := requiredDocuments[document.Name]
		if !knownDocument {
			return fmt.Errorf("document %q is not part of the required upstream contract coverage", document.Name)
		}
		if document.URL != required.URL {
			return fmt.Errorf("document %q URL must be %q", document.Name, required.URL)
		}
		if err := validateGitBookDocumentURL(document.URL); err != nil {
			return fmt.Errorf("document %q URL: %w", document.Name, err)
		}
		if document.Revision != sha256Prefix+document.SHA256 || !validSHA256(document.SHA256) {
			return fmt.Errorf("document %q revision must equal sha256:<digest>", document.Name)
		}
		if strings.TrimSpace(document.Summary) == "" {
			return fmt.Errorf("document %q summary must not be empty", document.Name)
		}
		if err := validateMarkers(document.RequiredMarkers, "document "+document.Name); err != nil {
			return err
		}
		if err := validateSemantics(document.Semantics, "document "+document.Name); err != nil {
			return err
		}
		if err := validateDocumentSemanticBinding(document.Semantics, required.Semantic, document.Name); err != nil {
			return err
		}
	}
	var missingDocuments []string
	for name := range requiredDocuments {
		if _, found := seenDocuments[name]; !found {
			missingDocuments = append(missingDocuments, name)
		}
	}
	if len(missingDocuments) > 0 {
		sort.Strings(missingDocuments)
		return fmt.Errorf("missing required document coverage: %s", strings.Join(missingDocuments, ", "))
	}

	python := lock.PythonSDK
	if err := validateCanonicalHTTPSURL(python.Repository, "github.com", "/hyperliquid-dex/hyperliquid-python-sdk"); err != nil {
		return fmt.Errorf("python_sdk.repository: %w", err)
	}
	if err := validateCanonicalHTTPSURL(python.HeadAPIURL, "api.github.com", "/repos/hyperliquid-dex/hyperliquid-python-sdk/commits/HEAD"); err != nil {
		return fmt.Errorf("python_sdk.head_api_url: %w", err)
	}
	if err := validateCanonicalHTTPSURL(python.RawBaseURL, "raw.githubusercontent.com", "/hyperliquid-dex/hyperliquid-python-sdk"); err != nil {
		return fmt.Errorf("python_sdk.raw_base_url: %w", err)
	}
	if !validGitRevision(python.Revision) {
		return fmt.Errorf("python_sdk revision must be a 40-character lowercase git revision")
	}
	if len(python.Files) == 0 {
		return errors.New("python_sdk files must not be empty")
	}
	seenFiles := map[string]struct{}{}
	for _, file := range python.Files {
		if !validPythonFilePath(file.Path) {
			return fmt.Errorf("invalid python_sdk file path %q", file.Path)
		}
		if _, duplicate := seenFiles[file.Path]; duplicate {
			return fmt.Errorf("duplicate python_sdk file %q", file.Path)
		}
		seenFiles[file.Path] = struct{}{}
		if _, required := requiredPythonSDKFiles[file.Path]; !required {
			return fmt.Errorf("python_sdk file %q is not part of the required upstream contract coverage", file.Path)
		}
		if !validSHA256(file.SHA256) {
			return fmt.Errorf("python_sdk file %q has invalid sha256", file.Path)
		}
		if err := validateMarkers(file.RequiredMarkers, "python_sdk file "+file.Path); err != nil {
			return err
		}
	}
	var missingFiles []string
	for path := range requiredPythonSDKFiles {
		if _, found := seenFiles[path]; !found {
			missingFiles = append(missingFiles, path)
		}
	}
	if len(missingFiles) > 0 {
		sort.Strings(missingFiles)
		return fmt.Errorf("missing required python_sdk files: %s", strings.Join(missingFiles, ", "))
	}
	return nil
}

var allowedGitBookPaths = map[string]struct{}{
	"/hyperliquid-docs/for-developers/api":                             {},
	"/hyperliquid-docs/for-developers/api/signing":                     {},
	"/hyperliquid-docs/for-developers/api/exchange-endpoint":           {},
	"/hyperliquid-docs/for-developers/api/info-endpoint":               {},
	"/hyperliquid-docs/for-developers/api/websocket":                   {},
	"/hyperliquid-docs/for-developers/api/websocket/subscriptions":     {},
	"/hyperliquid-docs/for-developers/api/rate-limits-and-user-limits": {},
	"/hyperliquid-docs/trading/fees":                                   {},
	"/hyperliquid-docs/hypercore/staking":                              {},
	"/hyperliquid-docs/trading/account-abstraction-modes":              {},
	"/hyperliquid-docs/trading/portfolio-margin":                       {},
}

type requiredDocument struct {
	URL      string
	Semantic string
}

var requiredDocuments = map[string]requiredDocument{
	"official-api-index":                 {URL: "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api"},
	"official-signing":                   {URL: "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/signing", Semantic: semanticSigningMarkers},
	"official-exchange-endpoint":         {URL: "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/exchange-endpoint", Semantic: semanticExchangeActionTypes},
	"official-info-endpoint":             {URL: "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/info-endpoint", Semantic: semanticInfoRequestTypes},
	"official-websocket":                 {URL: "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/websocket"},
	"official-websocket-subscriptions":   {URL: "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/websocket/subscriptions", Semantic: semanticWebSocketSubscription},
	"official-rate-limits":               {URL: "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/rate-limits-and-user-limits"},
	"official-fees":                      {URL: "https://hyperliquid.gitbook.io/hyperliquid-docs/trading/fees"},
	"official-staking":                   {URL: "https://hyperliquid.gitbook.io/hyperliquid-docs/hypercore/staking"},
	"official-account-abstraction-modes": {URL: "https://hyperliquid.gitbook.io/hyperliquid-docs/trading/account-abstraction-modes"},
	"official-portfolio-margin":          {URL: "https://hyperliquid.gitbook.io/hyperliquid-docs/trading/portfolio-margin"},
}

var requiredPythonSDKFiles = map[string]struct{}{
	"hyperliquid/exchange.py":      {},
	"hyperliquid/info.py":          {},
	"hyperliquid/utils/signing.py": {},
	"hyperliquid/utils/types.py":   {},
}

func validateGitBookDocumentURL(rawURL string) error {
	parsed, err := parseCanonicalHTTPSURL(rawURL, "hyperliquid.gitbook.io")
	if err != nil {
		return err
	}
	if _, allowed := allowedGitBookPaths[parsed.Path]; !allowed {
		return fmt.Errorf("path %q is not an approved official GitBook document", parsed.Path)
	}
	return nil
}

func validateCanonicalHTTPSURL(rawURL, host, expectedPath string) error {
	parsed, err := parseCanonicalHTTPSURL(rawURL, host)
	if err != nil {
		return err
	}
	if parsed.Path != expectedPath {
		return fmt.Errorf("path must be %q", expectedPath)
	}
	return nil
}

func parseCanonicalHTTPSURL(rawURL, expectedHost string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	if parsed.Scheme != "https" || parsed.Hostname() != expectedHost || parsed.Port() != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawPath != "" {
		return nil, fmt.Errorf("must be a canonical https URL for %s without userinfo, port, query, fragment, or escaped path", expectedHost)
	}
	if parsed.Host != expectedHost || rawURL != "https://"+expectedHost+parsed.Path {
		return nil, fmt.Errorf("must use canonical host and path")
	}
	return parsed, nil
}

func validateMarkers(markers []string, scope string) error {
	seen := map[string]struct{}{}
	for _, marker := range markers {
		marker = strings.TrimSpace(marker)
		if marker == "" {
			return fmt.Errorf("%s contains an empty required marker", scope)
		}
		if _, duplicate := seen[marker]; duplicate {
			return fmt.Errorf("%s has duplicate required marker %q", scope, marker)
		}
		seen[marker] = struct{}{}
	}
	return nil
}

const (
	semanticInfoRequestTypes      = "info-request-types"
	semanticExchangeActionTypes   = "exchange-action-types"
	semanticWebSocketSubscription = "websocket-subscription-types"
	semanticSigningMarkers        = "signing-markers"
)

var knownSemanticNames = map[string]struct{}{
	semanticInfoRequestTypes:      {},
	semanticExchangeActionTypes:   {},
	semanticWebSocketSubscription: {},
	semanticSigningMarkers:        {},
}

var semanticValuePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

func validateSemantics(semantics []semanticLock, scope string) error {
	seen := map[string]struct{}{}
	for _, semantic := range semantics {
		if _, known := knownSemanticNames[semantic.Name]; !known {
			return fmt.Errorf("%s has unknown semantic snapshot %q", scope, semantic.Name)
		}
		if _, duplicate := seen[semantic.Name]; duplicate {
			return fmt.Errorf("%s has duplicate semantic snapshot %q", scope, semantic.Name)
		}
		seen[semantic.Name] = struct{}{}
		if len(semantic.Values) == 0 {
			return fmt.Errorf("%s semantic snapshot %q must not be empty", scope, semantic.Name)
		}
		for index, value := range semantic.Values {
			if !semanticValuePattern.MatchString(value) {
				return fmt.Errorf("%s semantic snapshot %q has invalid value %q", scope, semantic.Name, value)
			}
			if index > 0 && semantic.Values[index-1] >= value {
				return fmt.Errorf("%s semantic snapshot %q values must be strictly sorted", scope, semantic.Name)
			}
		}
	}
	return nil
}

func validateDocumentSemanticBinding(semantics []semanticLock, requiredName, documentName string) error {
	if requiredName == "" {
		if len(semantics) != 0 {
			return fmt.Errorf("document %q must not have a semantic snapshot", documentName)
		}
		return nil
	}
	if len(semantics) != 1 || semantics[0].Name != requiredName {
		return fmt.Errorf("document %q must have exactly the %q semantic snapshot", documentName, requiredName)
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

var gitRevisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var pythonPathPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

func validGitRevision(value string) bool { return gitRevisionPattern.MatchString(value) }

func validPythonFilePath(path string) bool {
	if !pythonPathPattern.MatchString(path) || strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func newDependencies(ctx context.Context) dependencies {
	client := &http.Client{}
	return dependencies{
		fetch: func(rawURL string) ([]byte, error) {
			return fetchURL(ctx, client, rawURL)
		},
		pythonHead: func(headAPIURL string) (string, error) {
			contents, err := fetchURL(ctx, client, headAPIURL)
			if err != nil {
				return "", err
			}
			var response struct {
				SHA string `json:"sha"`
			}
			if err := json.Unmarshal(contents, &response); err != nil {
				return "", fmt.Errorf("decode Python SDK HEAD response: %w", err)
			}
			if !validGitRevision(response.SHA) {
				return "", fmt.Errorf("python SDK HEAD response has invalid SHA %q", response.SHA)
			}
			return response.SHA, nil
		},
	}
}

func fetchURL(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	noRedirectClient := *client
	noRedirectClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "hyperliquid-go-sdk-upstreamcheck/1")
	response, err := noRedirectClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("GET %s: %s", rawURL, response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 20<<20))
}

func checkNetwork(lock lockFile, deps dependencies) (string, error) {
	var findings []string
	for _, document := range lock.Documents {
		contents, err := deps.fetch(document.URL)
		if err != nil {
			return "", fmt.Errorf("fetch official document %q: %w", document.Name, err)
		}
		if observed := digest(contents); observed != document.SHA256 {
			findings = append(findings, fmt.Sprintf("DOCUMENT_DRIFT name=%s\n  url=%s\n  locked_sha256=%s\n  observed_sha256=%s", document.Name, document.URL, document.SHA256, observed))
		}
		if missing := missingMarkers(contents, document.RequiredMarkers); len(missing) > 0 {
			findings = append(findings, fmt.Sprintf("DOCUMENT_STRUCTURE_DRIFT name=%s\n  missing required markers: %s", document.Name, strings.Join(missing, ", ")))
		}
		for _, semantic := range document.Semantics {
			if delta := documentSemanticDelta(semantic, contents); delta != "" {
				findings = append(findings, fmt.Sprintf("DOCUMENT_SEMANTIC_DRIFT name=%s\n  %s", document.Name, delta))
			}
		}
	}

	head, err := deps.pythonHead(lock.PythonSDK.HeadAPIURL)
	if err != nil {
		return formatFindings(findings), fmt.Errorf("read official Python SDK HEAD: %w", err)
	}
	if head != lock.PythonSDK.Revision {
		findings = append(findings, fmt.Sprintf("PYTHON_SDK_REVISION_DRIFT\n  repository=%s\n  locked_revision=%s\n  upstream_revision=%s", lock.PythonSDK.Repository, lock.PythonSDK.Revision, head))
	}
	for _, file := range lock.PythonSDK.Files {
		lockedContents, err := deps.fetch(rawPythonURL(lock.PythonSDK, lock.PythonSDK.Revision, file.Path))
		if err != nil {
			return formatFindings(findings), fmt.Errorf("fetch locked Python SDK file %q: %w", file.Path, err)
		}
		if observed := digest(lockedContents); observed != file.SHA256 {
			return formatFindings(findings), fmt.Errorf("locked Python SDK file %q digest mismatch: lock=%s observed=%s", file.Path, file.SHA256, observed)
		}
		if missing := missingMarkers(lockedContents, file.RequiredMarkers); len(missing) > 0 {
			return formatFindings(findings), fmt.Errorf("locked Python SDK file %q lacks required markers: %s", file.Path, strings.Join(missing, ", "))
		}
		if head == lock.PythonSDK.Revision {
			continue
		}
		upstreamContents, err := deps.fetch(rawPythonURL(lock.PythonSDK, head, file.Path))
		if err != nil {
			return formatFindings(findings), fmt.Errorf("fetch upstream Python SDK file %q: %w", file.Path, err)
		}
		if observed := digest(upstreamContents); observed != file.SHA256 {
			finding := fmt.Sprintf("PYTHON_SDK_FILE_DRIFT path=%s\n  locked_sha256=%s\n  upstream_sha256=%s", file.Path, file.SHA256, observed)
			if delta := actionTypeDelta(lockedContents, upstreamContents); delta != "" {
				finding += "\n  " + delta
			}
			if delta := pythonMethodDelta(lockedContents, upstreamContents); delta != "" {
				finding += "\n  " + delta
			}
			if missing := missingMarkers(upstreamContents, file.RequiredMarkers); len(missing) > 0 {
				finding += "\n  missing required markers: " + strings.Join(missing, ", ")
			}
			findings = append(findings, finding)
		}
	}
	if len(findings) == 0 {
		return "", nil
	}
	return formatFindings(findings), errors.New("official upstream changed; lock not updated automatically")
}

func formatFindings(findings []string) string {
	if len(findings) == 0 {
		return ""
	}
	return strings.Join(findings, "\n\n") + "\n"
}

func rawPythonURL(lock pythonSDKLock, revision, path string) string {
	return strings.TrimRight(lock.RawBaseURL, "/") + "/" + revision + "/" + path
}

func digest(contents []byte) string {
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}

func missingMarkers(contents []byte, required []string) []string {
	text := strings.ToLower(normalizeUpstreamText(contents))
	var missing []string
	for _, marker := range required {
		if !strings.Contains(text, strings.ToLower(marker)) {
			missing = append(missing, marker)
		}
	}
	sort.Strings(missing)
	return missing
}

var actionTypePattern = regexp.MustCompile(`["']type["']\s*:\s*["']([A-Za-z][A-Za-z0-9]*)["']`)
var infoRequestTypePattern = regexp.MustCompile(`(?i)(?:^|[\s|])type\s*\*?\s*string\s*["']?([A-Za-z][A-Za-z0-9]*)["']?`)
var pythonMethodPattern = regexp.MustCompile(`(?m)^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
var signingMarkerPattern = regexp.MustCompile(`\b(sign_[a-z][a-z0-9_]*)\b`)
var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

var nonActionTypeValues = map[string]struct{}{
	"bool":    {},
	"bytes":   {},
	"default": {},
	"string":  {},
	"uint64":  {},
}

func actionTypeDelta(locked, upstream []byte) string {
	before := actionTypes(locked)
	after := actionTypes(upstream)
	added := setDifference(after, before)
	removed := setDifference(before, after)
	var parts []string
	if len(added) > 0 {
		parts = append(parts, "action-types added: "+strings.Join(added, ", "))
	}
	if len(removed) > 0 {
		parts = append(parts, "action-types removed: "+strings.Join(removed, ", "))
	}
	return strings.Join(parts, "; ")
}

func actionTypes(contents []byte) map[string]struct{} {
	set := map[string]struct{}{}
	for _, match := range actionTypePattern.FindAllStringSubmatch(normalizeUpstreamText(contents), -1) {
		if _, scalar := nonActionTypeValues[match[1]]; scalar {
			continue
		}
		set[match[1]] = struct{}{}
	}
	return set
}

func documentSemanticDelta(semantic semanticLock, contents []byte) string {
	observed := semanticValues(semantic.Name, contents)
	if len(observed) == 0 {
		return "semantic extraction unavailable: " + semantic.Name
	}
	expected := sliceSet(semantic.Values)
	return formatSemanticDelta(semantic.Name, setDifference(observed, expected), setDifference(expected, observed))
}

func semanticValues(name string, contents []byte) map[string]struct{} {
	text := normalizeUpstreamText(contents)
	switch name {
	case semanticInfoRequestTypes:
		if values := infoRequestTypes(text); len(values) > 0 {
			return values
		}
		return actionTypes([]byte(text))
	case semanticExchangeActionTypes, semanticWebSocketSubscription:
		return actionTypes([]byte(text))
	case semanticSigningMarkers:
		return matchesToSet(signingMarkerPattern, text)
	default:
		return nil
	}
}

func infoRequestTypes(text string) map[string]struct{} {
	plain := strings.Join(strings.Fields(htmlTagPattern.ReplaceAllString(text, " ")), " ")
	return matchesToSet(infoRequestTypePattern, plain)
}

func pythonMethodDelta(locked, upstream []byte) string {
	before := matchesToSet(pythonMethodPattern, string(locked))
	after := matchesToSet(pythonMethodPattern, string(upstream))
	added := setDifference(after, before)
	removed := setDifference(before, after)
	var parts []string
	if len(added) > 0 {
		parts = append(parts, "python-methods added: "+strings.Join(added, ", "))
	}
	if len(removed) > 0 {
		parts = append(parts, "python-methods removed: "+strings.Join(removed, ", "))
	}
	return strings.Join(parts, "; ")
}

func formatSemanticDelta(name string, added, removed []string) string {
	label := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	var parts []string
	if len(added) > 0 {
		parts = append(parts, label+" added: "+strings.Join(added, ", "))
	}
	if len(removed) > 0 {
		parts = append(parts, label+" removed: "+strings.Join(removed, ", "))
	}
	return strings.Join(parts, "; ")
}

func matchesToSet(pattern *regexp.Regexp, text string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, match := range pattern.FindAllStringSubmatch(text, -1) {
		set[match[1]] = struct{}{}
	}
	return set
}

func sliceSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func normalizeUpstreamText(contents []byte) string {
	text := html.UnescapeString(string(contents))
	text = strings.ReplaceAll(text, `\\"`, `"`)
	text = strings.ReplaceAll(text, `\\n`, " ")
	return text
}

func setDifference(left, right map[string]struct{}) []string {
	var difference []string
	for value := range left {
		if _, exists := right[value]; !exists {
			difference = append(difference, value)
		}
	}
	sort.Strings(difference)
	return difference
}
