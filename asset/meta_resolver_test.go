package asset

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apexllcc/hypersdk-go/info"
	"github.com/Apexllcc/hypersdk-go/transport"
	"github.com/Apexllcc/hypersdk-go/types"
)

func TestMetaResolverIndexesSpotAndHIP3Assets(t *testing.T) {
	t.Parallel()
	server, calls := metadataServer(t, 0)
	defer server.Close()
	r := newTestMetaResolver(t, server.URL, time.Hour)

	base, err := r.Resolve(context.Background(), "BTC")
	if err != nil {
		t.Fatal(err)
	}
	if base.ID != 0 || base.Kind != Perp {
		t.Fatalf("base asset = %+v", base)
	}
	spot, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "PURR/USDC", Kind: Spot})
	if err != nil {
		t.Fatal(err)
	}
	if spot.ID != 10007 || spot.SzDecimals != 2 || spot.Symbol != "@7" {
		t.Fatalf("spot asset = %+v", spot)
	}
	hip3, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "test:ABC", Kind: HIP3, DEX: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if hip3.ID != 110000 || hip3.DEX != "test" {
		t.Fatalf("HIP-3 asset = %+v", hip3)
	}
	byID, err := r.ResolveID(context.Background(), 110000)
	if err != nil {
		t.Fatal(err)
	}
	if byID != hip3 {
		t.Fatalf("ResolveID = %+v, want %+v", byID, hip3)
	}
	if got := calls.Load(); got != 4 {
		t.Fatalf("metadata calls = %d, want 4", got)
	}
}

func TestMetaResolverIndexesTestnetOutcomesWhenEnabled(t *testing.T) {
	t.Parallel()
	server, calls := metadataServer(t, 0)
	defer server.Close()
	client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	r, err := NewMetaResolver(client, WithOutcomeMetadata())
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "#100", Kind: Outcome})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.ID != 100000100 || outcome.Name != "+100" || outcome.SzDecimals != 0 {
		t.Fatalf("outcome asset = %+v", outcome)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("metadata calls = %d, want 1", got)
	}
}

func TestMetaResolverResolvesBaseAssetsWhenPerpDEXsFails(t *testing.T) {
	t.Parallel()
	server := failingOptionalMetadataServer(t, "perpDexs")
	defer server.Close()
	r := newTestMetaResolver(t, server.URL, time.Hour)

	assertResolvesBTCAndPURRUSDC(t, r)
	if _, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "test:ABC", Kind: HIP3, DEX: "test"}); err == nil {
		t.Fatal("HIP-3 resolution unexpectedly succeeded")
	}
}

func TestMetaResolverResolvesBaseAssetsWhenHIP3MetadataFails(t *testing.T) {
	t.Parallel()
	server := failingOptionalMetadataServer(t, "hip3Meta")
	defer server.Close()
	r := newTestMetaResolver(t, server.URL, time.Hour)

	assertResolvesBTCAndPURRUSDC(t, r)
	if _, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "test:ABC", Kind: HIP3, DEX: "test"}); err == nil {
		t.Fatal("HIP-3 resolution unexpectedly succeeded")
	}
}

func TestMetaResolverResolvesBaseAssetsWhenOutcomeMetadataFails(t *testing.T) {
	t.Parallel()
	server := failingOptionalMetadataServer(t, "outcomeMeta")
	defer server.Close()
	client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	r, err := NewMetaResolver(client, WithOutcomeMetadata(), WithMetaRefreshTTL(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	assertResolvesBTCAndPURRUSDC(t, r)
	if _, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "#100", Kind: Outcome}); err == nil {
		t.Fatal("outcome resolution unexpectedly succeeded")
	}
}

