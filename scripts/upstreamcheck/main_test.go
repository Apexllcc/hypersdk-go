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

func testLock() lockFile {
	return lockFile{
		Version: 1,
		Documents: []documentLock{{
			Name:            "api",
			URL:             "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api",
			Revision:        "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SHA256:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Summary:         "Official API contract.",
			RequiredMarkers: []string{"nonce"},
		}},
		PythonSDK: pythonSDKLock{
			Repository: "https://github.com/hyperliquid-dex/hyperliquid-python-sdk",
			HeadAPIURL: "https://api.github.com/repos/hyperliquid-dex/hyperliquid-python-sdk/commits/HEAD",
			RawBaseURL: "https://raw.githubusercontent.com/hyperliquid-dex/hyperliquid-python-sdk",
			Revision:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Files: []pythonFileLock{{
				Path:   "hyperliquid/exchange.py",
				SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			}},
		},
	}
}
