// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package issuewatch

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/margince/margince/backend/pkg/extension"
)

// The open issues of the product's own tracker, read and never written: the
// watcher shows a developer how many are known about the screen they are on.
const (
	githubRepo = "margince/margince"
	// githubEvery is how old the copy may be before a reader refreshes it. The
	// repository is read without a token, which GitHub allows sixty requests an
	// hour; a refresh takes one request per hundred issues.
	githubEvery     = 15 * time.Minute
	githubPageSize  = 100
	githubMaxPages  = 20
	githubTimeout   = 30 * time.Second
	githubMaxBody   = 8000
	maxGithubErrors = 300
	// An issue about this many screens or more is about the app, not a screen:
	// a release checklist names every one.
	acrossScreens = 4
)

// offScreenLabels mark issues no screen shows, like a failing pipeline's.
var offScreenLabels = []string{"area: ci-tests"}

// screen is a kind of page and what puts an issue on it: a path it names, a
// phrase it says, or a label that covers the screen and every screen below it.
type screen struct {
	Pattern string
	// Paths are what an issue names when it means this screen; the screen's own
	// pattern is always one. The API path behind a screen counts.
	Paths   []string
	Phrases []string
	Labels  []string
}

// screens are the app's screens. Phrases are lower case and match whole words.
var screens = []screen{
	{Pattern: "/home", Paths: []string{"/worklist"}, Phrases: []string{"worklist", "home screen", "home page"}},
	{Pattern: "/deals", Phrases: []string{"deals table", "deals list", "deal list", "deals page", "deals screen", "pipeline board", "deal board", "kanban"}, Labels: []string{"area: deals"}},
	{Pattern: "/deals/:id", Phrases: []string{"deal page", "deal screen", "deal detail", "deal details", "on a deal", "deal room"}},
	{Pattern: "/contacts", Phrases: []string{"contacts list", "contact list", "contacts table", "contacts page", "contacts screen"}},
	{Pattern: "/contacts/:id", Phrases: []string{"contact page", "contact screen", "contact detail", "contact details", "on a contact"}},
	{Pattern: "/companies", Phrases: []string{"companies list", "company list", "companies table", "companies page", "companies screen"}},
	{Pattern: "/companies/:id", Phrases: []string{"company page", "company screen", "company detail", "company details", "on a company"}},
	{Pattern: "/leads", Phrases: []string{"leads list", "lead list", "leads page", "leads screen", "leads inbox", "lead inbox"}},
	{Pattern: "/leads/:id", Phrases: []string{"lead page", "lead screen", "lead detail", "lead details", "on a lead"}},
	{Pattern: "/projects/:id", Phrases: []string{"project page", "project screen"}},
	{Pattern: "/analytics", Phrases: []string{"analytics", "report builder", "reports page", "forecast"}, Labels: []string{"area: reports"}},
	{Pattern: "/settings", Phrases: []string{"settings overview"}},
	{Pattern: "/settings/connections", Phrases: []string{"connections settings", "connections page", "connections screen", "settings, connections"}},
	{Pattern: "/settings/account", Phrases: []string{"account settings", "account page"}},
	{Pattern: "/settings/company", Phrases: []string{"company profile", "company settings"}},
	{Pattern: "/settings/roles", Phrases: []string{"roles settings", "settings, roles"}},
}

// githubIssue is one open issue as the copy keeps it.
type githubIssue struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	URL    string   `json:"url"`
	Labels []string `json:"labels"`
	Body   string   `json:"-"`
	// Why says what put it on the screen asked about.
	Why string `json:"why"`
}

// patternOfPage is the kind of page a route is: every deal is /deals/:id. An
// id in braces or after a colon, as an issue writes one, is an id too.
func patternOfPage(page string) string {
	id := regexp.MustCompile(`^(\{[^/]*\}|:[A-Za-z_]+|<[^/]*>|` + idPart + `)$`)
	segments := strings.Split(page, "/")
	for i, segment := range segments {
		if id.MatchString(segment) {
			segments[i] = ":id"
		}
	}
	return strings.Join(segments, "/")
}

