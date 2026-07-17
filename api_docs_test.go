package hyperliquid_test

import (
	"fmt"
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
var goCodeBlockRE = regexp.MustCompile("(?s)```go\\n(.*?)\\n```")
var inlineMethodSignatureRE = regexp.MustCompile("`(func \\([^`\\n]+)`")

type methodSignature struct {
	receiver string
	params   []string
	results  []string
}

func TestAPIMarkerRequiresMatchingMethodSignature(t *testing.T) {
	t.Parallel()

	matching := "### AllMids\n\n```go\nfunc (c *info.Client) AllMids(ctx context.Context) (info.AllMidsResponse, error)\n```"
	if !methodCardContainsSignature(matching, "info", "AllMids") {
		t.Fatal("matching method signature was not recognized")
	}
	if methodCardContainsSignature("### AllMids\n\nDescription only.", "info", "AllMids") {
		t.Fatal("method card without a signature was recognized")
	}
	if methodCardContainsSignature("```go\nfunc (c *info.Client) OtherMethod() error\n```", "info", "AllMids") {
		t.Fatal("different method signature was recognized")
	}
	if methodCardContainsSignature("```go\nfunc (c *info.Client) OtherMethod() error\n``` mentions AllMids(", "info", "AllMids") {
		t.Fatal("method name in prose was recognized as a signature")
	}
	if methodCardContainsSignature("```go\nfunc (c *OtherClient) AllMids() error\n```", "info", "AllMids") {
		t.Fatal("non-Client receiver was recognized as a signature")
	}
	if methodCardContainsSignature("```go\nfunc (c *info.Client) PlaceOrder() error\n```", "exchange", "PlaceOrder") {
		t.Fatal("wrong package Client receiver was recognized as a signature")
	}
}

func TestAPIMarkerRejectsParameterTypeDrift(t *testing.T) {
	t.Parallel()

	source := mustParseMethodSignature(t, "func (c *Client) AllMids(ctx context.Context) (AllMidsResponse, error)", "info")
	card := "```go\nfunc (c *info.Client) AllMids(ctx string) (info.AllMidsResponse, error)\n```"
	documented, ok := methodCardSignature(card, "info", "AllMids")
	if !ok {
		t.Fatal("documented signature was not recognized")
	}
	if signaturesEqual(source, documented) {
		t.Fatalf("parameter type drift was accepted: %+v", documented)
	}
}

func TestAPIMarkerRejectsResultTypeDrift(t *testing.T) {
	t.Parallel()

	source := mustParseMethodSignature(t, "func (c *Client) AllMids(ctx context.Context) (AllMidsResponse, error)", "info")
	card := "```go\nfunc (c *info.Client) AllMids(ctx context.Context) (map[string]string, error)\n```"
	documented, ok := methodCardSignature(card, "info", "AllMids")
	if !ok {
		t.Fatal("documented signature was not recognized")
	}
	if signaturesEqual(source, documented) {
		t.Fatalf("result type drift was accepted: %+v", documented)
	}
}

func TestAPIMarkerIgnoresParameterNames(t *testing.T) {
	t.Parallel()

	source := mustParseMethodSignature(t, "func (c *Client) AllMids(ctx context.Context) (AllMidsResponse, error)", "info")
	card := "```go\nfunc (receiver *info.Client) AllMids(requestContext context.Context) (info.AllMidsResponse, error)\n```"
	documented, ok := methodCardSignature(card, "info", "AllMids")
	if !ok {
		t.Fatal("documented signature was not recognized")
	}
	if !signaturesEqual(source, documented) {
		t.Fatalf("parameter name-only change caused drift: source=%s documented=%s", formatSignature(source), formatSignature(documented))
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

func exportedClientMethods(t *testing.T, packageName string) map[string]methodSignature {
	t.Helper()
	entries, err := os.ReadDir(packageName)
	if err != nil {
		t.Fatalf("read %s package: %v", packageName, err)
	}

	methods := make(map[string]methodSignature)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(packageName, entry.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parse %s source %q: %v", packageName, entry.Name(), err)
		}
		if file.Name.Name != packageName {
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || !function.Name.IsExported() || !isClientPointerReceiver(function) {
				continue
			}
			if _, exists := methods[function.Name.Name]; exists {
				t.Fatalf("%s.Client.%s is declared more than once", packageName, function.Name.Name)
			}
			methods[function.Name.Name] = signatureFromDecl(t, function, packageName)
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
	switch receiver := pointer.X.(type) {
	case *ast.Ident:
		return receiver.Name == "Client"
	case *ast.SelectorExpr:
		return receiver.Sel.Name == "Client"
	default:
		return false
	}
}

func documentedClientMethods(t *testing.T, document, packageName string) map[string]methodSignature {
	t.Helper()
	contents, err := os.ReadFile(filepath.Clean(document))
	if err != nil {
		t.Fatalf("read %s: %v", document, err)
	}
	text := string(contents)
	matches := apiMarkerRE.FindAllStringSubmatchIndex(text, -1)
	methods := make(map[string]methodSignature)
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
		signature, ok := methodCardSignature(text[match[1]:cardEnd], packageName, method)
		if !ok {
			t.Fatalf("%s marker for %s.Client.%s has no parseable Go method signature", document, packageName, method)
		}
		if _, exists := methods[method]; exists {
			t.Fatalf("%s has duplicate marker for %s.Client.%s", document, packageName, method)
		}
		methods[method] = signature
	}
	return methods
}

func methodCardContainsSignature(card, packageName, method string) bool {
	_, ok := methodCardSignature(card, packageName, method)
	return ok
}

func methodCardSignature(card, packageName, method string) (methodSignature, bool) {
	var declarations []string
	for _, block := range goCodeBlockRE.FindAllStringSubmatch(card, -1) {
		declarations = append(declarations, block[1])
	}
	for _, inline := range inlineMethodSignatureRE.FindAllStringSubmatch(card, -1) {
		declarations = append(declarations, inline[1])
	}
	for _, declaration := range declarations {
		// Documentation cards show declaration signatures without function bodies.
		// Append an empty body so the Go parser validates the signature itself.
		file, err := parser.ParseFile(token.NewFileSet(), "card.go", "package documentation\n"+strings.TrimSpace(declaration)+" {}", 0)
		if err != nil {
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || function.Name.Name != method || !isClientPointerReceiver(function) {
				continue
			}
			signature, err := signatureFromDeclNoTest(function, packageName)
			if err != nil {
				continue
			}
			if signature.receiver == "*"+packageName+".Client" {
				return signature, true
			}
		}
	}
	return methodSignature{}, false
}

func mustParseMethodSignature(t *testing.T, declaration, packageName string) methodSignature {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "source.go", "package source\n"+declaration, 0)
	if err != nil {
		t.Fatalf("parse method: %v", err)
	}
	function, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok {
		t.Fatal("parsed declaration is not a function")
	}
	return signatureFromDecl(t, function, packageName)
}

func signatureFromDecl(t *testing.T, function *ast.FuncDecl, packageName string) methodSignature {
	t.Helper()
	signature, err := signatureFromDeclNoTest(function, packageName)
	if err != nil {
		t.Fatalf("parse signature for %s.Client.%s: %v", packageName, function.Name.Name, err)
	}
	return signature
}

func signatureFromDeclNoTest(function *ast.FuncDecl, packageName string) (methodSignature, error) {
	if len(function.Recv.List) != 1 {
		return methodSignature{}, fmt.Errorf("receiver count = %d", len(function.Recv.List))
	}
	signature := methodSignature{
		receiver: canonicalType(function.Recv.List[0].Type, packageName),
		params:   canonicalFields(function.Type.Params, packageName),
		results:  canonicalFields(function.Type.Results, packageName),
	}
	if strings.Contains(formatSignature(signature), "<unsupported:") {
		return methodSignature{}, fmt.Errorf("unsupported signature type")
	}
	return signature, nil
}

func canonicalFields(fields *ast.FieldList, packageName string) []string {
	if fields == nil {
		return nil
	}
	var types []string
	for _, field := range fields.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for range count {
			types = append(types, canonicalType(field.Type, packageName))
		}
	}
	return types
}

func canonicalType(expression ast.Expr, packageName string) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		if builtinTypeNames[expression.Name] {
			return expression.Name
		}
		return packageName + "." + expression.Name
	case *ast.SelectorExpr:
		return canonicalSelector(expression.X) + "." + expression.Sel.Name
	case *ast.StarExpr:
		return "*" + canonicalType(expression.X, packageName)
	case *ast.ArrayType:
		if expression.Len == nil {
			return "[]" + canonicalType(expression.Elt, packageName)
		}
		return "[" + canonicalExpression(expression.Len, packageName) + "]" + canonicalType(expression.Elt, packageName)
	case *ast.MapType:
		return "map[" + canonicalType(expression.Key, packageName) + "]" + canonicalType(expression.Value, packageName)
	case *ast.Ellipsis:
		return "..." + canonicalType(expression.Elt, packageName)
	case *ast.ChanType:
		prefix := "chan "
		switch expression.Dir {
		case ast.SEND:
			prefix = "chan<- "
		case ast.RECV:
			prefix = "<-chan "
		}
		return prefix + canonicalType(expression.Value, packageName)
	case *ast.ParenExpr:
		return "(" + canonicalType(expression.X, packageName) + ")"
	case *ast.IndexExpr:
		return canonicalType(expression.X, packageName) + "[" + canonicalType(expression.Index, packageName) + "]"
	case *ast.IndexListExpr:
		indices := make([]string, 0, len(expression.Indices))
		for _, index := range expression.Indices {
			indices = append(indices, canonicalType(index, packageName))
		}
		return canonicalType(expression.X, packageName) + "[" + strings.Join(indices, ",") + "]"
	default:
		return canonicalExpression(expression, packageName)
	}
}