func TestMetaResolverCachesInitializedEmptyHIP3SnapshotUntilTTL(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request struct {
			Type string `json:"type"`
		}
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if request.Type != "perpDexs" {
			t.Errorf("unexpected request type %q", request.Type)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		calls.Add(1)
		_, _ = w.Write([]byte(`[null]`))
	}))
	defer server.Close()
	r := newTestMetaResolver(t, server.URL, time.Minute)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	ref := types.MarketRef{Symbol: "test:UNKNOWN", Kind: HIP3, DEX: "test"}

	if _, err := r.ResolveMarket(context.Background(), ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("first empty HIP-3 lookup error = %v, want ErrNotFound", err)
	}
	if _, err := r.ResolveMarket(context.Background(), ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cached empty HIP-3 lookup error = %v, want ErrNotFound", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("HIP-3 calls before expiry = %d, want 1", got)
	}

	now = now.Add(time.Minute)
	if _, err := r.ResolveMarket(context.Background(), ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired empty HIP-3 lookup error = %v, want ErrNotFound", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("HIP-3 calls after expiry = %d, want 2", got)
	}
}

func TestMetaResolverCachesInitializedEmptyOutcomeSnapshotUntilTTL(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request struct {
			Type string `json:"type"`
		}
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if request.Type != "outcomeMeta" {
			t.Errorf("unexpected request type %q", request.Type)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		calls.Add(1)
		_, _ = w.Write([]byte(`{"outcomes":[],"questions":[]}`))
	}))
	defer server.Close()
	client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	r, err := NewMetaResolver(client, WithOutcomeMetadata(), WithMetaRefreshTTL(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	ref := types.MarketRef{Symbol: "#100", Kind: Outcome}

	if _, err := r.ResolveMarket(context.Background(), ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("first empty outcome lookup error = %v, want ErrNotFound", err)
	}
	if _, err := r.ResolveMarket(context.Background(), ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cached empty outcome lookup error = %v, want ErrNotFound", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("outcome calls before expiry = %d, want 1", got)
	}

	now = now.Add(time.Minute)
	if _, err := r.ResolveMarket(context.Background(), ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired empty outcome lookup error = %v, want ErrNotFound", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("outcome calls after expiry = %d, want 2", got)
	}
}

func TestMetaResolverRefreshesInitializedOptionalNamespacesWithZeroTTL(t *testing.T) {
	t.Parallel()
	var hip3Calls atomic.Int32
	var outcomeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request struct {
			Type string `json:"type"`
			DEX  string `json:"dex"`
		}
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		switch request.Type {
		case "meta":
			if request.DEX == "test" {
				if hip3Calls.Add(1) == 1 {
					_, _ = w.Write([]byte(`{"universe":[{"name":"test:OLD","szDecimals":1,"maxLeverage":3}]}`))
				} else {
					_, _ = w.Write([]byte(`{"universe":[{"name":"test:NEW","szDecimals":2,"maxLeverage":3}]}`))
				}
				return
			}
			_, _ = w.Write([]byte(`{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":50}]}`))
		case "spotMeta":
			_, _ = w.Write([]byte(`{"tokens":[],"universe":[]}`))
		case "perpDexs":
			_, _ = w.Write([]byte(`[{"name":"test"}]`))
		case "outcomeMeta":
			if outcomeCalls.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"outcomes":[{"outcome":10,"sideSpecs":[{},{}]}],"questions":[]}`))
			} else {
				_, _ = w.Write([]byte(`{"outcomes":[{"outcome":11,"sideSpecs":[{},{}]}],"questions":[]}`))
			}
		default:
			t.Errorf("unexpected request: %+v", request)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	r, err := NewMetaResolver(client, WithOutcomeMetadata(), WithMetaRefreshTTL(0))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "test:OLD", Kind: HIP3, DEX: "test"}); err != nil {
		t.Fatalf("initialize HIP-3 cache: %v", err)
	}
	if _, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "#100", Kind: Outcome}); err != nil {
		t.Fatalf("initialize outcome cache: %v", err)
	}
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if _, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "test:NEW", Kind: HIP3, DEX: "test"}); err != nil {
		t.Fatalf("resolve refreshed HIP-3 asset: %v", err)
	}
	if _, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "#110", Kind: Outcome}); err != nil {
		t.Fatalf("resolve refreshed outcome asset: %v", err)
	}
	if got := hip3Calls.Load(); got != 2 {
		t.Fatalf("HIP-3 metadata calls = %d, want 2", got)
	}
	if got := outcomeCalls.Load(); got != 2 {
		t.Fatalf("outcome metadata calls = %d, want 2", got)
	}
}

func TestMetaResolverOptionalRefreshFailurePreservesRefreshedBaseAndOptionalSnapshot(t *testing.T) {
	t.Parallel()
	var failOptional atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request struct {
			Type string `json:"type"`
			DEX  string `json:"dex"`
		}
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		switch request.Type {
		case "meta":
			if request.DEX == "test" {
				if failOptional.Load() {
					http.Error(w, "optional metadata unavailable", http.StatusBadRequest)
					return
				}
				_, _ = w.Write([]byte(`{"universe":[{"name":"test:OLD","szDecimals":1,"maxLeverage":3}]}`))
				return
			}
			if failOptional.Load() {
				_, _ = w.Write([]byte(`{"universe":[{"name":"ETH","szDecimals":4,"maxLeverage":50}]}`))
			} else {
				_, _ = w.Write([]byte(`{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":50}]}`))
			}
		case "spotMeta":
			_, _ = w.Write([]byte(`{"tokens":[],"universe":[]}`))
		case "perpDexs":
			_, _ = w.Write([]byte(`[{"name":"test"}]`))
		default:
			t.Errorf("unexpected request: %+v", request)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	r := newTestMetaResolver(t, server.URL, 0)
	if _, err := r.Resolve(context.Background(), "BTC"); err != nil {
		t.Fatalf("initialize base cache: %v", err)
	}
	oldRef := types.MarketRef{Symbol: "test:OLD", Kind: HIP3, DEX: "test"}
	if _, err := r.ResolveMarket(context.Background(), oldRef); err != nil {
		t.Fatalf("initialize HIP-3 cache: %v", err)
	}

	failOptional.Store(true)
	if err := r.Refresh(context.Background()); err == nil {
		t.Fatal("refresh unexpectedly ignored initialized HIP-3 failure")
	}
	if _, err := r.Resolve(context.Background(), "ETH"); err != nil {
		t.Fatalf("refreshed base snapshot unavailable after optional failure: %v", err)
	}
	if _, err := r.ResolveMarket(context.Background(), oldRef); err != nil {
		t.Fatalf("last successful HIP-3 snapshot unavailable after refresh failure: %v", err)
	}
}

func TestMetaResolverResolveIDRejectsCrossNamespaceCollisions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		id           int
		spotIndex    int
		perpDEXs     string
		hip3Meta     string
		outcomeMeta  string
		withOutcomes bool
	}{
		{
			name:      "base and HIP-3",
			id:        100000,
			spotIndex: 90000,
			perpDEXs:  `[{"name":"test"}]`,
			hip3Meta:  `{"universe":[{"name":"test:COLLISION","szDecimals":1,"maxLeverage":3}]}`,
		},
		{
			name:         "base and outcome",
			id:           100000100,
			spotIndex:    99990100,
			perpDEXs:     `[]`,
			outcomeMeta:  `{"outcomes":[{"outcome":10,"sideSpecs":[{},{}]}],"questions":[]}`,
			withOutcomes: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var request struct {
					Type string `json:"type"`
					DEX  string `json:"dex"`
				}
				if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				switch request.Type {
				case "meta":
					if request.DEX != "" {
						_, _ = w.Write([]byte(tt.hip3Meta))
						return
					}
					_, _ = w.Write([]byte(`{"universe":[]}`))
				case "spotMeta":
					response := struct {
						Tokens   []info.SpotToken `json:"tokens"`
						Universe []info.SpotPair  `json:"universe"`
					}{
						Tokens:   []info.SpotToken{{Name: "BASE", SzDecimals: 2, Index: 1}, {Name: "USDC", SzDecimals: 6, Index: 0}},
						Universe: []info.SpotPair{{Name: "@1", Tokens: [2]int{1, 0}, Index: tt.spotIndex}},
					}
					if err := json.NewEncoder(w).Encode(response); err != nil {
						t.Errorf("encode spot metadata: %v", err)
					}
				case "perpDexs":
					_, _ = w.Write([]byte(tt.perpDEXs))
				case "outcomeMeta":
					_, _ = w.Write([]byte(tt.outcomeMeta))
				default:
					t.Errorf("unexpected request: %+v", request)
					http.Error(w, "unexpected request", http.StatusBadRequest)
				}
			}))
			defer server.Close()
			client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
			options := []MetaResolverOption{WithMetaRefreshTTL(time.Hour)}
			if tt.withOutcomes {
				options = append(options, WithOutcomeMetadata())
			}
			r, err := NewMetaResolver(client, options...)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := r.ResolveID(context.Background(), tt.id); err == nil || err.Error() != "ambiguous asset ID: "+strconv.Itoa(tt.id) {
				t.Fatalf("ResolveID(%d) error = %v, want cross-namespace ambiguity", tt.id, err)
			}
		})
	}
}

