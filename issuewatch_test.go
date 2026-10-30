// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package issuewatch

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestNewNamesTheUnitAfterItsModulePath(t *testing.T) {
	if got := New().Name; got != "issue-watch" {
		t.Fatalf("Name = %q, want issue-watch: the composer takes the name from the module path", got)
	}
}

func TestFingerprintGroupsTheSameProblemAcrossIds(t *testing.T) {
	a := fingerprint("server", "/contacts/:id", "PATCH /v1/contacts/0198a2b4-1c2d-7e3f-8a9b-0c1d2e3f4a5b 500")
	b := fingerprint("server", "/contacts/:id", "PATCH /v1/contacts/01a185c4-350d-74d8-94d8-2950be42409f 500")
	if a != b {
		t.Fatal("two contacts failing the same way are two issues, want one")
	}
	if a == fingerprint("server", "/contacts/:id", "PATCH /v1/contacts/0198a2b4-1c2d-7e3f-8a9b-0c1d2e3f4a5b 502") {
		t.Fatal("a 500 and a 502 are one issue, want two")
	}
	if a == fingerprint("server", "/deals/:id", "PATCH /v1/contacts/0198a2b4-1c2d-7e3f-8a9b-0c1d2e3f4a5b 500") {
		t.Fatal("the same failure on two kinds of page is one issue, want two")
	}
}

func TestCutKeepsWholeCharacters(t *testing.T) {
	if got := cut("Ngọc", 3); got != "Ng" {
		t.Fatalf("cut(Ngọc, 3) = %q, want Ng: a cut mid-character stores invalid text", got)
	}
}

func TestConfigCheck(t *testing.T) {
	c := defaultConfig()
	c.Ignore = []string{" ResizeObserver loop ", "", "  "}
	if err := c.check(); err != nil || len(c.Ignore) != 1 || c.Ignore[0] != "ResizeObserver loop" {
		t.Fatalf("check = %v, ignore %q; want the one text, trimmed", err, c.Ignore)
	}
	for name, bad := range map[string]config{
		"a negative slow time":  {SlowMs: -1},
		"a stuck time too long": {StuckMs: maxStuckMs + 1},
		"too many ignores":      {Ignore: make([]string, maxIgnores+1)},
		"an ignore too long":    {Ignore: []string{strings.Repeat("x", maxIgnoreText+1)}},
	} {
		if err := bad.check(); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

// The routes refuse a bad argument before they touch the runtime, so a nil
// runtime is enough to hold the checks.
func TestRoutesRefuseBadArguments(t *testing.T) {
	report := map[string]any{"kind": "server", "title": "POST /v1/contacts 500", "pattern": "/contacts", "page": "/contacts"}
	with := func(key string, value any) map[string]any {
		out := map[string]any{}
		for k, v := range report {
			out[k] = v
		}
		out[key] = value
		return out
	}
	cases := map[string]struct {
		handle extension.ToolHandler
		args   map[string]any
	}{
		"an unknown kind":           {reportIssue, with("kind", "oops")},
		"no title":                  {reportIssue, with("title", " ")},
		"a page that is no route":   {reportIssue, with("page", "contacts")},
		"an unknown status":         {listIssues, map[string]any{"status": "closed"}},
		"resolve by no id":          {resolveIssue, map[string]any{"id": "nope"}},
		"a slow time out of bounds": {saveConfig, map[string]any{"slow_ms": maxSlowMs + 1}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in, err := json.Marshal(c.args)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.handle(context.Background(), nil, in); !errors.Is(err, extension.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

// The settings page sends every field it shows; a field the decoder does not
// know refuses the whole save.
func TestConfigDecodesWhatThePageSends(t *testing.T) {
	sent := `{"server_errors":true,"refusals":true,"app_errors":true,"ui_problems":false,"slow_ms":3000,"stuck_ms":10000,"toasts":true,"ignore":[]}`
	c, err := extension.DecodeArgs[config](json.RawMessage(sent))
	if err != nil {
		t.Fatalf("decode = %v, want the page's settings accepted", err)
	}
	if c.UIProblems {
		t.Fatal("ui_problems = true, want the false the page sent")
	}
}