func canonicalSelector(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.SelectorExpr:
		return canonicalSelector(expression.X) + "." + expression.Sel.Name
	default:
		return "<invalid-selector>"
	}
}

func canonicalExpression(expression ast.Expr, packageName string) string {
	if literal, ok := expression.(*ast.BasicLit); ok {
		return literal.Value
	}
	return "<unsupported:" + fmt.Sprintf("%T", expression) + ">"
}

var builtinTypeNames = map[string]bool{
	"any": true, "bool": true, "byte": true, "complex64": true, "complex128": true,
	"comparable": true, "error": true, "float32": true, "float64": true, "int": true,
	"int8": true, "int16": true, "int32": true, "int64": true, "rune": true,
	"string": true, "uint": true, "uint8": true, "uint16": true, "uint32": true,
	"uint64": true, "uintptr": true,
}

func assertMatchingMethods(t *testing.T, packageName string, exported, marked map[string]methodSignature) {
	t.Helper()
	var missing, stale, drifted []string
	for method, signature := range exported {
		documented, ok := marked[method]
		if !ok {
			missing = append(missing, method)
			continue
		}
		if !signaturesEqual(signature, documented) {
			drifted = append(drifted, method+" source="+formatSignature(signature)+" documented="+formatSignature(documented))
		}
	}
	for method := range marked {
		if _, ok := exported[method]; !ok {
			stale = append(stale, method)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	sort.Strings(drifted)
	if len(missing) != 0 || len(stale) != 0 || len(drifted) != 0 {
		t.Fatalf("%s API documentation markers mismatch: missing=%v stale=%v signature_drift=%v", packageName, missing, stale, drifted)
	}
}

func formatSignature(signature methodSignature) string {
	return signature.receiver + "(" + strings.Join(signature.params, ",") + ")(" + strings.Join(signature.results, ",") + ")"
}

func signaturesEqual(left, right methodSignature) bool {
	return left.receiver == right.receiver &&
		strings.Join(left.params, "\x00") == strings.Join(right.params, "\x00") &&
		strings.Join(left.results, "\x00") == strings.Join(right.results, "\x00")
}
