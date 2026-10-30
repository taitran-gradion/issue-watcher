// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package issuewatch watches every page of the app in members' browsers and
// shows a problem the moment it happens: a failed request, an app error, a
// slow request or a spinner that never stops. Each problem is kept, counted
// and grouped, so developers see what broke, where, and how often.
//
//	issues.go     report an issue, list them, resolve one
//	config.go     what counts as an issue, set by an admin
//	migrations/   its two tables, and the screen-problem kind
//	api/          its routes, merged into core's contract
//	frontend/     the watcher on every page, and the Issues page
package issuewatch

import (
	"embed"

	"github.com/margince/margince/backend/pkg/extension"
)

//go:embed migrations
var migrations embed.FS

// New returns the unit's declaration: everything it gives core, in one place.
func New() extension.Extension {
	return extension.Extension{
		Name:        "issue-watch",
		Version:     "0.3.0",
		Description: "Watches every page for failed requests, app errors, slow loads and stuck spinners, and keeps each issue for developers.",
		Tools: []extension.Tool{
			{Name: "issue_watch_report", Handle: reportIssue},
			{Name: "issue_watch_list", Handle: listIssues},
			{Name: "issue_watch_resolve", Handle: resolveIssue},
			{Name: "issue_watch_read_config", Handle: readConfig},
			{Name: "issue_watch_save_config", Handle: saveConfig},
			{Name: "issue_watch_screen_github", Handle: screenGithub},
		},
		Migrations: migrations,
	}
}
