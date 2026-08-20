package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/markcheno/go-quote"
)

// Fetcher retrieves OHLCV data for one ticker. It exists so the pipeline can be
// exercised without network access.
type Fetcher interface {
	Fetch(ctx context.Context, cfg *Config, ticker string) (quote.Quote, error)
}

// MarketLister resolves a market name to its constituent symbols.
type MarketLister interface {
	Market(ctx context.Context, market string) ([]string, error)
}

// FetchRetries is how many times a transport error, 429 or 5xx is retried
// before the ticker is recorded as failed.
const FetchRetries = 2

// QuoteFetcher fetches from the configured upstream data source, optionally
// backed by an on-disk cache.
type QuoteFetcher struct {
	Cache  *Cache
	client *quote.Client
}

// NewQuoteFetcher returns a fetcher using the given cache. A nil cache disables
// caching.
func NewQuoteFetcher(cache *Cache) *QuoteFetcher {
	return &QuoteFetcher{
		Cache: cache,
		// One client for every request, so connections are pooled.
		client: &quote.Client{
			UserAgent: "markcheno/go-scan",
			Retry:     quote.RetryPolicy{Max: FetchRetries},
		},
	}
}

// Fetch retrieves one ticker, consulting the cache first.
func (f *QuoteFetcher) Fetch(ctx context.Context, cfg *Config, ticker string) (quote.Quote, error) {
	key := cacheKey("quote", cfg.Source, ticker, cfg.StartDate, cfg.EndDate, cfg.Period)

	if f.Cache != nil {
		var q quote.Quote
		if ok, err := f.Cache.Get(key, cacheTTL(cfg.EndDate), &q); err != nil {
			return quote.Quote{}, err
		} else if ok {
			return q, nil
		}
	}

	if err := ctx.Err(); err != nil {
		return quote.Quote{}, err
	}

	q, err := f.fetchUpstream(ctx, cfg, ticker)
	if err != nil {
		return quote.Quote{}, err
	}
	if len(q.Date) == 0 {
		return q, fmt.Errorf("no data returned for %s between %s and %s", ticker, cfg.StartDate, cfg.EndDate)
	}

	if f.Cache != nil {
		if err := f.Cache.Put(key, q); err != nil {
			return q, nil // a cache write failure must not fail the fetch
		}
	}
	return q, nil
}

// fetchUpstream resolves the source through go-quote's provider registry, so a
// provider added there needs no change here.
func (f *QuoteFetcher) fetchUpstream(ctx context.Context, cfg *Config, ticker string) (quote.Quote, error) {
	provider, err := f.client.Provider(cfg.Source)
	if err != nil {
		return quote.Quote{}, err
	}
	req, err := quoteRequest(cfg, ticker)
	if err != nil {
		return quote.Quote{}, err
	}
	if err := quote.CheckPeriod(provider, req.Period); err != nil {
		return quote.Quote{}, err
	}
	return provider.Fetch(ctx, req)
}

// quoteRequest converts a Config into a go-quote request. Validate has already
// checked the dates and period, so the parses here are belt and braces.
func quoteRequest(cfg *Config, ticker string) (quote.Request, error) {
	from, err := time.Parse(DateLayout, cfg.StartDate)
	if err != nil {
		return quote.Request{}, fmt.Errorf("start_date: %w", err)
	}
	to, err := time.Parse(DateLayout, cfg.EndDate)
	if err != nil {
		return quote.Request{}, fmt.Errorf("end_date: %w", err)
	}
	period, err := ParsePeriod(cfg.Period)
	if err != nil {
		return quote.Request{}, err
	}
	return quote.Request{
		Symbol: ticker,
		From:   from,
		To:     to,
		Period: period,
		Token:  cfg.TiingoToken,
	}, nil
}

// Market resolves a market name to its symbols, caching the result for a day.
func (f *QuoteFetcher) Market(ctx context.Context, market string) ([]string, error) {
	key := cacheKey("market", market)

	if f.Cache != nil {
		var symbols []string
		if ok, err := f.Cache.Get(key, 24*time.Hour, &symbols); err != nil {
			return nil, err
		} else if ok {
			return symbols, nil
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	symbols, err := f.client.MarketList(ctx, market)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch market %s: %w", market, err)
	}
	if len(symbols) == 0 {
		return nil, fmt.Errorf("market %s resolved to no symbols", market)
	}

	if f.Cache != nil {
		_ = f.Cache.Put(key, symbols)
	}
	return symbols, nil
}

// cacheTTL returns how long a fetch for the given end date stays fresh. A range
// that ends today is still moving; anything historical never changes.
//
// Both sides are parsed as bare dates so this is a calendar comparison. Doing
// it on instants would misjudge any timezone whose local date differs from
// UTC's, caching today's still-forming bar forever.
func cacheTTL(endDate string) time.Duration {
	end, err := time.Parse(DateLayout, endDate)
	if err != nil {
		return time.Hour
	}
	today, err := time.Parse(DateLayout, time.Now().Format(DateLayout))
	if err != nil {
		return time.Hour
	}
	if end.Before(today) {
		return 0 // never expires
	}
	return time.Hour
}

func cacheKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// Cache is a small JSON file cache for fetched data.
type Cache struct {
	dir string
	mu  sync.Mutex
}

// DefaultCacheDir returns the per-user cache location.
func DefaultCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "go-scan")
	}
	return filepath.Join(base, "go-scan")
}

// NewCache opens (and creates) a cache rooted at dir.
func NewCache(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory %s: %w", dir, err)
	}
	return &Cache{dir: dir}, nil
}

// Dir returns the cache root.
func (c *Cache) Dir() string { return c.dir }

func (c *Cache) path(key string) string {
	return filepath.Join(c.dir, key[:2], key+".json")
}

// Get decodes a cached entry into out. It reports false when the entry is
// missing or older than ttl; a ttl of 0 means entries never expire.
func (c *Cache) Get(key string, ttl time.Duration, out any) (bool, error) {
	if c == nil {
		return false, nil
	}
	path := c.path(key)
	info, err := os.Stat(path)
	if err != nil {
		return false, nil
	}
	if ttl > 0 && time.Since(info.ModTime()) > ttl {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		// A corrupt entry is not fatal; drop it and refetch.
		_ = os.Remove(path)
		return false, nil
	}
	return true, nil
}

// Put stores a value under key.
func (c *Cache) Put(key string, value any) error {
	if c == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	path := c.path(key)

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Clear removes every cached entry.
func (c *Cache) Clear() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(c.dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
