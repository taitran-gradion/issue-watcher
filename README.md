# issue-watch

A Margince extension unit that shows developers what is wrong on the screen they are on.

- **The watcher** floats over every page: the problems the app hit on this screen (failed requests, UI errors), each one copyable as a report.
- **GitHub issues per screen**: a count badge and a list of the open issues in `margince/margince` that are about this screen — matched by a path the issue names, a phrase, or an `area:` label — and why each one matched. Read-only: it fetches public issues without a token, caches them for 15 minutes, and never writes to GitHub. Issues labelled `area: ci-tests`, or that name four or more screens, are left out as being about the whole app.
- **`#/ext/issue-watch`** lists every problem seen, with its status.
- MCP tools: `issue_watch_list`, `issue_watch_report`, `issue_watch_resolve`, `issue_watch_read_config`, `issue_watch_save_config`, `issue_watch_screen_github`.

## Install

In an installation's `extensions.mod`:

```
require github.com/taitran-gradion/issue-watch v0.0.0
replace github.com/taitran-gradion/issue-watch => ./addons/issue-watch
```

Roles need the unit's `ext_issue_watch_*` objects under Settings › Roles.
