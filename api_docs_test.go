package hyperliquid_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var apiMarkerRE = regexp.MustCompile(`(?m)^<!-- api: ([a-z]+)\.Client\.([A-Za-z][A-Za-z0-9_]*) -->$`)

func TestAPIMarkerRequiresMatchingMethodSignature(t *testing.T) {
	t.Parallel()

	if !methodCardContainsSignature("### AllMids\n\n`func (c *Client) AllMids(ctx context.Context) (AllMidsResponse, error)`", "info", "AllMids") {
		t.Fatal("matching method signature was not recognized")
	}
	if methodCardContainsSignature("### AllMids\n\nDescription only.", "info", "AllMids") {
		t.Fatal("method card without a signature was recognized")
	}
	if methodCardContainsSignature("`func (c *Client) OtherMethod() error`", "info", "AllMids") {
		t.Fatal("different method signature was recognized")
	}
	if methodCardContainsSignature("`func (c *Client) OtherMethod() error` mentions AllMids(", "info", "AllMids") {
		t.Fatal("method name in prose was recognized as a signature")
	}
	if methodCardContainsSignature("`func (c *OtherClient) AllMids() error`", "info", "AllMids") {
		t.Fatal("non-Client receiver was recognized as a signature")
	}
	if methodCardContainsSignature("`func (c *info.Client) PlaceOrder() error`", "exchange", "PlaceOrder") {
		t.Fatal("wrong package Client receiver was recognized as a signature")
	}
}

func TestAPIDocumentationMarkersCoverExportedClientMethods(t *testing.T) {
	t.Parallel()

	documentation := map[string]string{
		"info":      "docs/api/info.md",
		"exchange":  "docs/api/exchange.md",
		"websocket": "docs/api/websocket.md",
		"explorer":  "docs/api/explorer.md",
	}
	for packageName, document := range documentation {
		exported := exportedClientMethods(t, packageName)
		marked := documentedClientMethods(t, document, packageName)
		assertMatchingMethods(t, packageName, exported, marked)
	}
}

func exportedClientMethods(t *testing.T, packageName string) map[string]int {
	t.Helper()
	packages, err := parser.ParseDir(token.NewFileSet(), packageName, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse %s package: %v", packageName, err)
	}
	pkg, ok := packages[packageName]
	if !ok {
		t.Fatalf("package %q was not parsed", packageName)
	}

	methods := make(map[string]int)
	for _, file := range pkg.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || !function.Name.IsExported() || !isClientPointerReceiver(function) {
				continue
			}
			methods[function.Name.Name]++
		}
	}
	return methods
}

func isClientPointerReceiver(function *ast.FuncDecl) bool {
	if len(function.Recv.List) != 1 {
		return false
	}
	pointer, ok := function.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	identifier, ok := pointer.X.(*ast.Ident)
	return ok && identifier.Name == "Client"
}

func documentedClientMethods(t *testing.T, document, packageName string) map[string]int {
	t.Helper()
	contents, err := os.ReadFile(filepath.Clean(document))
	if err != nil {
		t.Fatalf("read %s: %v", document, err)
	}
	text := string(contents)
	matches := apiMarkerRE.FindAllStringSubmatchIndex(text, -1)
	methods := make(map[string]int)
	for index, match := range matches {
		markerPackage := text[match[2]:match[3]]
		method := text[match[4]:match[5]]
		if markerPackage != packageName {
			t.Fatalf("%s contains marker for %s.Client.%s", document, markerPackage, method)
		}
		cardEnd := len(text)
		if index+1 < len(matches) {
			cardEnd = matches[index+1][0]
		}
		if !methodCardContainsSignature(text[match[1]:cardEnd], packageName, method) {
			t.Fatalf("%s marker for %s.Client.%s has no matching method signature", document, packageName, method)
		}
		methods[method]++
	}
	return methods
}

func methodCardContainsSignature(card, packageName, method string) bool {
	prefix := regexp.QuoteMeta(packageName) + `\.`
	if packageName == "info" {
		prefix = `(?:info\.)?`
	}
	signature := regexp.MustCompile(`func \([A-Za-z_][A-Za-z0-9_]* \*` + prefix + `Client\) ` + regexp.QuoteMeta(method) + `\(`)
	return signature.MatchString(card)
}

func assertMatchingMethods(t *testing.T, packageName string, exported, marked map[string]int) {
	t.Helper()
	var missing, duplicated, stale []string
	for method, count := range exported {
		if count != 1 {
			t.Fatalf("%s.Client.%s is declared %d times", packageName, method, count)
		}
		switch marked[method] {
		case 0:
			missing = append(missing, method)
		case 1:
		default:
			duplicated = append(duplicated, method)
		}
	}
	for method := range marked {
		if _, ok := exported[method]; !ok {
			stale = append(stale, method)
		}
	}
	sort.Strings(missing)
	sort.Strings(duplicated)
	sort.Strings(stale)
	if len(missing) != 0 || len(duplicated) != 0 || len(stale) != 0 {
		t.Fatalf("%s API documentation markers mismatch: missing=%v duplicated=%v stale=%v", packageName, missing, duplicated, stale)
	}
}
