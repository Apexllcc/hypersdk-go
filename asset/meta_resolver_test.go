package asset

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
