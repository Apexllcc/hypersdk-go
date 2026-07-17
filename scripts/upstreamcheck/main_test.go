package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRepositoryLockIsValidOffline(t *testing.T) {
	lock, err := readLock(filepath.Join("..", "..", defaultLockPath))
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	if err := validateLock(lock); err != nil {
		t.Fatalf("validateLock() error = %v", err)
	}
}

func TestValidateLockRejectsMalformedRevision(t *testing.T) {
	lock := testLock()
	lock.PythonSDK.Revision = "not-a-git-revision"

	err := validateLock(lock)
	if err == nil || !strings.Contains(err.Error(), "revision") {
		t.Fatalf("validateLock() error = %v, want malformed revision error", err)
	}
}

func TestValidateLockRejectsUntrustedOrNonCanonicalSources(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*lockFile)
	}{
		{"document host", func(lock *lockFile) {
			lock.Documents[0].URL = "https://example.com/hyperliquid-docs/for-developers/api"
		}},
		{"document query", func(lock *lockFile) { lock.Documents[0].URL += "?redirect=evil" }},
		{"document userinfo", func(lock *lockFile) {
			lock.Documents[0].URL = "https://trusted@hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api"
		}},
		{"document port", func(lock *lockFile) {
			lock.Documents[0].URL = "https://hyperliquid.gitbook.io:443/hyperliquid-docs/for-developers/api"
		}},
		{"document non canonical path", func(lock *lockFile) {
			lock.Documents[0].URL = "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/"
		}},
		{"python head endpoint", func(lock *lockFile) { lock.PythonSDK.HeadAPIURL += "?ref=main" }},
		{"python raw endpoint", func(lock *lockFile) {
			lock.PythonSDK.RawBaseURL = "https://raw.githubusercontent.com/hyperliquid-dex/other-sdk"
		}},
		{"python path query", func(lock *lockFile) { lock.PythonSDK.Files[0].Path = "hyperliquid/exchange.py?evil" }},
		{"python path backslash", func(lock *lockFile) { lock.PythonSDK.Files[0].Path = "hyperliquid\\exchange.py" }},
		{"python path dot segment", func(lock *lockFile) { lock.PythonSDK.Files[0].Path = "hyperliquid/./exchange.py" }},
		{"python path empty segment", func(lock *lockFile) { lock.PythonSDK.Files[0].Path = "hyperliquid//exchange.py" }},
		{"python path trailing slash", func(lock *lockFile) { lock.PythonSDK.Files[0].Path = "hyperliquid/exchange.py/" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lock := testLock()
			test.mutate(&lock)
			if err := validateLock(lock); err == nil {
				t.Fatal("validateLock() error = nil, want source rejection")
			}
		})
	}
}

func TestValidateLockRejectsIncompleteRequiredDocumentCoverage(t *testing.T) {
	lock := testLock()
	lock.Documents = lock.Documents[1:]

	err := validateLock(lock)
	if err == nil || !strings.Contains(err.Error(), "missing required document") {
		t.Fatalf("validateLock() error = %v, want required document coverage error", err)
	}
}

func TestValidateLockRequiresExactPythonSDKFileCoverage(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*lockFile)
		want   string
	}{
		{
			name: "missing required path",
			mutate: func(lock *lockFile) {
				lock.PythonSDK.Files = lock.PythonSDK.Files[:len(lock.PythonSDK.Files)-1]
			},
			want: "missing required python_sdk files: hyperliquid/utils/types.py",
		},
		{
			name: "duplicate required path",
			mutate: func(lock *lockFile) {
				lock.PythonSDK.Files = append(lock.PythonSDK.Files, lock.PythonSDK.Files[0])
			},
			want: "duplicate python_sdk file \"hyperliquid/exchange.py\"",
		},
		{
			name: "unknown path",
			mutate: func(lock *lockFile) {
				lock.PythonSDK.Files = append(lock.PythonSDK.Files, pythonFileLock{
					Path:   "hyperliquid/client.py",
					SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				})
			},
			want: "python_sdk file \"hyperliquid/client.py\" is not part of the required upstream contract coverage",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lock := testLock()
			test.mutate(&lock)

			err := validateLock(lock)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateLock() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateLockRejectsSemanticSnapshotOnWrongPage(t *testing.T) {
	lock := testLock()
	lock.Documents[0].Semantics = []semanticLock{{
		Name:   semanticSigningMarkers,
		Values: []string{"sign_l1_action"},
	}}

	err := validateLock(lock)
	if err == nil || !strings.Contains(err.Error(), "must not have a semantic snapshot") {
		t.Fatalf("validateLock() error = %v, want semantic binding error", err)
	}
}

func TestFetchRefusesRedirects(t *testing.T) {
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "https://example.com", http.StatusFound)
	}))
	defer redirect.Close()

	_, err := fetchURL(context.Background(), &http.Client{Timeout: time.Second}, redirect.URL)
	if err == nil || !strings.Contains(err.Error(), "302") {
		t.Fatalf("fetchURL() error = %v, want redirect status rejection", err)
	}
}

func TestNewDependenciesUsesTotalContextDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deps := newDependencies(ctx)
	_, err := deps.fetch("https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("fetch() error = %v, want context cancellation", err)
	}
}

func TestNetworkCheckReportsPythonActionDrift(t *testing.T) {
	lock := testLock()
	locked := []byte(`def action(): return {"type": "order"}`)
	upstream := []byte(`def action(): return {"type": "twapOrder"}`)
	lock.PythonSDK.Files[0].SHA256 = digest(locked)

	report, err := checkNetwork(lock, dependencies{
		pythonHead: func(string) (string, error) { return "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", nil },
		fetch: func(rawURL string) ([]byte, error) {
			if strings.Contains(rawURL, lock.PythonSDK.Revision) {
				return locked, nil
			}
			return upstream, nil
		},
	})
	if err == nil {
		t.Fatal("checkNetwork() error = nil, want drift error")
	}
	if !strings.Contains(report, "action-types added: twapOrder") || !strings.Contains(report, "action-types removed: order") {
		t.Fatalf("report = %q, want action type delta", report)
	}
}

func TestNetworkCheckReportsMissingDocumentMarker(t *testing.T) {
	lock := testLock()
	lock.Documents[0].SHA256 = digest([]byte("current official API with nonce"))
	lock.Documents[0].RequiredMarkers = []string{"nonce", "EIP-712"}

	report, err := checkNetwork(lock, dependencies{
		pythonHead: func(string) (string, error) { return lock.PythonSDK.Revision, nil },
		fetch:      func(string) ([]byte, error) { return []byte("current official API with nonce"), nil },
	})
	if err == nil {
		t.Fatal("checkNetwork() error = nil, want missing document marker error")
	}
	if !strings.Contains(report, "missing required markers: EIP-712") {
		t.Fatalf("report = %q, want missing marker", report)
	}
}

func TestNetworkCheckReportsChildPageSemanticAddedAndRemovedValues(t *testing.T) {
	lock := testLock()
	lock.Documents[0] = documentLock{
		Name:            "official-info-endpoint",
		URL:             "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/info-endpoint",
		Revision:        "sha256:" + digest([]byte(`{"type": "clearinghouseState"}`)),
		SHA256:          digest([]byte(`{"type": "clearinghouseState"}`)),
		Summary:         "Official Info request types.",
		RequiredMarkers: []string{"type"},
		Semantics: []semanticLock{{
			Name:   semanticInfoRequestTypes,
			Values: []string{"clearinghouseState"},
		}},
	}
	lock.PythonSDK.Files = nil

	report, err := checkNetwork(lock, dependencies{
		pythonHead: func(string) (string, error) { return lock.PythonSDK.Revision, nil },
		fetch:      func(string) ([]byte, error) { return []byte(`{"type": "userFunding"}`), nil },
	})
	if err == nil {
		t.Fatal("checkNetwork() error = nil, want drift error")
	}
	if !strings.Contains(report, "INFO_REQUEST_TYPES added: userFunding") || !strings.Contains(report, "INFO_REQUEST_TYPES removed: clearinghouseState") {
		t.Fatalf("report = %q, want deterministic Info request type delta", report)
	}
}

