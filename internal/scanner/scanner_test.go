package scanner

import (
	"context"
	"errors"
	"testing"

	"github.com/ahmetartuc/dighub/internal/config"
	"github.com/ahmetartuc/dighub/internal/logger"
	"github.com/ahmetartuc/dighub/internal/provider"
)

type fakeProvider struct {
	fail map[string]bool
}

func (f *fakeProvider) Name() string { return "Fake" }

func (f *fakeProvider) Search(_ context.Context, pattern string) (*provider.Result, error) {
	if f.fail[pattern] {
		return nil, errors.New("boom")
	}
	return &provider.Result{
		Hits:      []provider.Hit{{URL: "https://example.test/" + pattern, Repository: "acme/repo", Path: pattern}},
		Truncated: pattern == "ghp_",
	}, nil
}

func TestScanReportsSkippedDorks(t *testing.T) {
	cfg := &config.Config{
		Organization: "acme",
		Priority:     "all",
		IncludeDorks: []string{"GitHub"},
		Workers:      3,
	}
	fp := &fakeProvider{fail: map[string]bool{"gho_": true}}
	s := NewWithProvider(cfg, logger.New(false, true), fp)

	results, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}

	if len(results.Skipped) != 1 || results.Skipped[0].Pattern != "gho_" || results.Skipped[0].Reason != "boom" {
		t.Errorf("skipped = %+v, want only gho_", results.Skipped)
	}
	if results.TotalMatches != results.TotalDorks-1 {
		t.Errorf("matches = %d, want one per dork except the skipped one (%d)", results.TotalMatches, results.TotalDorks-1)
	}
	if results.Truncated != 1 {
		t.Errorf("truncated = %d, want 1", results.Truncated)
	}
	if results.Platform != "Fake" {
		t.Errorf("platform = %q", results.Platform)
	}
}
