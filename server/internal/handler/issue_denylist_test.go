package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/multica-ai/multica/server/internal/denylist"
)

// newTestHandlerWithDenyList builds a minimal *Handler with only the
// denyList field populated. The deny-list filter in CreateIssue runs
// BEFORE any DB / auth resolution, so the surrounding fields (Queries,
// TxStarter, etc.) are intentionally left nil — touching them in the
// pass-through tests would mean the deny-list filter accepted the
// request, which is exactly what we want to verify.
//
// Pass-through tests assert "not 422"; they may receive any other
// status code (e.g. 400 from missing workspace) and that is fine.
func newTestHandlerWithDenyList(t *testing.T, engine *denylist.Engine) *Handler {
	t.Helper()
	return &Handler{denyList: engine}
}

// denyListRequest builds a POST /api/issues request with no auth and
// no workspace identifier. Auth and workspace resolution happen AFTER
// the deny-list check, so omitting them keeps the pass-through tests
// independent of the auth middleware. The reject test never reaches
// those checks at all.
func denyListRequest(body any) *http.Request {
	buf := new(bytes.Buffer)
	_ = json.NewEncoder(buf).Encode(body)
	req := httptest.NewRequest(http.MethodPost, "/api/issues", buf)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestCreateIssue_DenyList_Rejects422(t *testing.T) {
	engine := denylist.NewEngine([]denylist.Rule{
		{
			Code:        "TEST_BITBUCKET",
			Description: "test rule",
			TitleRegex:  regexp.MustCompile(`(?i)\[Bitbucket\]`),
		},
	})
	h := newTestHandlerWithDenyList(t, engine)

	req := denyListRequest(map[string]any{
		"title":       "[Bitbucket] PR #1",
		"description": "auto-generated",
	})

	rec := httptest.NewRecorder()
	h.CreateIssue(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d want 422; body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if out["rule_code"] != "TEST_BITBUCKET" {
		t.Fatalf("rule_code=%v want TEST_BITBUCKET", out["rule_code"])
	}
}

func TestCreateIssue_DenyList_NilEngine_PassesThrough(t *testing.T) {
	h := newTestHandlerWithDenyList(t, nil)
	req := denyListRequest(map[string]any{
		"title":       "[Bitbucket] PR #1",
		"description": "auto-generated",
	})

	rec := httptest.NewRecorder()
	h.CreateIssue(rec, req)

	if rec.Code == http.StatusUnprocessableEntity {
		t.Fatalf("nil engine should not filter; got 422: %s", rec.Body.String())
	}
	// We only assert "not 422" — without auth/workspace the request will
	// fail at a later validation step (e.g. 400 missing workspace_id), and
	// any non-422 status confirms the deny-list filter is genuinely
	// disabled when the engine is nil.
}

func TestCreateIssue_DenyList_PassesNonMatching(t *testing.T) {
	engine := denylist.NewEngine([]denylist.Rule{
		{
			Code:       "TEST_NOTHING",
			TitleRegex: regexp.MustCompile(`^IMPOSSIBLE_PREFIX_xyz`),
		},
	})
	h := newTestHandlerWithDenyList(t, engine)

	req := denyListRequest(map[string]any{
		"title":       "Real customer question",
		"description": "Hello support",
	})

	rec := httptest.NewRecorder()
	h.CreateIssue(rec, req)

	if rec.Code == http.StatusUnprocessableEntity {
		t.Fatalf("non-matching title should pass; got 422: %s", rec.Body.String())
	}
}
