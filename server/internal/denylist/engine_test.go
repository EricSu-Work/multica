package denylist

import (
	"regexp"
	"testing"
)

func mustRe(t *testing.T, s string) *regexp.Regexp {
	t.Helper()
	r, err := regexp.Compile(s)
	if err != nil {
		t.Fatalf("compile %q: %v", s, err)
	}
	return r
}

func TestEngine_Evaluate(t *testing.T) {
	rules := []Rule{
		{
			Code:        "EMAIL_BITBUCKET_NOTIFICATION",
			Description: "Bitbucket notification",
			TitleRegex:  mustRe(t, `(?i)\[Bitbucket\]|回复\s*[:：]\s*\[Bitbucket\]`),
		},
		{
			Code:             "EMAIL_NOREPLY_SENDER",
			Description:      "noreply / system sender",
			DescriptionRegex: mustRe(t, `(?i)\b(?:noreply|no-reply|donotreply)@`),
		},
	}
	e := NewEngine(rules)

	cases := []struct {
		name        string
		in          Input
		wantBlocked bool
		wantCode    string
	}{
		{
			name:        "Bitbucket title hits",
			in:          Input{Title: "[Bitbucket] Pull request #1"},
			wantBlocked: true,
			wantCode:    "EMAIL_BITBUCKET_NOTIFICATION",
		},
		{
			name:        "CJK 回复:[Bitbucket]",
			in:          Input{Title: "回复：[Bitbucket] 拉取请求 # 4443"},
			wantBlocked: true,
			wantCode:    "EMAIL_BITBUCKET_NOTIFICATION",
		},
		{
			name:        "noreply sender in description hits",
			in:          Input{Title: "Account updated", Description: "From: noreply@vendor.example"},
			wantBlocked: true,
			wantCode:    "EMAIL_NOREPLY_SENDER",
		},
		{
			name:        "real customer subject + sender passes",
			in:          Input{Title: "Question about my order #12345", Description: "From: jane@customer.example"},
			wantBlocked: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := e.Evaluate(tc.in)
			if got.Blocked != tc.wantBlocked {
				t.Fatalf("Blocked=%v want %v (rule=%q)", got.Blocked, tc.wantBlocked, got.RuleCode)
			}
			if tc.wantBlocked && got.RuleCode != tc.wantCode {
				t.Fatalf("RuleCode=%q want %q", got.RuleCode, tc.wantCode)
			}
		})
	}
}

func TestEngine_Replace_IsAtomic(t *testing.T) {
	e := NewEngine([]Rule{{
		Code:       "OLD",
		TitleRegex: mustRe(t, `^OLD`),
	}})
	if !e.Evaluate(Input{Title: "OLD title"}).Blocked {
		t.Fatal("initial OLD rule should hit")
	}
	e.Replace([]Rule{{
		Code:       "NEW",
		TitleRegex: mustRe(t, `^NEW`),
	}})
	if e.Evaluate(Input{Title: "OLD title"}).Blocked {
		t.Fatal("OLD title should no longer match after Replace")
	}
	if !e.Evaluate(Input{Title: "NEW title"}).Blocked {
		t.Fatal("NEW title should match after Replace")
	}
}
