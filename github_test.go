// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package issuewatch

import (
	"slices"
	"testing"
)

func TestScreenOfFindsTheKindOfPage(t *testing.T) {
	for page, want := range map[string]string{
		"/deals/01a185c4-350d-74d8-94d8-2950be42409f":     "/deals/:id",
		"/deals/01a185c4-350d-74d8-94d8-2950be42409f/tab": "/deals/:id",
		"/deals":                "/deals",
		"/settings/connections": "/settings/connections",
		"/settings/unknown":     "/settings",
		"/ext/issue-watch":      "/ext/issue-watch",
		"/":                     "/",
	} {
		if got := screenOf(page); got != want {
			t.Errorf("screenOf(%q) = %q, want %q", page, got, want)
		}
	}
}

func TestMatchSaysWhyAnIssueIsOnAScreen(t *testing.T) {
	cases := []struct {
		name   string
		issue  githubIssue
		screen string
		want   string
	}{
		{"a phrase", githubIssue{Title: "The Meet verb on a deal with no company is greyed"}, "/deals/:id", `says "on a deal"`},
		{"a path", githubIssue{Body: "Opening `/deals/{id}/documents` fails"}, "/deals/:id", "names /deals/:id/documents"},
		{"an api path", githubIssue{Body: "GET /v1/worklist is slow"}, "/home", "names /worklist"},
		{"a label, below its screen", githubIssue{Labels: []string{"area: deals"}}, "/deals/:id", "labelled area: deals"},
		{"a phrase cut mid-word", githubIssue{Title: "the redeal page"}, "/deals/:id", ""},
		{"another screen", githubIssue{Title: "The deals table loses its sort"}, "/deals/:id", ""},
		{"a source file", githubIssue{Body: "see frontend/src/deals/page.tsx"}, "/deals", ""},
	}
	for _, c := range cases {
		if got := matchScreen(c.issue, c.screen); got != c.want {
			t.Errorf("%s: matchScreen = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestOnScreenListsTheNewestFirst(t *testing.T) {
	issues := []githubIssue{
		{Number: 1, Title: "deals table is slow"},
		{Number: 9, Title: "pipeline board drags"},
		{Number: 5, Title: "contact page"},
	}
	var got []int
	for _, i := range onScreen(issues, "/deals") {
		got = append(got, i.Number)
	}
	if !slices.Equal(got, []int{9, 1}) {
		t.Fatalf("onScreen = %v, want [9 1]", got)
	}
}

func TestOnScreenLeavesOutIssuesAboutTheWholeApp(t *testing.T) {
	issues := []githubIssue{
		{Number: 1, Title: "main is red", Body: "fails on the worklist, the deals table, the contact page and analytics"},
		{Number: 2, Title: "the deals table is slow", Labels: []string{"area: ci-tests"}},
		{Number: 3, Title: "the deals table and the deal page disagree"},
	}
	var got []int
	for _, i := range onScreen(issues, "/deals") {
		got = append(got, i.Number)
	}
	if !slices.Equal(got, []int{3}) {
		t.Fatalf("onScreen = %v, want [3]", got)
	}
}
