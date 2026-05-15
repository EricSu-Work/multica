package denylist

import "regexp"

// Rule is a deny-list rule loaded from the ConfigMap YAML.
//
// At least one of TitleRegex / DescriptionRegex must be non-nil. The
// engine treats them as OR — either match blocks the request. A future
// extension could add ASSIGNEE_TYPE / WORKSPACE_ID filters here.
type Rule struct {
	Code             string
	Description      string
	TitleRegex       *regexp.Regexp
	DescriptionRegex *regexp.Regexp
}

// Input is the request shape the engine evaluates.
type Input struct {
	Title        string
	Description  string
	AssigneeType string // reserved for future rule extensions
}

// Verdict is the engine's decision. RuleCode and Reason populated only
// when Blocked is true.
type Verdict struct {
	Blocked  bool
	RuleCode string
	Reason   string
}