func TestMetaResolverUnknownUnqualifiedSymbolIgnoresOptionalEndpointFailures(t *testing.T) {
	t.Parallel()
	for _, failing := range []string{"perpDexs", "outcomeMeta"} {
		t.Run(failing, func(t *testing.T) {
			var optionalCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var request struct {
					Type string `json:"type"`
				}
				if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				switch request.Type {
				case "meta":
					_, _ = w.Write([]byte(`{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":50}]}`))
				case "spotMeta":
					_, _ = w.Write([]byte(`{"tokens":[],"universe":[]}`))
				case "perpDexs", "outcomeMeta":
					optionalCalls.Add(1)
					if request.Type == failing {
						http.Error(w, "optional metadata unavailable", http.StatusBadRequest)
						return
					}
					if request.Type == "perpDexs" {
						_, _ = w.Write([]byte(`[]`))
					} else {
						_, _ = w.Write([]byte(`{"outcomes":[],"questions":[]}`))
					}
				default:
					t.Errorf("unexpected request: %+v", request)
					http.Error(w, "unexpected request", http.StatusBadRequest)
				}
			}))
			defer server.Close()
			client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
			r, err := NewMetaResolver(client, WithOutcomeMetadata(), WithMetaRefreshTTL(time.Hour))
			if err != nil {
				t.Fatal(err)
			}

			if _, err := r.Resolve(context.Background(), "UNKNOWN"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("Resolve(UNKNOWN) error = %v, want ErrNotFound", err)
			}
			if got := optionalCalls.Load(); got != 0 {
				t.Fatalf("optional endpoint calls = %d, want 0", got)
			}
		})
	}
}

