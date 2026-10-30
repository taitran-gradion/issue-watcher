// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package issuewatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/pkg/extension"
)

const (
	configEntity = "ext_issue_watch_config"
	// The config row's id: there is one row, so its ledger entries share an id.
	configRowID   = "00000000-0000-4000-8000-000000000001"
	maxSlowMs     = 60_000
	maxStuckMs    = 120_000
	maxIgnores    = 20
	maxIgnoreText = 200
)

// config is what counts as an issue. The watcher in every browser reads it;
// only an admin changes it.
type config struct {
	// ServerErrors watches answers of 500 and over, and requests that never arrived.
	ServerErrors bool `json:"server_errors"`
	// Refusals watches answers of 400 to 499, but for signed-out (401) and not-found (404).
	Refusals bool `json:"refusals"`
	// AppErrors watches errors the app throws or logs.
	AppErrors bool `json:"app_errors"`
	// UIProblems watches what the screen shows wrong: an error on screen, broken
	// or untranslated text, a picture that did not load, a blank or too-wide page.
	UIProblems bool `json:"ui_problems"`
	// SlowMs is how long a request may take before it is slow; 0 does not watch.
	SlowMs int `json:"slow_ms"`
	// StuckMs is how long a loading indicator may show before it is stuck; 0 does not watch.
	StuckMs int `json:"stuck_ms"`
	// Toasts shows each new issue to the member who hit it, as it happens.
	Toasts bool `json:"toasts"`
	// Ignore lists texts: an issue whose title holds one is not watched.
	Ignore []string `json:"ignore"`
}

func defaultConfig() config {
	return config{ServerErrors: true, Refusals: true, AppErrors: true, UIProblems: true, SlowMs: 3000, StuckMs: 10_000, Toasts: true, Ignore: []string{}}
}

func (c *config) check() error {
	switch {
	case c.SlowMs < 0 || c.SlowMs > maxSlowMs:
		return fmt.Errorf("%w: slow_ms is 0 to %d", extension.ErrInvalid, maxSlowMs)
	case c.StuckMs < 0 || c.StuckMs > maxStuckMs:
		return fmt.Errorf("%w: stuck_ms is 0 to %d", extension.ErrInvalid, maxStuckMs)
	case len(c.Ignore) > maxIgnores:
		return fmt.Errorf("%w: at most %d ignored texts", extension.ErrInvalid, maxIgnores)
	}
	kept := make([]string, 0, len(c.Ignore))
	for _, text := range c.Ignore {
		text = strings.TrimSpace(text)
		if len(text) > maxIgnoreText {
			return fmt.Errorf("%w: an ignored text is at most %d characters", extension.ErrInvalid, maxIgnoreText)
		}
		if text != "" {
			kept = append(kept, text)
		}
	}
	c.Ignore = kept
	return nil
}

func configOf(ctx context.Context, tx extension.Tx) (config, error) {
	var raw string
	err := tx.QueryRow(ctx, `SELECT config::text FROM ext.ext_issue_watch_config WHERE id = 1`).Scan(&raw)
	if errors.Is(err, extension.ErrNoRows) {
		return defaultConfig(), nil
	}
	if err != nil {
		return config{}, err
	}
	c := defaultConfig()
	return c, json.Unmarshal([]byte(raw), &c)
}

// readConfig answers GET /ext/issue-watch/config.
func readConfig(ctx context.Context, rt extension.Runtime, _ json.RawMessage) (json.RawMessage, error) {
	var c config
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		var err error
		c, err = configOf(ctx, tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(c)
}

// saveConfig answers POST /ext/issue-watch/config: the whole config, replaced.
func saveConfig(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	c, err := extension.DecodeArgs[config](in)
	if err != nil {
		return nil, err
	}
	if err := c.check(); err != nil {
		return nil, err
	}
	member := rt.Caller().UserID
	if member == "" {
		return nil, fmt.Errorf("%w: settings need a member behind them", extension.ErrForbidden)
	}
	after, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	err = rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		before, err := configOf(ctx, tx)
		if err != nil {
			return err
		}
		prior, err := json.Marshal(before)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ext.ext_issue_watch_config (id, config, updated_by) VALUES (1, $1::jsonb, $2::uuid)
			ON CONFLICT (id) DO UPDATE SET config = EXCLUDED.config, updated_by = EXCLUDED.updated_by, updated_at = now()`,
			string(after), member); err != nil {
			return err
		}
		return tx.Record(ctx,
			extension.Change{Action: extension.AuditUpdate, Entity: configEntity, ID: configRowID, Before: prior, After: after},
			extension.Event{Verb: "config_saved", Payload: json.RawMessage(`{}`)})
	})
	if err != nil {
		return nil, err
	}
	return after, nil
}
