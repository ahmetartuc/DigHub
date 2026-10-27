package config

import (
	"errors"
	"fmt"
	"net/url"
)

type Config struct {
	Platform     string
	BaseURL      string
	Organization string
	User         string
	Token        string

	OutputFormat string
	OutputFile   string

	IncludeDorks []string
	ExcludeDorks []string
	Priority     string

	Workers   int
	RateLimit int
	Delay     int
	MaxPages  int

	Verbose      bool
	Quiet        bool
	NoColor      bool
	SaveProgress bool
	ProgressFile string
}

func (c *Config) Validate() error {
	if c.Platform != "github" && c.Platform != "gitlab" {
		return fmt.Errorf("invalid platform: %s (valid: github, gitlab)", c.Platform)
	}

	if c.BaseURL != "" {
		u, err := url.Parse(c.BaseURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fmt.Errorf("invalid --url: %s (expected e.g. https://gitlab.example.com)", c.BaseURL)
		}
	}

	if c.Organization == "" && c.User == "" {
		return errors.New("either --org or --user must be provided")
	}

	if c.Organization != "" && c.User != "" {
		return errors.New("cannot use both --org and --user at the same time")
	}

	if c.Token == "" {
		return errors.New("--token is required")
	}

	validFormats := map[string]bool{
		"terminal": true,
		"json":     true,
		"csv":      true,
		"html":     true,
	}
	if !validFormats[c.OutputFormat] {
		return fmt.Errorf("invalid output format: %s (valid: terminal, json, csv, html)", c.OutputFormat)
	}

	validPriorities := map[string]bool{
		"all":    true,
		"high":   true,
		"medium": true,
		"low":    true,
	}
	if !validPriorities[c.Priority] {
		return fmt.Errorf("invalid priority: %s (valid: all, high, medium, low)", c.Priority)
	}

	if c.Workers < 1 || c.Workers > 20 {
		return errors.New("workers must be between 1 and 20")
	}

	if c.RateLimit < 0 || c.RateLimit > 600 {
		return errors.New("rate-limit must be between 0 (platform default) and 600")
	}

	if c.MaxPages < 1 || c.MaxPages > 10 {
		return errors.New("max-pages must be between 1 and 10")
	}

	if c.Delay < 0 || c.Delay > 10 {
		return errors.New("delay must be between 0 and 10 seconds")
	}

	if c.OutputFile == "" && c.OutputFormat != "terminal" {
		c.OutputFile = fmt.Sprintf("./dighub-results.%s", c.OutputFormat)
	}

	return nil
}

func (c *Config) GetTarget() string {
	if c.Organization != "" {
		return c.Organization
	}
	return c.User
}

// GetTargetType returns "org" or "user". On GitLab an org is a group.
func (c *Config) GetTargetType() string {
	if c.Organization != "" {
		return "org"
	}
	return "user"
}
