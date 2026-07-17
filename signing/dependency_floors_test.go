package signing_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestReviewedDependencyFloorsAreDeclared(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}

	contents, err := os.ReadFile(filepath.Join(filepath.Dir(testFile), "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}

	goVersion := moduleDirective(contents, "go")
	if compareVersion(goVersion, "1.26.5") < 0 {
		t.Errorf("go directive %q is below reviewed floor %q", goVersion, "1.26.5")
	}

	gethVersion := moduleRequirement(contents, "github.com/ethereum/go-ethereum")
	if compareVersion(gethVersion, "1.17.0") < 0 {
		t.Errorf("go-ethereum requirement %q is below reviewed floor %q", gethVersion, "v1.17.0")
	}
}

func moduleDirective(contents []byte, directive string) string {
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == directive {
			return fields[1]
		}
	}
	return ""
}

func moduleRequirement(contents []byte, module string) string {
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == module {
			return fields[1]
		}
	}
	return ""
}

func compareVersion(got, want string) int {
	gotParts := strings.Split(strings.TrimPrefix(got, "v"), ".")
	wantParts := strings.Split(strings.TrimPrefix(want, "v"), ".")
	for i := 0; i < len(wantParts); i++ {
		gotPart := 0
		if i < len(gotParts) {
			var err error
			gotPart, err = strconv.Atoi(gotParts[i])
			if err != nil {
				return -1
			}
		}
		wantPart, err := strconv.Atoi(wantParts[i])
		if err != nil {
			panic(fmt.Sprintf("invalid expected version %q: %v", want, err))
		}
		if gotPart < wantPart {
			return -1
		}
		if gotPart > wantPart {
			return 1
		}
	}
	return 0
}
