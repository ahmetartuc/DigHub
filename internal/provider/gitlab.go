package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/ahmetartuc/dighub/v2/internal/logger"
)

const (
	// GitLab.com allows 30 search requests per minute; self-hosted limits vary
	gitlabDefaultRPM = 30
	gitlabPerPage    = 100
)

type gitlabProject struct {
	ID                int    `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url"`
}

type gitlabBlob struct {
	Path      string `json:"path"`
	Data      string `json:"data"`
	Ref       string `json:"ref"`
	Startline int    `json:"startline"`
	ProjectID int    `json:"project_id"`
}

// gitlab searches a group or user namespace on gitlab.com or a self-hosted instance.
//
// Group-wide blob search needs advanced search (Elasticsearch or Zoekt). When the
// instance does not have it, the provider falls back to searching each project,
// which works everywhere but costs one request per project per dork.
type gitlab struct {
	client     *client
	apiURL     string
	target     string
	targetType string
	maxPages   int
	logger     *logger.Logger

	setup     sync.Once
	setupErr  error
	perProj   bool
	projects  []gitlabProject
	projMu    sync.Mutex
	projCache map[int]gitlabProject
}

func newGitLab(opts Options) *gitlab {
	base := "https://gitlab.com"
	if opts.BaseURL != "" {
		base = strings.TrimRight(opts.BaseURL, "/")
	}

	header := http.Header{}
	header.Set("PRIVATE-TOKEN", opts.Token)

	return &gitlab{
		client:     newClient(interval(opts.RateLimit, gitlabDefaultRPM), header, opts.Logger),
		apiURL:     base + "/api/v4",
		target:     opts.Target,
		targetType: opts.TargetType,
		maxPages:   opts.MaxPages,
		logger:     opts.Logger,
		projCache:  map[int]gitlabProject{},
	}
}

func (g *gitlab) Name() string { return "GitLab" }

func (g *gitlab) Search(ctx context.Context, pattern string) (*Result, error) {
	g.setup.Do(func() { g.setupErr = g.prepare(ctx) })
	if g.setupErr != nil {
		return nil, g.setupErr
	}

	if !g.perProj {
		endpoint := fmt.Sprintf("%s/groups/%s/search?scope=blobs&search=%s",
			g.apiURL, url.PathEscape(g.target), url.QueryEscape(pattern))
		return g.searchPages(ctx, endpoint)
	}

	result := &Result{}
	for _, p := range g.projects {
		endpoint := fmt.Sprintf("%s/projects/%d/search?scope=blobs&search=%s",
			g.apiURL, p.ID, url.QueryEscape(pattern))
		r, err := g.searchPages(ctx, endpoint)
		if err != nil {
			// Empty or access-restricted repositories reject search; skip them
			var se *StatusError
			if errors.As(err, &se) && se.Code < 500 {
				g.debug("Skipping project %s: %v", p.PathWithNamespace, err)
				continue
			}
			return nil, err
		}
		result.Hits = append(result.Hits, r.Hits...)
		result.Truncated = result.Truncated || r.Truncated
	}
	return result, nil
}

// prepare decides between group-wide and per-project search
func (g *gitlab) prepare(ctx context.Context) error {
	if g.targetType == "org" {
		probe := fmt.Sprintf("%s/groups/%s/search?scope=blobs&search=dighub&per_page=1",
			g.apiURL, url.PathEscape(g.target))
		var blobs []gitlabBlob
		_, err := g.client.getJSON(ctx, probe, &blobs)
		if err == nil {
			g.info("GitLab group search available, using group-wide search")
			return nil
		}
		var se *StatusError
		if !errors.As(err, &se) || se.Code != http.StatusBadRequest {
			return fmt.Errorf("group %q: %w", g.target, err)
		}
		g.info("GitLab advanced search unavailable, falling back to per-project search")
	}

	g.perProj = true
	projects, err := g.listProjects(ctx)
	if err != nil {
		return err
	}
	g.projects = projects
	g.info("Found %d GitLab projects to search", len(projects))
	return nil
}

func (g *gitlab) listProjects(ctx context.Context) ([]gitlabProject, error) {
	var base string
	if g.targetType == "org" {
		base = fmt.Sprintf("%s/groups/%s/projects?include_subgroups=true&simple=true",
			g.apiURL, url.PathEscape(g.target))
	} else {
		base = fmt.Sprintf("%s/users/%s/projects?simple=true", g.apiURL, url.PathEscape(g.target))
	}

	var all []gitlabProject
	for page := 1; ; page++ {
		var projects []gitlabProject
		h, err := g.client.getJSON(ctx, fmt.Sprintf("%s&per_page=%d&page=%d", base, gitlabPerPage, page), &projects)
		if err != nil {
			return nil, fmt.Errorf("list projects of %q: %w", g.target, err)
		}
		all = append(all, projects...)
		g.cacheProjects(projects)
		if h.Get("X-Next-Page") == "" || len(projects) == 0 {
			return all, nil
		}
	}
}

// searchPages runs one blob search, following pagination up to maxPages
func (g *gitlab) searchPages(ctx context.Context, endpoint string) (*Result, error) {
	result := &Result{}

	for page := 1; page <= g.maxPages; page++ {
		var blobs []gitlabBlob
		h, err := g.client.getJSON(ctx, fmt.Sprintf("%s&per_page=%d&page=%d", endpoint, gitlabPerPage, page), &blobs)
		if err != nil {
			return nil, err
		}

		for _, b := range blobs {
			hit, err := g.toHit(ctx, b)
			if err != nil {
				return nil, err
			}
			result.Hits = append(result.Hits, hit)
		}

		if h.Get("X-Next-Page") == "" || len(blobs) == 0 {
			return result, nil
		}
	}

	result.Truncated = true
	return result, nil
}

func (g *gitlab) toHit(ctx context.Context, b gitlabBlob) (Hit, error) {
	p, err := g.project(ctx, b.ProjectID)
	if err != nil {
		return Hit{}, err
	}

	segments := strings.Split(b.Path, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	link := fmt.Sprintf("%s/-/blob/%s/%s", p.WebURL, url.PathEscape(b.Ref), strings.Join(segments, "/"))
	if b.Startline > 0 {
		link += fmt.Sprintf("#L%d", b.Startline)
	}

	hit := Hit{URL: link, Repository: p.PathWithNamespace, Path: b.Path}
	if b.Data != "" {
		hit.Fragments = []string{b.Data}
	}
	return hit, nil
}

// project resolves a project ID to its path, fetching it once if not yet known
func (g *gitlab) project(ctx context.Context, id int) (gitlabProject, error) {
	g.projMu.Lock()
	p, ok := g.projCache[id]
	g.projMu.Unlock()
	if ok {
		return p, nil
	}

	if _, err := g.client.getJSON(ctx, fmt.Sprintf("%s/projects/%d?simple=true", g.apiURL, id), &p); err != nil {
		return gitlabProject{}, fmt.Errorf("look up project %d: %w", id, err)
	}
	g.cacheProjects([]gitlabProject{p})
	return p, nil
}

func (g *gitlab) cacheProjects(projects []gitlabProject) {
	g.projMu.Lock()
	for _, p := range projects {
		g.projCache[p.ID] = p
	}
	g.projMu.Unlock()
}

func (g *gitlab) info(format string, args ...interface{}) {
	if g.logger != nil {
		g.logger.Info(format, args...)
	}
}

func (g *gitlab) debug(format string, args ...interface{}) {
	if g.logger != nil {
		g.logger.Debug(format, args...)
	}
}