func TestMetaResolverCoalescesConcurrentInitialLoads(t *testing.T) {
	t.Parallel()
	server, calls := metadataServer(t, 10*time.Millisecond)
	defer server.Close()
	r := newTestMetaResolver(t, server.URL, time.Hour)

	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.ResolveID(context.Background(), 0)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("metadata calls = %d, want 2 for a coalesced base load", got)
	}
}

func TestMetaResolverRefreshesAfterTTLAndCanRefreshExplicitly(t *testing.T) {
	t.Parallel()
	server, calls := metadataServer(t, 0)
	defer server.Close()
	r := newTestMetaResolver(t, server.URL, time.Minute)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	if _, err := r.Resolve(context.Background(), "BTC"); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("initial metadata calls = %d", got)
	}
	now = now.Add(time.Minute)
	if _, err := r.Resolve(context.Background(), "BTC"); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 4 {
		t.Fatalf("expired metadata calls = %d, want 4", got)
	}
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 6 {
		t.Fatalf("explicit refresh calls = %d, want 6", got)
	}
}

func TestMetaResolverCoalescedRefreshPropagatesFailure(t *testing.T) {
	t.Parallel()
	failedMetaStarted := make(chan struct{})
	releaseFailure := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFailure) }) }
	defer release()
	var mainMetaCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Type string `json:"type"`
			DEX  string `json:"dex"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Type == "meta" && request.DEX == "" && mainMetaCalls.Add(1) == 2 {
			close(failedMetaStarted)
			<-releaseFailure
			http.Error(w, "metadata unavailable", http.StatusBadRequest)
			return
		}
		switch request.Type {
		case "meta":
			if request.DEX == "test" {
				_, _ = w.Write([]byte(`{"universe":[{"name":"test:ABC","szDecimals":1,"maxLeverage":3}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":50}]}`))
		case "spotMeta":
			_, _ = w.Write([]byte(`{"tokens":[{"name":"PURR","szDecimals":2,"index":7},{"name":"USDC","szDecimals":6,"index":0}],"universe":[{"name":"@7","tokens":[7,0],"index":7}]}`))
		case "perpDexs":
			_, _ = w.Write([]byte(`[null,{"name":"test"}]`))
		case "outcomeMeta":
			_, _ = w.Write([]byte(`{"outcomes":[{"outcome":10,"name":"yes","description":"d","sideSpecs":[{"name":"yes"},{"name":"no"}],"quoteToken":"USDC"}],"questions":[]}`))
		default:
			t.Fatalf("unexpected request: %+v", request)
		}
	}))
	defer server.Close()
	r := newTestMetaResolver(t, server.URL, time.Hour)
	if _, err := r.Resolve(context.Background(), "BTC"); err != nil {
		t.Fatal(err)
	}

	first := make(chan error, 1)
	go func() { first <- r.Refresh(context.Background()) }()
	<-failedMetaStarted
	second := make(chan error, 1)
	go func() { second <- r.Refresh(context.Background()) }()
	select {
	case err := <-second:
		t.Fatalf("coalesced refresh returned before source completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	release()
	if err := <-first; err == nil {
		t.Fatal("first refresh unexpectedly succeeded")
	}
	if err := <-second; err == nil {
		t.Fatal("coalesced refresh unexpectedly succeeded")
	}
}

func TestMetaResolverCanceledWaiterLeavesSharedLoadRunning(t *testing.T) {
	t.Parallel()
	metaStarted := make(chan struct{})
	releaseMeta := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseMeta) }) }
	defer release()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Type string `json:"type"`
			DEX  string `json:"dex"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request.Type {
		case "meta":
			if request.DEX == "" {
				select {
				case <-metaStarted:
				default:
					close(metaStarted)
					<-releaseMeta
				}
				_, _ = w.Write([]byte(`{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":50}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"universe":[{"name":"test:ABC","szDecimals":1,"maxLeverage":3}]}`))
		case "spotMeta":
			_, _ = w.Write([]byte(`{"tokens":[{"name":"PURR","szDecimals":2,"index":7},{"name":"USDC","szDecimals":6,"index":0}],"universe":[{"name":"@7","tokens":[7,0],"index":7}]}`))
		case "perpDexs":
			_, _ = w.Write([]byte(`[null,{"name":"test"}]`))
		case "outcomeMeta":
			_, _ = w.Write([]byte(`{"outcomes":[{"outcome":10,"name":"yes","description":"d","sideSpecs":[{"name":"yes"},{"name":"no"}],"quoteToken":"USDC"}],"questions":[]}`))
		default:
			t.Fatalf("unexpected request: %+v", request)
		}
	}))
	defer server.Close()
	r := newTestMetaResolver(t, server.URL, time.Hour)
	first := make(chan error, 1)
	go func() {
		_, err := r.ResolveID(context.Background(), 0)
		first <- err
	}()
	<-metaStarted
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.ResolveID(ctx, 0); err != context.Canceled {
		t.Fatalf("canceled waiter error = %v, want context.Canceled", err)
	}
	release()
	if err := <-first; err != nil {
		t.Fatalf("shared load failed after canceled waiter: %v", err)
	}
}

