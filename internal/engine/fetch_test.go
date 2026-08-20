package engine

import (
	"os"
	"testing"
	"time"
)

// TestListMarketETF covers the one market that does not come from an HTTP JSON
// API: etf is served over anonymous FTP from the NASDAQ symbol directory.
// go-quote used to list it as valid while having no URL for it, which made it
// unresolvable; this guards the path that replaced that workaround.
//
// It reaches the network, so it is skipped unless explicitly enabled.
func TestListMarketETF(t *testing.T) {
	if testing.Short() {
		t.Skip("needs network access")
	}
	if os.Getenv("GO_SCAN_NETWORK_TESTS") == "" {
		t.Skip("set GO_SCAN_NETWORK_TESTS=1 to run")
	}

	symbols, err := NewQuoteFetcher(nil).Market(t.Context(), "etf")
	if err != nil {
		t.Fatalf("Market(etf): %v", err)
	}
	if len(symbols) < 100 {
		t.Errorf("got %d symbols, expected the full ETF directory", len(symbols))
	}
}

func TestCacheRoundTrip(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("new cache: %v", err)
	}

	key := cacheKey("quote", "coinbase", "BTC-USD", "2024-01-01", "2024-06-30")
	if ok, err := cache.Get(key, 0, &[]string{}); err != nil || ok {
		t.Fatalf("empty cache reported a hit (ok=%v err=%v)", ok, err)
	}

	want := []string{"AAA", "BBB"}
	if err := cache.Put(key, want); err != nil {
		t.Fatalf("put: %v", err)
	}

	var got []string
	ok, err := cache.Get(key, 0, &got)
	if err != nil || !ok {
		t.Fatalf("miss after put (ok=%v err=%v)", ok, err)
	}
	if len(got) != 2 || got[0] != "AAA" {
		t.Errorf("got %v, want %v", got, want)
	}

	// A zero TTL never expires; a tiny one always has.
	if ok, _ := cache.Get(key, time.Nanosecond, &got); ok {
		t.Error("entry should have expired")
	}

	if err := cache.Clear(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if ok, _ := cache.Get(key, 0, &got); ok {
		t.Error("entry survived Clear")
	}
}

// The period is part of what a fetch returns, so it has to be part of the key.
// Without it an hourly run would be served a cached daily result.
func TestCacheKeyIncludesPeriod(t *testing.T) {
	daily := cacheKey("quote", "binance", "BTCUSDT", "2024-01-01", "2024-06-30", "d")
	hourly := cacheKey("quote", "binance", "BTCUSDT", "2024-01-01", "2024-06-30", "1h")
	if daily == hourly {
		t.Error("daily and hourly fetches share a cache key")
	}

	// The same guarantee for every other component.
	base := []string{"quote", "binance", "BTCUSDT", "2024-01-01", "2024-06-30", "d"}
	seen := map[string]int{cacheKey(base...): -1}
	for i := range base {
		changed := append([]string{}, base...)
		changed[i] += "x"
		key := cacheKey(changed...)
		if prev, dup := seen[key]; dup {
			t.Errorf("component %d collides with component %d", i, prev)
		}
		seen[key] = i
	}
}

func TestCacheTTL(t *testing.T) {
	past := cacheTTL("2020-01-01")
	if past != 0 {
		t.Errorf("a historical range should never expire, got %v", past)
	}
	// Today's bar is still forming. This must hold in every timezone, including
	// the evening in one behind UTC, when the UTC date has already rolled over.
	if cacheTTL(time.Now().Format(DateLayout)) == 0 {
		t.Error("a range ending today should expire")
	}
	if cacheTTL(time.Now().AddDate(0, 0, 1).Format(DateLayout)) == 0 {
		t.Error("a range ending in the future should expire")
	}
	if cacheTTL("garbage") == 0 {
		t.Error("an unparseable date should expire rather than cache forever")
	}
}

// A nil cache is the "-no-cache" path and must be safe to use.
func TestNilCacheIsInert(t *testing.T) {
	var cache *Cache
	if ok, err := cache.Get("key", 0, &[]string{}); ok || err != nil {
		t.Errorf("nil cache Get returned ok=%v err=%v", ok, err)
	}
	if err := cache.Put("key", []string{"x"}); err != nil {
		t.Errorf("nil cache Put returned %v", err)
	}
	if err := cache.Clear(); err != nil {
		t.Errorf("nil cache Clear returned %v", err)
	}
}
