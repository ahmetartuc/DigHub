package scanner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ahmetartuc/dighub/v2/internal/config"
	"github.com/ahmetartuc/dighub/v2/internal/dorks"
	"github.com/ahmetartuc/dighub/v2/internal/logger"
	"github.com/ahmetartuc/dighub/v2/internal/provider"
	"github.com/schollz/progressbar/v3"
)

// Scanner handles the code search operations
type Scanner struct {
	config   *config.Config
	logger   *logger.Logger
	provider provider.Provider
	bar      *progressbar.ProgressBar
}

// Match represents a single search result
type Match struct {
	Dork       dorks.Dork
	URL        string
	Repository string
	Path       string
	Score      float64
	Fragments  []string
	Timestamp  time.Time
}

// SkippedDork is a dork whose search failed, so its results are unknown
type SkippedDork struct {
	Pattern string
	Reason  string
}

// ScanResults holds the aggregated scan results
type ScanResults struct {
	Matches        []Match
	Skipped        []SkippedDork
	TotalDorks     int
	TotalMatches   int
	UniqueFiles    int
	HighPriority   int
	MediumPriority int
	LowPriority    int
	// Truncated counts dorks that had more results than were fetched
	Truncated  int
	Duration   string
	OutputFile string
	Target     string
	Platform   string
	ScanDate   time.Time
}

// New creates a new scanner instance
func New(cfg *config.Config, log *logger.Logger) (*Scanner, error) {
	p, err := provider.New(cfg.Platform, provider.Options{
		BaseURL:    cfg.BaseURL,
		Token:      cfg.Token,
		Target:     cfg.GetTarget(),
		TargetType: cfg.GetTargetType(),
		MaxPages:   cfg.MaxPages,
		RateLimit:  cfg.RateLimit,
		Logger:     log,
	})
	if err != nil {
		return nil, err
	}
	return NewWithProvider(cfg, log, p), nil
}

// NewWithProvider creates a scanner that searches through the given provider
func NewWithProvider(cfg *config.Config, log *logger.Logger, p provider.Provider) *Scanner {
	return &Scanner{
		config:   cfg,
		logger:   log,
		provider: p,
	}
}

// Scan performs the complete scanning operation
func (s *Scanner) Scan() (*ScanResults, error) {
	startTime := time.Now()

	// Get and filter dorks
	allDorks := dorks.GetDorks()
	filteredDorks := s.filterDorks(allDorks)

	if len(filteredDorks) == 0 {
		return nil, fmt.Errorf("no dorks match the specified filters")
	}

	s.logger.Info("Loaded %d dorks for scanning", len(filteredDorks))

	// Initialize progress bar
	if !s.config.Quiet {
		s.bar = progressbar.NewOptions(len(filteredDorks),
			progressbar.OptionSetDescription("Scanning"),
			progressbar.OptionSetWidth(40),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "█",
				SaucerPadding: "░",
				BarStart:      "[",
				BarEnd:        "]",
			}),
		)
	}

	// Scan with concurrency
	matches, skipped, truncated := s.scanConcurrent(filteredDorks)

	// Calculate statistics
	results := s.buildResults(matches, filteredDorks, startTime)
	results.Skipped = skipped
	results.Truncated = truncated

	return results, nil
}

// filterDorks applies all filters to the dork list
func (s *Scanner) filterDorks(allDorks []dorks.Dork) []dorks.Dork {
	filtered := allDorks

	// Filter by priority
	filtered = dorks.FilterByPriority(filtered, s.config.Priority)

	// Filter by include patterns
	filtered = dorks.FilterByInclude(filtered, s.config.IncludeDorks)

	// Filter by exclude patterns
	filtered = dorks.FilterByExclude(filtered, s.config.ExcludeDorks)

	return filtered
}

// scanConcurrent performs concurrent scanning with worker pool.
// Request pacing is handled by the provider, shared across all workers.
func (s *Scanner) scanConcurrent(dorkList []dorks.Dork) ([]Match, []SkippedDork, int) {
	var (
		matches   []Match
		skipped   []SkippedDork
		truncated int
		mu        sync.Mutex
		wg        sync.WaitGroup
	)

	ctx := context.Background()
	jobs := make(chan dorks.Dork, len(dorkList))

	for i := 0; i < s.config.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for dork := range jobs {
				dorkMatches, wasTruncated, err := s.searchDork(ctx, dork)

				mu.Lock()
				if err != nil {
					skipped = append(skipped, SkippedDork{Pattern: dork.Pattern, Reason: err.Error()})
				}
				if wasTruncated {
					truncated++
				}
				matches = append(matches, dorkMatches...)
				mu.Unlock()

				if s.bar != nil {
					s.bar.Add(1)
				}

				if s.config.Delay > 0 {
					time.Sleep(time.Duration(s.config.Delay) * time.Second)
				}
			}
		}()
	}

	for _, dork := range dorkList {
		jobs <- dork
	}
	close(jobs)

	wg.Wait()

	if s.bar != nil {
		s.bar.Finish()
		fmt.Println() // New line after progress bar
	}

	return matches, skipped, truncated
}

// searchDork performs a single dork search
func (s *Scanner) searchDork(ctx context.Context, dork dorks.Dork) ([]Match, bool, error) {
	s.logger.Debug("Searching %s: %s", s.provider.Name(), dork.Pattern)

	result, err := s.provider.Search(ctx, dork.Pattern)
	if err != nil {
		s.logger.Warning("Search failed, dork skipped: %s (%v)", dork.Pattern, err)
		return nil, false, err
	}

	if len(result.Hits) == 0 {
		s.logger.NoMatch(dork.Pattern)
		return nil, false, nil
	}

	matches := make([]Match, 0, len(result.Hits))
	urls := make([]string, 0, len(result.Hits))
	now := time.Now()

	for _, hit := range result.Hits {
		matches = append(matches, Match{
			Dork:       dork,
			URL:        hit.URL,
			Repository: hit.Repository,
			Path:       hit.Path,
			Score:      hit.Score,
			Fragments:  hit.Fragments,
			Timestamp:  now,
		})
		urls = append(urls, hit.URL)
	}

	s.logger.Match(dork.Pattern, len(matches), urls)
	if result.Truncated {
		s.logger.Warning("More results exist for %s than were fetched; raise --max-pages to see them", dork.Pattern)
	}

	return matches, result.Truncated, nil
}

// buildResults creates the final results structure
func (s *Scanner) buildResults(matches []Match, dorkList []dorks.Dork, startTime time.Time) *ScanResults {
	// Count unique files
	uniqueFiles := make(map[string]bool)
	for _, match := range matches {
		uniqueFiles[match.Repository+"/"+match.Path] = true
	}

	// Count by priority
	highCount := 0
	mediumCount := 0
	lowCount := 0

	for _, match := range matches {
		switch match.Dork.Priority {
		case dorks.PriorityHigh:
			highCount++
		case dorks.PriorityMedium:
			mediumCount++
		case dorks.PriorityLow:
			lowCount++
		}
	}

	return &ScanResults{
		Matches:        matches,
		TotalDorks:     len(dorkList),
		TotalMatches:   len(matches),
		UniqueFiles:    len(uniqueFiles),
		HighPriority:   highCount,
		MediumPriority: mediumCount,
		LowPriority:    lowCount,
		Duration:       time.Since(startTime).Round(time.Second).String(),
		Target:         s.config.GetTarget(),
		Platform:       s.provider.Name(),
		ScanDate:       time.Now(),
	}
}