func TestStaticResolverResolvesAssetID(t *testing.T) {
	t.Parallel()
	r := NewStaticResolver([]Asset{{ID: 0, Symbol: "BTC", Kind: Perp}})
	got, err := r.ResolveID(context.Background(), 0)
	if err != nil || got.Symbol != "BTC" {
		t.Fatalf("ResolveID = %+v, %v", got, err)
	}
	if _, err := r.ResolveID(context.Background(), 1); err == nil {
		t.Fatal("unknown ID resolved")
	}
}

func TestCachedResolverNamespacesSymbolAndMarketEntries(t *testing.T) {
	t.Parallel()
	source := cacheTestResolver{}
	r, err := NewCachedResolver(source, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	bySymbol, err := r.Resolve(context.Background(), "perp::BTC")
	if err != nil {
		t.Fatal(err)
	}
	byMarket, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "BTC", Kind: Perp})
	if err != nil {
		t.Fatal(err)
	}
	if bySymbol.ID != 1 || byMarket.ID != 2 {
		t.Fatalf("cache entries collided: symbol=%+v market=%+v", bySymbol, byMarket)
	}
}

type cacheTestResolver struct{}

func (cacheTestResolver) Resolve(_ context.Context, symbol string) (Asset, error) {
	if symbol != "perp::BTC" {
		return Asset{}, ErrNotFound
	}
	return Asset{ID: 1, Symbol: symbol, Kind: Perp}, nil
}

