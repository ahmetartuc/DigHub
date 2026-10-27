package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/ahmetartuc/dighub/v2/internal/logger"
)

// Hit is a single file matched by a search
type Hit struct {
	URL        string
	Repository string
	Path       string
	Score      float64
	Fragments  []string
}

// Result is the outcome of one dork search
type Result struct {
	Hits []Hit
	// Truncated is set when the platform had more matches than were fetched
	Truncated bool
}

// Provider searches code on one platform, scoped to a single target
type Provider interface {
	Name() string
	Search(ctx context.Context, pattern string) (*Result, error)
}

// Options configures a provider
type Options struct {
	// BaseURL is the instance URL for self-hosted installs; empty means the public service
	BaseURL    string
	Token      string
	Target     string
	TargetType string // "org" or "user"
	MaxPages   int
	// RateLimit is in requests per minute; 0 uses the platform default
	RateLimit int
	Logger    *logger.Logger
}

// Platforms lists the supported platform names
var Platforms = []string{"github", "gitlab"}

// New returns the provider for the given platform
func New(platform string, opts Options) (Provider, error) {
	if opts.MaxPages < 1 {
		opts.MaxPages = 1
	}

	switch platform {
	case "github":
		return newGitHub(opts), nil
	case "gitlab":
		return newGitLab(opts), nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}

// interval converts a requests-per-minute limit into the gap between requests
func interval(rpm, defaultRPM int) time.Duration {
	if rpm <= 0 {
		rpm = defaultRPM
	}
	return time.Minute / time.Duration(rpm)
}
