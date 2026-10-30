-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- Problems the app ran into in members' browsers, one row per kind of
-- problem: the same failure on the same kind of page counts up rather than
-- adding a row. A resolved issue that happens again reopens.

CREATE TABLE ext.ext_issue_watch_issue (
    id              uuid        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    fingerprint     text        NOT NULL UNIQUE CHECK (length(fingerprint) = 64),
    kind            text        NOT NULL CHECK (kind IN ('server', 'refused', 'network', 'app', 'slow', 'stuck')),
    title           text        NOT NULL CHECK (length(title) BETWEEN 1 AND 300),
    detail          text        NOT NULL CHECK (length(detail) <= 2000),
    pattern         text        NOT NULL CHECK (pattern LIKE '/%' AND length(pattern) <= 300),
    last_page       text        NOT NULL CHECK (last_page LIKE '/%' AND length(last_page) <= 300),
    reference       text        NOT NULL CHECK (length(reference) <= 300),
    browser         text        NOT NULL CHECK (length(browser) <= 300),
    occurrences     integer     NOT NULL DEFAULT 1 CHECK (occurrences >= 1),
    first_seen      timestamptz NOT NULL DEFAULT now(),
    last_seen       timestamptz NOT NULL DEFAULT now(),
    last_reporter   uuid        NOT NULL,
    resolved_at     timestamptz,
    resolved_by     uuid,
    CHECK ((resolved_at IS NULL) = (resolved_by IS NULL))
);

CREATE INDEX ext_issue_watch_issue_by_last_seen
    ON ext.ext_issue_watch_issue (last_seen DESC);

-- The watcher's settings: one row, written by an admin.
CREATE TABLE ext.ext_issue_watch_config (
    id          smallint    NOT NULL PRIMARY KEY CHECK (id = 1),
    config      jsonb       NOT NULL,
    updated_by  uuid        NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_issue_watch_issue TO margince_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_issue_watch_config TO margince_app;
