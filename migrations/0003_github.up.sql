-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- A copy of the open issues on the product's GitHub tracker, read and never
-- written, so the watcher can say how many are known about a screen. It is
-- refreshed by whoever reads it once it is old.

CREATE TABLE ext.ext_issue_watch_github_issue (
    number  integer NOT NULL PRIMARY KEY,
    title   text    NOT NULL,
    url     text    NOT NULL,
    -- One label per line.
    labels  text    NOT NULL,
    body    text    NOT NULL
);

-- When the copy was last refreshed, who is refreshing it now, and why the last
-- refresh failed: one row.
CREATE TABLE ext.ext_issue_watch_github_sync (
    id          smallint    NOT NULL PRIMARY KEY CHECK (id = 1),
    synced_at   timestamptz,
    claimed_at  timestamptz,
    error       text        NOT NULL DEFAULT ''
);
INSERT INTO ext.ext_issue_watch_github_sync (id) VALUES (1);

GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_issue_watch_github_issue TO margince_app;
GRANT SELECT, UPDATE ON ext.ext_issue_watch_github_sync TO margince_app;