func (cacheTestResolver) ResolveMarket(_ context.Context, ref types.MarketRef) (Asset, error) {
	if ref.Symbol != "BTC" || ref.Kind != Perp || ref.DEX != "" {
		return Asset{}, ErrNotFound
	}
	return Asset{ID: 2, Symbol: "BTC", Kind: Perp}, nil
}

func newTestMetaResolver(t *testing.T, endpoint string, ttl time.Duration) *MetaResolver {
	t.Helper()
	client := info.NewClient(endpoint, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	r, err := NewMetaResolver(client, WithMetaRefreshTTL(ttl))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func assertResolvesBTCAndPURRUSDC(t *testing.T, r *MetaResolver) {
	t.Helper()
	if _, err := r.Resolve(context.Background(), "BTC"); err != nil {
		t.Fatalf("resolve BTC: %v", err)
	}
	if _, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "PURR/USDC", Kind: Spot}); err != nil {
		t.Fatalf("resolve PURR/USDC: %v", err)
	}
}

func failingOptionalMetadataServer(t *testing.T, failing string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Type string `json:"type"`
			DEX  string `json:"dex"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if (failing == request.Type) || (failing == "hip3Meta" && request.Type == "meta" && request.DEX == "test") {
			http.Error(w, "optional metadata unavailable", http.StatusBadRequest)
			return
		}
		switch request.Type {
		case "meta":
			if request.DEX == "test" {
				_, _ = w.Write([]byte(`{"universe":[{"name":"test:ABC","szDecimals":1,"maxLeverage":3}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":50}]}`))
		case "spotMeta":
			_, _ = w.Write([]byte(`{"tokens":[{"name":"PURR","szDecimals":2,"index":7},{"name":"USDC","szDecimals":6,"index":0}],"universe":[{"name":"@7","tokens":[7,0],"index":7}]}`))
		case "perpDexs":
			_, _ = w.Write([]byte(`[null,{"name":"test"}]`))
		case "outcomeMeta":
			_, _ = w.Write([]byte(`{"outcomes":[{"outcome":10,"name":"yes","description":"d","sideSpecs":[{"name":"yes"},{"name":"no"}],"quoteToken":"USDC"}],"questions":[]}`))
		default:
			t.Fatalf("unexpected request: %+v", request)
		}
	}))
}

func metadataServer(t *testing.T, delay time.Duration) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delay > 0 {
			time.Sleep(delay)
		}
		var request struct {
			Type string `json:"type"`
			DEX  string `json:"dex"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		calls.Add(1)
		switch request.Type {
		case "meta":
			if request.DEX == "test" {
				_, _ = w.Write([]byte(`{"universe":[{"name":"test:ABC","szDecimals":1,"maxLeverage":3}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":50}]}`))
		case "spotMeta":
			_, _ = w.Write([]byte(`{"tokens":[{"name":"PURR","szDecimals":2,"index":7},{"name":"USDC","szDecimals":6,"index":0}],"universe":[{"name":"@7","tokens":[7,0],"index":7}]}`))
		case "perpDexs":
			_, _ = w.Write([]byte(`[null,{"name":"test"}]`))
		case "outcomeMeta":
			_, _ = w.Write([]byte(`{"outcomes":[{"outcome":10,"name":"yes","description":"d","sideSpecs":[{"name":"yes"},{"name":"no"}],"quoteToken":"USDC"}],"questions":[]}`))
		default:
			t.Fatalf("unexpected request: %+v", request)
		}
	}))
	return server, &calls
}
