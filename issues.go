// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package issuewatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

const (
	issueEntity     = "ext_issue_watch_issue"
	maxTitle        = 300
	maxDetail       = 2000
	maxRoute        = 300
	maxReference    = 300
	maxBrowser      = 300
	listedIssues    = 200
	statusOpen      = "open"
	statusResolved  = "resolved"
	statusAnyIssues = "all"
)

var kinds = map[string]bool{"server": true, "refused": true, "network": true, "app": true, "slow": true, "stuck": true, "ui": true}

type issue struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	Title        string     `json:"title"`
	Detail       string     `json:"detail"`
	Pattern      string     `json:"pattern"`
	LastPage     string     `json:"last_page"`
	Reference    string     `json:"reference"`
	Browser      string     `json:"browser"`
	Occurrences  int        `json:"occurrences"`
	FirstSeen    time.Time  `json:"first_seen"`
	LastSeen     time.Time  `json:"last_seen"`
	LastReporter string     `json:"last_reporter"`
	ResolvedAt   *time.Time `json:"resolved_at"`
	ResolvedBy   *string    `json:"resolved_by"`
}

const issueColumns = `id::text, kind, title, detail, pattern, last_page, reference, browser,
	occurrences, first_seen, last_seen, last_reporter::text, resolved_at, resolved_by::text`

func scanIssue(row interface{ Scan(...any) error }) (issue, error) {
	var i issue
	err := row.Scan(&i.ID, &i.Kind, &i.Title, &i.Detail, &i.Pattern, &i.LastPage, &i.Reference, &i.Browser,
		&i.Occurrences, &i.FirstSeen, &i.LastSeen, &i.LastReporter, &i.ResolvedAt, &i.ResolvedBy)
	return i, err
}

// idPart matches an id in a route or a message: a UUID, or four digits or
// more, so a three-digit status stays part of what the issue says.
// It is built where it is used: core refuses a package-level value that runs
// code at import.
const idPart = `(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|\b\d{4,}\b`

// fingerprint is what makes two reports one issue: the same kind of problem,
// on the same kind of page, saying the same thing once its ids are taken out.
func fingerprint(kind, pattern, title string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + pattern + "\x00" + regexp.MustCompile(idPart).ReplaceAllString(title, ":id")))
	return hex.EncodeToString(sum[:])
}

func checkRoute(name, route string) error {
	if !strings.HasPrefix(route, "/") || len(route) > maxRoute {
		return fmt.Errorf("%w: %s is an app route, starting with / and at most %d characters", extension.ErrInvalid, name, maxRoute)
	}
	return nil
}

// cut keeps a browser-sent text to its column's length, on a whole character.
func cut(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	for limit > 0 && !utf8RuneStart(text[limit]) {
		limit--
	}
	return text[:limit]
}

func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }

// reportIssue answers POST /ext/issue-watch/report: one problem a member's
// browser saw. A problem already known counts up, and reopens if resolved.
func reportIssue(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	args, err := extension.DecodeArgs[struct {
		Kind      string `json:"kind"`
		Title     string `json:"title"`
		Detail    string `json:"detail"`
		Pattern   string `json:"pattern"`
		Page      string `json:"page"`
		Reference string `json:"reference"`
		Browser   string `json:"browser"`
	}](in)
	if err != nil {
		return nil, err
	}
	title := cut(args.Title, maxTitle)
	switch {
	case !kinds[args.Kind]:
		return nil, fmt.Errorf("%w: kind is server, refused, network, app, slow, stuck or ui", extension.ErrInvalid)
	case title == "":
		return nil, fmt.Errorf("%w: an issue needs a title", extension.ErrInvalid)
	}
	if err := checkRoute("pattern", args.Pattern); err != nil {
		return nil, err
	}
	if err := checkRoute("page", args.Page); err != nil {
		return nil, err
	}
	reporter := rt.Caller().UserID
	if reporter == "" {
		return nil, fmt.Errorf("%w: an issue needs a member behind it", extension.ErrForbidden)
	}
	var out issue
	err = rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		var inserted bool
		row := tx.QueryRow(ctx, `
			INSERT INTO ext.ext_issue_watch_issue AS known
				(fingerprint, kind, title, detail, pattern, last_page, reference, browser, last_reporter)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::uuid)
			ON CONFLICT (fingerprint) DO UPDATE SET
				title = EXCLUDED.title, detail = EXCLUDED.detail, last_page = EXCLUDED.last_page,
				reference = EXCLUDED.reference, browser = EXCLUDED.browser,
				last_reporter = EXCLUDED.last_reporter,
				occurrences = known.occurrences + 1, last_seen = now(),
				resolved_at = NULL, resolved_by = NULL
			RETURNING (xmax = 0), `+issueColumns,
			fingerprint(args.Kind, args.Pattern, title), args.Kind, title, cut(args.Detail, maxDetail),
			args.Pattern, args.Page, cut(args.Reference, maxReference), cut(args.Browser, maxBrowser), reporter)
		var i issue
		if err := row.Scan(&inserted, &i.ID, &i.Kind, &i.Title, &i.Detail, &i.Pattern, &i.LastPage, &i.Reference, &i.Browser,
			&i.Occurrences, &i.FirstSeen, &i.LastSeen, &i.LastReporter, &i.ResolvedAt, &i.ResolvedBy); err != nil {
			return err
		}
		out = i
		// The ledger keeps an issue's first report; a repeat only counts up.
		if !inserted {
			return nil
		}
		after, err := json.Marshal(i)
		if err != nil {
			return err
		}
		return record(ctx, tx, extension.Change{Action: extension.AuditCreate, Entity: issueEntity, ID: i.ID, After: after}, "issue_seen", i)
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// listIssues answers GET /ext/issue-watch/issues: the most recently seen first.
func listIssues(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	args, err := extension.DecodeArgs[struct {
		Status string `json:"status"`
	}](in)
	if err != nil {
		return nil, err
	}
	if args.Status == "" {
		args.Status = statusOpen
	}
	if args.Status != statusOpen && args.Status != statusResolved && args.Status != statusAnyIssues {
		return nil, fmt.Errorf("%w: status is open, resolved or all", extension.ErrInvalid)
	}
	issues := []issue{}
	err = rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+issueColumns+`
			FROM ext.ext_issue_watch_issue
			WHERE $1 = 'all' OR ($1 = 'open') = (resolved_at IS NULL)
			ORDER BY last_seen DESC
			LIMIT $2`, args.Status, listedIssues)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			i, err := scanIssue(rows)
			if err != nil {
				return err
			}
			issues = append(issues, i)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string][]issue{"issues": issues})
}

// resolveIssue answers POST /ext/issue-watch/issues/resolve. The issue
// reopens if it happens again.
func resolveIssue(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	args, err := extension.DecodeArgs[struct {
		ID string `json:"id"`
	}](in)
	if err != nil {
		return nil, err
	}
	if !extension.IsCanonicalUUID(args.ID) {
		return nil, fmt.Errorf("%w: id is not an issue id", extension.ErrInvalid)
	}
	member := rt.Caller().UserID
	if member == "" {
		return nil, fmt.Errorf("%w: resolving needs a member behind it", extension.ErrForbidden)
	}
	var out issue
	err = rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		before, err := scanIssue(tx.QueryRow(ctx, `
			SELECT `+issueColumns+` FROM ext.ext_issue_watch_issue WHERE id = $1::uuid FOR UPDATE`, args.ID))
		if errors.Is(err, extension.ErrNoRows) {
			return extension.ErrNotFound
		}
		if err != nil {
			return err
		}
		out, err = scanIssue(tx.QueryRow(ctx, `
			UPDATE ext.ext_issue_watch_issue SET resolved_at = now(), resolved_by = $2::uuid
			WHERE id = $1::uuid
			RETURNING `+issueColumns, args.ID, member))
		if err != nil {
			return err
		}
		prior, err := json.Marshal(before)
		if err != nil {
			return err
		}
		after, err := json.Marshal(out)
		if err != nil {
			return err
		}
		return record(ctx, tx, extension.Change{Action: extension.AuditUpdate, Entity: issueEntity, ID: out.ID, Before: prior, After: after}, "issue_resolved", out)
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func record(ctx context.Context, tx extension.Tx, change extension.Change, verb string, i issue) error {
	payload, err := json.Marshal(map[string]string{"issue_id": i.ID, "kind": i.Kind, "pattern": i.Pattern})
	if err != nil {
		return err
	}
	return tx.Record(ctx, change, extension.Event{Verb: verb, Payload: payload})
}