// under says whether path is at or below prefix.
func under(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// screenOf is the screen a page is on: the most specific one it sits under,
// or the page's own pattern when no screen is known.
func screenOf(page string) string {
	pattern := patternOfPage(page)
	best := ""
	for _, s := range screens {
		if under(pattern, s.Pattern) && len(s.Pattern) > len(best) {
			best = s.Pattern
		}
	}
	return cmp.Or(best, pattern)
}

// screenNamed is the screen a path an issue names is about, or "".
func screenNamed(path string) string {
	best, bestLen := "", 0
	for _, s := range screens {
		for _, p := range append([]string{s.Pattern}, s.Paths...) {
			if under(path, p) && len(p) > bestLen {
				best, bestLen = s.Pattern, len(p)
			}
		}
	}
	return best
}

// pathsIn are the app paths a text names, as patterns: "/v1/deals/{id}" is
// "/deals/:id". A path inside a word, like a source file's, is not one.
func pathsIn(text string) []string {
	found := regexp.MustCompile(`(?:^|[^\w./-])(/[a-z][a-z0-9-]*(?:/[A-Za-z0-9{}:<>_-]+)*)`).FindAllStringSubmatch(text, -1)
	paths := make([]string, 0, len(found))
	for _, f := range found {
		path := strings.TrimPrefix(f[1], "/v1")
		if path == "" {
			continue
		}
		paths = append(paths, patternOfPage(path))
	}
	return paths
}

// says whether text holds phrase as whole words. Both are lower case.
func says(text, phrase string) bool {
	for from := 0; ; {
		at := strings.Index(text[from:], phrase)
		if at < 0 {
			return false
		}
		start, end := from+at, from+at+len(phrase)
		if !wordAt(text, start-1) && !wordAt(text, end) {
			return true
		}
		from = start + 1
	}
}

func wordAt(text string, i int) bool {
	if i < 0 || i >= len(text) {
		return false
	}
	r := rune(text[i])
	return r >= 0x80 || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// matchScreen says why an issue is on a screen, or "" when it is not: a path
// it names first, then a phrase it says, then a label covering the screen.
func matchScreen(i githubIssue, on string) string {
	text := i.Title + "\n" + i.Body
	for _, path := range pathsIn(text) {
		if screenNamed(path) == on {
			return "names " + path
		}
	}
	lower := strings.ToLower(text)
	for _, s := range screens {
		if s.Pattern != on {
			continue
		}
		for _, phrase := range s.Phrases {
			if says(lower, phrase) {
				return fmt.Sprintf("says %q", phrase)
			}
		}
	}
	for _, s := range screens {
		if !under(on, s.Pattern) {
			continue
		}
		for _, label := range s.Labels {
			if slices.Contains(i.Labels, label) {
				return "labelled " + label
			}
		}
	}
	return ""
}

// aboutScreens says whether an issue is about a few screens rather than the
// whole app or none of it.
func aboutScreens(i githubIssue) bool {
	for _, label := range offScreenLabels {
		if slices.Contains(i.Labels, label) {
			return false
		}
	}
	matched := 0
	for _, s := range screens {
		if matchScreen(i, s.Pattern) != "" {
			matched++
		}
	}
	return matched < acrossScreens
}

// onScreen are the issues on a screen, the newest first.
func onScreen(issues []githubIssue, on string) []githubIssue {
	out := []githubIssue{}
	for _, i := range issues {
		if why := matchScreen(i, on); why != "" && aboutScreens(i) {
			i.Why = why
			out = append(out, i)
		}
	}
	slices.SortFunc(out, func(a, b githubIssue) int { return b.Number - a.Number })
	return out
}

// fetchGithub reads every open issue of the tracker. Pull requests share the
// issues list on GitHub and are left out.
func fetchGithub(ctx context.Context) ([]githubIssue, error) {
	ctx, cancel := context.WithTimeout(ctx, githubTimeout)
	defer cancel()
	client := extension.OutboundClient()
	issues := []githubIssue{}
	for page := 1; page <= githubMaxPages; page++ {
		url := fmt.Sprintf("https://api.github.com/repos/%s/issues?state=open&per_page=%d&page=%d", githubRepo, githubPageSize, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		req.Header.Set("User-Agent", "margince-issue-watch")
		got, err := fetchGithubPage(client, req)
		if err != nil {
			return nil, err
		}
		for _, g := range got {
			if g.PullRequest != nil {
				continue
			}
			labels := make([]string, 0, len(g.Labels))
			for _, l := range g.Labels {
				labels = append(labels, l.Name)
			}
			issues = append(issues, githubIssue{Number: g.Number, Title: g.Title, URL: g.HTMLURL, Labels: labels, Body: cut(g.Body, githubMaxBody)})
		}
		if len(got) < githubPageSize {
			return issues, nil
		}
	}
	return issues, nil
}

type githubItem struct {
	Number      int                     `json:"number"`
	Title       string                  `json:"title"`
	HTMLURL     string                  `json:"html_url"`
	Body        string                  `json:"body"`
	Labels      []struct{ Name string } `json:"labels"`
	PullRequest json.RawMessage         `json:"pull_request"`
}

func fetchGithubPage(client *http.Client, req *http.Request) ([]githubItem, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var items []githubItem
	return items, json.NewDecoder(resp.Body).Decode(&items)
}

// refreshGithub replaces the copy when it is older than githubEvery. Only the
// reader that claims the refresh fetches; the others read the copy there is.
// The copy is a cache of another system's issues, so it writes no ledger.
func refreshGithub(ctx context.Context, rt extension.Runtime) error {
	var claimed int64
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		var err error
		claimed, err = tx.Exec(ctx, `
			UPDATE ext.ext_issue_watch_github_sync SET claimed_at = now()
			WHERE id = 1
			  AND (synced_at IS NULL OR synced_at < now() - make_interval(secs => $1))
			  AND (claimed_at IS NULL OR claimed_at < now() - interval '1 minute')`, githubEvery.Seconds())
		return err
	})
	if err != nil || claimed == 0 {
		return err
	}
	issues, fetchErr := fetchGithub(ctx)
	return rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		if fetchErr != nil {
			_, err := tx.Exec(ctx, `UPDATE ext.ext_issue_watch_github_sync SET error = $1 WHERE id = 1`, cut(fetchErr.Error(), maxGithubErrors))
			return err
		}
		rows := make([]map[string]any, 0, len(issues))
		for _, i := range issues {
			rows = append(rows, map[string]any{"number": i.Number, "title": i.Title, "url": i.URL, "labels": strings.Join(i.Labels, "\n"), "body": i.Body})
		}
		raw, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM ext.ext_issue_watch_github_issue`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ext.ext_issue_watch_github_issue (number, title, url, labels, body)
			SELECT number, title, url, labels, body
			FROM jsonb_to_recordset($1::jsonb) AS r(number integer, title text, url text, labels text, body text)`, string(raw)); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE ext.ext_issue_watch_github_sync SET synced_at = now(), error = '' WHERE id = 1`)
		return err
	})
}

// screenGithub answers GET /ext/issue-watch/github: the tracker's open issues
// about the screen a page is on.
func screenGithub(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	args, err := extension.DecodeArgs[struct {
		Page string `json:"page"`
	}](in)
	if err != nil {
		return nil, err
	}
	if err := checkRoute("page", args.Page); err != nil {
		return nil, err
	}
	// A refresh that fails leaves the copy there was, and says why below.
	if err := refreshGithub(ctx, rt); err != nil {
		return nil, err
	}
	out := struct {
		Page      string        `json:"page"`
		Screen    string        `json:"screen"`
		Repo      string        `json:"repo"`
		Total     int           `json:"total"`
		Open      int           `json:"open"`
		Issues    []githubIssue `json:"issues"`
		SyncedAt  *time.Time    `json:"synced_at,omitempty"`
		SyncError string        `json:"sync_error"`
	}{Page: args.Page, Screen: screenOf(args.Page), Repo: githubRepo}
	all := []githubIssue{}
	err = rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT synced_at, error FROM ext.ext_issue_watch_github_sync WHERE id = 1`).Scan(&out.SyncedAt, &out.SyncError); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT number, title, url, labels, body FROM ext.ext_issue_watch_github_issue`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var i githubIssue
			var labels string
			if err := rows.Scan(&i.Number, &i.Title, &i.URL, &labels, &i.Body); err != nil {
				return err
			}
			if labels != "" {
				i.Labels = strings.Split(labels, "\n")
			} else {
				i.Labels = []string{}
			}
			all = append(all, i)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	out.Issues = onScreen(all, out.Screen)
	out.Total, out.Open = len(out.Issues), len(all)
	return json.Marshal(out)
}
