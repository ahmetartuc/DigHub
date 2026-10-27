package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ahmetartuc/dighub/internal/logger"
)

const maxAttempts = 5

// StatusError is returned for a non-retryable HTTP error response
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	body := strings.TrimSpace(e.Body)
	if len(body) > 200 {
		body = body[:200] + "..."
	}
	return fmt.Sprintf("API error %d: %s", e.Code, body)
}

// limiter spaces requests evenly across all workers sharing it
type limiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

// wait blocks until the caller may send its next request
func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	at := l.next
	if at.Before(now) {
		at = now
	}
	l.next = at.Add(l.interval)
	l.mu.Unlock()

	return sleep(ctx, time.Until(at))
}

// pauseUntil holds back every worker until t, after a rate limit response
func (l *limiter) pauseUntil(t time.Time) {
	l.mu.Lock()
	if t.After(l.next) {
		l.next = t
	}
	l.mu.Unlock()
}

// client is a rate-limited JSON API client shared by the providers
type client struct {
	http    *http.Client
	limiter *limiter
	header  http.Header
	logger  *logger.Logger
}

func newClient(every time.Duration, header http.Header, log *logger.Logger) *client {
	return &client{
		http:    &http.Client{Timeout: 30 * time.Second},
		limiter: &limiter{interval: every},
		header:  header,
		logger:  log,
	}
}

// getJSON fetches url into out, waiting out rate limits and retrying server errors.
// It returns the response headers so callers can read pagination info.
func (c *client) getJSON(ctx context.Context, url string, out interface{}) (http.Header, error) {
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := c.limiter.wait(ctx); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range c.header {
			req.Header[k] = v
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			c.debug("Request failed (attempt %d/%d): %v", attempt, maxAttempts, err)
			if err := sleep(ctx, backoff(attempt)); err != nil {
				return nil, err
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			if err := json.Unmarshal(body, out); err != nil {
				return nil, fmt.Errorf("decode response: %w", err)
			}
			return resp.Header, nil

		case isRateLimited(resp, body):
			until := resetTime(resp.Header)
			c.limiter.pauseUntil(until)
			if c.logger != nil {
				c.logger.RateLimit(until, time.Until(until))
			}
			lastErr = fmt.Errorf("rate limited")

		case resp.StatusCode >= 500:
			lastErr = &StatusError{Code: resp.StatusCode, Body: string(body)}
			c.debug("Server error (attempt %d/%d): %v", attempt, maxAttempts, lastErr)
			if err := sleep(ctx, backoff(attempt)); err != nil {
				return nil, err
			}

		default:
			return nil, &StatusError{Code: resp.StatusCode, Body: string(body)}
		}
	}

	return nil, fmt.Errorf("giving up after %d attempts: %w", maxAttempts, lastErr)
}

func (c *client) debug(format string, args ...interface{}) {
	if c.logger != nil {
		c.logger.Debug(format, args...)
	}
}

// isRateLimited recognises both primary and secondary rate limit responses.
// GitHub answers these with 403 or 429; GitLab with 429.
func isRateLimited(resp *http.Response, body []byte) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	if resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	return strings.Contains(strings.ToLower(string(body)), "rate limit")
}

// resetTime works out when a rate-limited request may be retried
func resetTime(h http.Header) time.Time {
	if s := h.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil {
			return time.Now().Add(time.Duration(secs) * time.Second)
		}
	}
	for _, key := range []string{"X-RateLimit-Reset", "RateLimit-Reset"} {
		if s := h.Get(key); s != "" {
			if unix, err := strconv.ParseInt(s, 10, 64); err == nil {
				if t := time.Unix(unix, 0).Add(time.Second); t.After(time.Now()) {
					return t
				}
			}
		}
	}
	return time.Now().Add(60 * time.Second)
}

func backoff(attempt int) time.Duration {
	return time.Duration(1<<uint(attempt-1)) * time.Second
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
