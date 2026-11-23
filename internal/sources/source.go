// Package sources contains the adapters for public data providers. Every
// adapter implements Source; shared HTTP, rate-limit and error handling lives
// in this file so adapters only deal with their provider's response format.
package sources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"sentrix/internal/buildinfo"
	"sentrix/pkg/models"
)

// Access describes how a provider can be used.
type Access string

const (
	// AccessPublic needs no credentials.
	AccessPublic Access = "public"
	// AccessOptionalKey works without credentials but gives better limits with one.
	AccessOptionalKey Access = "optional-key"
)

// Finding is one raw observation returned by a source, before normalization.
type Finding struct {
	Raw        string
	Kind       models.Kind
	Evidence   string
	ObservedAt time.Time // zero when the provider gives no timestamp
}

// Source is the contract every data-source adapter fulfils.
type Source interface {
	Name() string
	Description() string
	Access() Access
	// Available performs a lightweight request to check that the provider is
	// reachable. It is only called after the user has confirmed a scan, or
	// when the user explicitly asks for a live check.
	Available(ctx context.Context) error
	// Fetch retrieves findings for a root domain. On failure it returns any
	// findings gathered so far together with the error.
	Fetch(ctx context.Context, domain string) ([]Finding, error)
}

// Info is static, offline metadata about a source.
type Info struct {
	Name        string
	Description string
	Access      Access
	KeyEnv      string // environment variable that supplies an API key, if any
}

// Settings configures one adapter instance.
type Settings struct {
	BaseURL       string // overrides the provider URL (used by tests)
	APIKey        string
	Client        *http.Client
	RatePerSecond float64
	MaxResults    int
}

// Sentinel errors give every adapter the same failure vocabulary.
var (
	ErrRateLimited = errors.New("rate limited by provider")
	ErrUnavailable = errors.New("source unavailable")
	ErrParse       = errors.New("unexpected response format")
)

// StatusError is returned for non-retryable HTTP failures such as 403 or 404.
type StatusError struct {
	Status int
	URL    string
}

func (e *StatusError) Error() string { return fmt.Sprintf("HTTP %d from %s", e.Status, e.URL) }

// Classify maps an error to a stable short label used in run history.
func Classify(err error) string {
	var se *StatusError
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, ErrUnavailable):
		return "unavailable"
	case errors.Is(err, ErrParse):
		return "parse_error"
	case errors.As(err, &se):
		return "http_error"
	}
	return "error"
}

// Limiter spaces requests so a source never exceeds its configured rate.
// It is safe for concurrent use.
type Limiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

// NewLimiter returns a limiter allowing perSecond requests per second;
// a value <= 0 disables limiting.
func NewLimiter(perSecond float64) *Limiter {
	l := &Limiter{}
	if perSecond > 0 {
		l.interval = time.Duration(float64(time.Second) / perSecond)
	}
	return l
}

// Wait blocks until the caller may send a request or ctx is done.
func (l *Limiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	slot := l.next
	if slot.Before(now) {
		slot = now
	}
	l.next = slot.Add(l.interval)
	l.mu.Unlock()
	return sleep(ctx, time.Until(slot))
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

const (
	maxBody    = 64 << 20
	maxRetries = 2
	maxBackoff = 30 * time.Second
)

// base carries what every adapter needs for HTTP access.
type base struct {
	url    string
	apiKey string
	hc     *http.Client
	lim    *Limiter
	max    int
}

func newBase(defaultURL string, s Settings) base {
	b := base{url: defaultURL, apiKey: s.APIKey, hc: s.Client, lim: NewLimiter(s.RatePerSecond), max: s.MaxResults}
	if s.BaseURL != "" {
		b.url = s.BaseURL
	}
	if b.hc == nil {
		b.hc = &http.Client{Timeout: 60 * time.Second}
	}
	if b.max <= 0 {
		b.max = 5000
	}
	return b
}

// get performs a rate-limited GET, retrying politely on 429/503.
func (b base) get(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		if err := b.lim.Wait(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("User-Agent", buildinfo.UserAgent())
		req.Header.Set("Accept", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := b.hc.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
		resp.Body.Close()
		if rerr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("%w: reading body: %v", ErrUnavailable, rerr)
		}
		if len(body) > maxBody {
			return nil, fmt.Errorf("%w: response larger than %d bytes", ErrParse, maxBody)
		}

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return body, nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable:
			if attempt >= maxRetries {
				if resp.StatusCode == http.StatusTooManyRequests {
					return nil, ErrRateLimited
				}
				return nil, fmt.Errorf("%w: HTTP 503", ErrUnavailable)
			}
			if err := sleep(ctx, retryDelay(resp, attempt)); err != nil {
				return nil, err
			}
		case resp.StatusCode >= 500:
			return nil, fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
		default:
			return nil, &StatusError{Status: resp.StatusCode, URL: rawURL}
		}
	}
}

func retryDelay(resp *http.Response, attempt int) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return min(time.Duration(n)*time.Second, maxBackoff)
		}
	}
	return min(time.Duration(attempt+1)*2*time.Second, maxBackoff)
}

// ping is the shared Available implementation: any non-5xx, non-429 answer
// means the provider is up.
func (b base) ping(ctx context.Context, probeURL string, headers map[string]string) error {
	_, err := b.get(ctx, probeURL, headers)
	var se *StatusError
	if errors.As(err, &se) && se.Status < 500 && se.Status != http.StatusTooManyRequests {
		return nil
	}
	return err
}

func parseTimestamp(layout, s string) time.Time {
	t, err := time.Parse(layout, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
