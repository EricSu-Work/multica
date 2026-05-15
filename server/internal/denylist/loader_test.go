package denylist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Valid(t *testing.T) {
	rules, err := Load(filepath.Join("testdata", "valid.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(rules) < 2 {
		t.Fatalf("expected >=2 rules, got %d", len(rules))
	}
	found := false
	for _, r := range rules {
		if r.Code == "EMAIL_BITBUCKET_NOTIFICATION" && r.TitleRegex != nil {
			// Verify case_insensitive applied — lowercase bitbucket should match
			if r.TitleRegex.MatchString("回复：[bitbucket] xxx") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("EMAIL_BITBUCKET_NOTIFICATION rule did not match case-insensitive variant")
	}
}

func TestLoad_BadSchema(t *testing.T) {
	_, err := Load(filepath.Join("testdata", "bad-schema.yaml"))
	if err == nil {
		t.Fatal("expected error for schema_version != 1")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join("testdata", "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_SkipsBadRule(t *testing.T) {
	tmpYAML := []byte(`schema_version: 1
rules:
  - code: GOOD
    title_pattern: 'ok'
    case_insensitive: true
  - code: BAD_REGEX
    title_pattern: '[invalid('
    case_insensitive: true
`)
	tmp := filepath.Join(t.TempDir(), "tmp.yaml")
	if err := os.WriteFile(tmp, tmpYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	rules, err := Load(tmp)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(rules) != 1 || rules[0].Code != "GOOD" {
		t.Fatalf("expected only GOOD rule, got %+v", rules)
	}
}
