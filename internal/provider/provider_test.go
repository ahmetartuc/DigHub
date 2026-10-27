package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fastOpts keeps the limiter out of the way so tests run quickly
func fastOpts(baseURL, targetType, target string) Options {
	return Options{
		BaseURL:    baseURL,
		Token:      "test-token",
		Target:     target,
		TargetType: targetType,
		MaxPages:   1,
		RateLimit:  6000,
	}
}

func TestLimiterSpacesRequests(t *testing.T) {
	l := &limiter{interval: 50 * time.Millisecond}
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := l.wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("3 requests took %v, want at least 100ms", elapsed)
	}
}

func TestIsRateLimited(t *testing.T) {
	cases := []struct {
		name   string
		code   int
		header map[string]string
		body   string
		want   bool
	}{
		{"429", 429, nil, "", true},
		{"403 retry-after", 403, map[string]string{"Retry-After": "30"}, "", true},
		{"403 remaining zero", 403, map[string]string{"X-RateLimit-Remaining": "0"}, "", true},
		{"403 secondary limit body", 403, nil, `{"message":"You have exceeded a secondary rate limit"}`, true},
		{"403 forbidden", 403, nil, `{"message":"Resource not accessible"}`, false},
		{"404", 404, nil, "", false},
	}
	for _, c := range cases {
		resp := &http.Response{StatusCode: c.code, Header: http.Header{}}
		for k, v := range c.header {
			resp.Header.Set(k, v)
		}
		if got := isRateLimited(resp, []byte(c.body)); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func githubItems(n, offset int) string {
	items := make([]string, n)
	for i := range items {
		items[i] = fmt.Sprintf(`{"path":"f%d.env","html_url":"https://github.com/acme/repo/blob/main/f%d.env",
			"repository":{"full_name":"acme/repo"},"score":1,"text_matches":[{"fragment":"KEY=%d"}]}`,
			offset+i, offset+i, offset+i)
	}
	return strings.Join(items, ",")
}

func TestGitHubPaginatesAndReadsFragments(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/search/code" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); !strings.Contains(got, "text-match") {
			t.Errorf("Accept header %q does not request text matches", got)
		}
		queries = append(queries, r.URL.Query().Get("q"))
		switch r.URL.Query().Get("page") {
		case "1":
			fmt.Fprintf(w, `{"total_count":150,"items":[%s]}`, githubItems(100, 0))
		case "2":
			fmt.Fprintf(w, `{"total_count":150,"items":[%s]}`, githubItems(50, 100))
		default:
			t.Errorf("unexpected page %s", r.URL.Query().Get("page"))
		}
	}))
	defer srv.Close()

	opts := fastOpts(srv.URL, "org", "acme")
	opts.MaxPages = 5
	res, err := New("github", opts)
	if err != nil {
		t.Fatal(err)
	}
	result, err := res.Search(context.Background(), "filename:.env KEY")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Hits) != 150 {
		t.Fatalf("got %d hits, want 150", len(result.Hits))
	}
	if result.Truncated {
		t.Error("result marked truncated although every match was fetched")
	}
	if got := result.Hits[0].Fragments; len(got) != 1 || got[0] != "KEY=0" {
		t.Errorf("fragments = %v, want [KEY=0]", got)
	}
	if len(queries) != 2 || queries[0] != "org:acme filename:.env KEY" {
		t.Errorf("queries = %q", queries)
	}
}

func TestGitHubMarksTruncatedResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"total_count":250,"items":[%s]}`, githubItems(100, 0))
	}))
	defer srv.Close()

	p, _ := New("github", fastOpts(srv.URL, "user", "jdoe"))
	result, err := p.Search(context.Background(), "ghp_")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated {
		t.Error("expected truncated result when total_count exceeds fetched hits")
	}
}

func TestGitHubWaitsOutRateLimit(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprintf(w, `{"total_count":1,"items":[%s]}`, githubItems(1, 0))
	}))
	defer srv.Close()

	p, _ := New("github", fastOpts(srv.URL, "org", "acme"))
	start := time.Now()
	result, err := p.Search(context.Background(), "ghp_")
	if err != nil {
		t.Fatalf("search should succeed after the rate limit clears: %v", err)
	}
	if len(result.Hits) != 1 || calls != 2 {
		t.Errorf("hits=%d calls=%d, want 1 hit after 2 calls", len(result.Hits), calls)
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Error("did not honour Retry-After")
	}
}

func TestGitHubFailsFastOnForbidden(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Resource not accessible by personal access token"}`)
	}))
	defer srv.Close()

	p, _ := New("github", fastOpts(srv.URL, "org", "acme"))
	_, err := p.Search(context.Background(), "ghp_")

	var se *StatusError
	if !errors.As(err, &se) || se.Code != 403 {
		t.Fatalf("err = %v, want StatusError 403", err)
	}
	if calls != 1 {
		t.Errorf("made %d calls, want 1 (no retry on a plain 403)", calls)
	}
}

func TestGitLabGroupSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "test-token" {
			t.Errorf("missing PRIVATE-TOKEN header")
		}
		switch r.URL.Path {
		case "/api/v4/groups/acme/platform/search":
			if r.URL.Query().Get("search") == "dighub" {
				fmt.Fprint(w, `[]`)
				return
			}
			fmt.Fprint(w, `[{"path":"config/app settings.env","data":"SECRET_KEY=abc\n","ref":"main","startline":4,"project_id":7}]`)
		case "/api/v4/projects/7":
			fmt.Fprint(w, `{"id":7,"path_with_namespace":"acme/platform/api","web_url":"https://gitlab.acme.test/acme/platform/api"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	p, _ := New("gitlab", fastOpts(srv.URL, "org", "acme/platform"))
	result, err := p.Search(context.Background(), "filename:.env SECRET_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(result.Hits))
	}
	hit := result.Hits[0]
	wantURL := "https://gitlab.acme.test/acme/platform/api/-/blob/main/config/app%20settings.env#L4"
	if hit.URL != wantURL {
		t.Errorf("URL = %s, want %s", hit.URL, wantURL)
	}
	if hit.Repository != "acme/platform/api" || hit.Fragments[0] != "SECRET_KEY=abc\n" {
		t.Errorf("hit = %+v", hit)
	}
}

func TestGitLabFallsBackToProjectSearch(t *testing.T) {
	var projectSearches int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/groups/acme/search":
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"message":"400 Bad request - Scope supported only with advanced search"}`)
		case "/api/v4/groups/acme/projects":
			if r.URL.Query().Get("include_subgroups") != "true" {
				t.Error("subgroup projects not requested")
			}
			fmt.Fprint(w, `[{"id":1,"path_with_namespace":"acme/empty","web_url":"https://gl.test/acme/empty"},
				{"id":2,"path_with_namespace":"acme/app","web_url":"https://gl.test/acme/app"}]`)
		case "/api/v4/projects/1/search":
			atomic.AddInt32(&projectSearches, 1)
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"404 Repository Not Found"}`)
		case "/api/v4/projects/2/search":
			atomic.AddInt32(&projectSearches, 1)
			fmt.Fprint(w, `[{"path":".env","data":"DB_PASSWORD=x","ref":"main","startline":1,"project_id":2}]`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	p, _ := New("gitlab", fastOpts(srv.URL, "org", "acme"))
	result, err := p.Search(context.Background(), "DB_PASSWORD")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].URL != "https://gl.test/acme/app/-/blob/main/.env#L1" {
		t.Fatalf("hits = %+v", result.Hits)
	}
	if projectSearches != 2 {
		t.Errorf("searched %d projects, want 2", projectSearches)
	}
}

func TestGitLabGroupNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"404 Group Not Found"}`)
	}))
	defer srv.Close()

	p, _ := New("gitlab", fastOpts(srv.URL, "org", "nope"))
	if _, err := p.Search(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v, want a 404 error", err)
	}
}
