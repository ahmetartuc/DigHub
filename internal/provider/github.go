package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	// GitHub's code search API allows 10 requests per minute per token
	githubDefaultRPM = 10
	githubPerPage    = 100
	// The search API never returns more than 1000 results per query
	githubMaxResults = 1000
)

type github struct {
	client   *client
	apiURL   string
	scope    string
	maxPages int
}

type githubSearchResponse struct {
	TotalCount int `json:"total_count"`
	Items      []struct {
		Path       string `json:"path"`
		HTMLURL    string `json:"html_url"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Score       float64 `json:"score"`
		TextMatches []struct {
			Fragment string `json:"fragment"`
		} `json:"text_matches"`
	} `json:"items"`
}

func newGitHub(opts Options) *github {
	apiURL := "https://api.github.com"
	if opts.BaseURL != "" {
		// GitHub Enterprise Server serves the API under /api/v3
		apiURL = strings.TrimRight(opts.BaseURL, "/") + "/api/v3"
	}

	header := http.Header{}
	header.Set("Authorization", "token "+opts.Token)
	// text-match returns the matched lines along with each file
	header.Set("Accept", "application/vnd.github.text-match+json")
	header.Set("X-GitHub-Api-Version", "2022-11-28")

	return &github{
		client:   newClient(interval(opts.RateLimit, githubDefaultRPM), header, opts.Logger),
		apiURL:   apiURL,
		scope:    fmt.Sprintf("%s:%s", opts.TargetType, opts.Target),
		maxPages: opts.MaxPages,
	}
}

func (g *github) Name() string { return "GitHub" }

func (g *github) Search(ctx context.Context, pattern string) (*Result, error) {
	query := url.QueryEscape(g.scope + " " + pattern)
	result := &Result{}
	total := 0

	for page := 1; page <= g.maxPages; page++ {
		endpoint := fmt.Sprintf("%s/search/code?q=%s&per_page=%d&page=%d", g.apiURL, query, githubPerPage, page)

		var resp githubSearchResponse
		if _, err := g.client.getJSON(ctx, endpoint, &resp); err != nil {
			return nil, err
		}
		total = resp.TotalCount

		for _, item := range resp.Items {
			hit := Hit{
				URL:        item.HTMLURL,
				Repository: item.Repository.FullName,
				Path:       item.Path,
				Score:      item.Score,
			}
			for _, tm := range item.TextMatches {
				hit.Fragments = append(hit.Fragments, tm.Fragment)
			}
			result.Hits = append(result.Hits, hit)
		}

		if len(resp.Items) < githubPerPage || len(result.Hits) >= total || page*githubPerPage >= githubMaxResults {
			break
		}
	}

	result.Truncated = total > len(result.Hits)
	return result, nil
}
