-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- A problem on screen is an issue too: an error shown, broken or untranslated
-- text, a picture that did not load, a blank or too-wide page.

ALTER TABLE ext.ext_issue_watch_issue DROP CONSTRAINT ext_issue_watch_issue_kind_check;
ALTER TABLE ext.ext_issue_watch_issue ADD CONSTRAINT ext_issue_watch_issue_kind_check
    CHECK (kind IN ('server', 'refused', 'network', 'app', 'slow', 'stuck', 'ui'));