func TestNetworkCheckReportsPythonMethodAddedAndRemovedValues(t *testing.T) {
	lock := testLock()
	locked := []byte("def old_method():\n    pass\n")
	upstream := []byte("def new_method():\n    pass\n")
	lock.PythonSDK.Files[0].SHA256 = digest(locked)

	report, err := checkNetwork(lock, dependencies{
		pythonHead: func(string) (string, error) { return "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", nil },
		fetch: func(rawURL string) ([]byte, error) {
			if strings.Contains(rawURL, lock.PythonSDK.Revision) {
				return locked, nil
			}
			return upstream, nil
		},
	})
	if err == nil {
		t.Fatal("checkNetwork() error = nil, want drift error")
	}
	if !strings.Contains(report, "python-methods added: new_method") || !strings.Contains(report, "python-methods removed: old_method") {
		t.Fatalf("report = %q, want deterministic Python method delta", report)
	}
}

func TestSigningSemanticValuesExtractMarkers(t *testing.T) {
	values := semanticValues(semanticSigningMarkers, []byte("use sign_l1_action then sign_user_signed_action"))
	if _, ok := values["sign_l1_action"]; !ok {
		t.Fatalf("semanticValues() = %#v, want sign_l1_action", values)
	}
	if _, ok := values["sign_user_signed_action"]; !ok {
		t.Fatalf("semanticValues() = %#v, want sign_user_signed_action", values)
	}
}

func TestInfoSemanticValuesExtractUnquotedTableValues(t *testing.T) {
	values := semanticValues(semanticInfoRequestTypes, []byte("type * String userRateLimit"))
	if _, ok := values["userRateLimit"]; !ok {
		t.Fatalf("semanticValues() = %#v, want userRateLimit", values)
	}
}

func TestExchangeSemanticValuesIgnoreSchemaDefault(t *testing.T) {
	values := semanticValues(semanticExchangeActionTypes, []byte(`{"type": "default"}`))
	if _, ok := values["default"]; ok {
		t.Fatalf("semanticValues() = %#v, do not want schema default", values)
	}
}

func TestInfoSemanticValuesIgnoreContentTypeHeader(t *testing.T) {
	values := semanticValues(semanticInfoRequestTypes, []byte(`Content-Type * String "application/json"`))
	if _, ok := values["application"]; ok {
		t.Fatalf("semanticValues() = %#v, do not want header media type", values)
	}
}

func testLock() lockFile {
	return lockFile{
		Version: 2,
		Documents: []documentLock{
			testDocument("official-api-index", "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api", ""),
			testDocument("official-signing", "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/signing", semanticSigningMarkers),
			testDocument("official-exchange-endpoint", "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/exchange-endpoint", semanticExchangeActionTypes),
			testDocument("official-info-endpoint", "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/info-endpoint", semanticInfoRequestTypes),
			testDocument("official-websocket", "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/websocket", ""),
			testDocument("official-websocket-subscriptions", "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/websocket/subscriptions", semanticWebSocketSubscription),
			testDocument("official-rate-limits", "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/rate-limits-and-user-limits", ""),
			testDocument("official-fees", "https://hyperliquid.gitbook.io/hyperliquid-docs/trading/fees", ""),
			testDocument("official-staking", "https://hyperliquid.gitbook.io/hyperliquid-docs/hypercore/staking", ""),
			testDocument("official-account-abstraction-modes", "https://hyperliquid.gitbook.io/hyperliquid-docs/trading/account-abstraction-modes", ""),
			testDocument("official-portfolio-margin", "https://hyperliquid.gitbook.io/hyperliquid-docs/trading/portfolio-margin", ""),
		},
		PythonSDK: pythonSDKLock{
			Repository: "https://github.com/hyperliquid-dex/hyperliquid-python-sdk",
			HeadAPIURL: "https://api.github.com/repos/hyperliquid-dex/hyperliquid-python-sdk/commits/HEAD",
			RawBaseURL: "https://raw.githubusercontent.com/hyperliquid-dex/hyperliquid-python-sdk",
			Revision:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Files: []pythonFileLock{
				{Path: "hyperliquid/exchange.py", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				{Path: "hyperliquid/info.py", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				{Path: "hyperliquid/utils/signing.py", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				{Path: "hyperliquid/utils/types.py", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			},
		},
	}
}

func testDocument(name, rawURL, semantic string) documentLock {
	document := documentLock{
		Name:            name,
		URL:             rawURL,
		Revision:        "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SHA256:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Summary:         "Official upstream contract.",
		RequiredMarkers: []string{"marker"},
	}
	if semantic != "" {
		document.Semantics = []semanticLock{{Name: semantic, Values: []string{"value"}}}
	}
	return document
}
