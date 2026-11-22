package store_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestChangeControlSettings(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "CC", Name: "Change control"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	cc, err := st.ChangeControl(ctx, ten.ID, p.ID)
	must(t, err)
	if cc.Mode != "off" || !cc.LockSignedBaselines || cc.Board.Quorum != "all" || len(store.EffectiveProtectedStatuses(cc)) != len(model.DefaultProtectedStatuses) {
		t.Fatalf("defaults %+v", cc)
	}
	for _, bad := range []model.ChangeControlSettings{
		{Mode: "strict"},
		{Mode: "warn", ProtectedStatuses: []string{"nope"}},
		{Mode: "warn", Board: model.ChangeBoard{Quorum: "most"}},
		{Mode: "warn", Board: model.ChangeBoard{Users: []string{"not-an-address"}}},
	} {
		if err := st.SetChangeControl(ctx, ten.ID, p.ID, &bad); !errors.Is(err, store.ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	in := model.ChangeControlSettings{Mode: "enforce", ProtectedStatuses: []string{"approved", " approved", "ready"}, Board: model.ChangeBoard{Users: []string{"A@X.org"}, Roles: []string{"manager"}, Quorum: "2"},
		SignatureMeaning: "Approved for release", BaselineOnClose: true, UpdatedBy: "me"}
	must(t, st.SetChangeControl(ctx, ten.ID, p.ID, &in))
	cc, err = st.ChangeControl(ctx, ten.ID, p.ID)
	must(t, err)
	if cc.Mode != "enforce" || len(cc.ProtectedStatuses) != 2 || cc.Board.Users[0] != "a@x.org" || cc.Board.Quorum != "2" || !cc.BaselineOnClose || cc.LockSignedBaselines || cc.UpdatedAt == nil {
		t.Fatalf("saved %+v", cc)
	}
	if !store.ValidQuorum("any") || store.ValidQuorum("0") || store.ValidQuorum("x") {
		t.Fatal("quorum")
	}
}

func TestChangeRequestLifecycle(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "CR", Name: "Change requests"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	other := &model.Project{Key: "OT", Name: "Other"}
	must(t, st.CreateProject(ctx, ten.ID, other, false))
	rq := &model.Requirement{ProjectID: p.ID, Title: "Login", Status: "approved"}
	must(t, st.CreateRequirement(ctx, ten.ID, rq))
	tc := &model.TestCase{ProjectID: p.ID, Title: "Login test"}
	must(t, st.CreateTest(ctx, ten.ID, tc))
	foreign := &model.Requirement{ProjectID: other.ID, Title: "Elsewhere"}
	must(t, st.CreateRequirement(ctx, ten.ID, foreign))
	d := &model.Defect{ProjectID: p.ID, Title: "Wrong text"}
	must(t, st.CreateDefect(ctx, ten.ID, d))

	raw := func(s string) json.RawMessage { return json.RawMessage(s) }
	if err := st.CreateChangeRequest(ctx, ten.ID, &model.ChangeRequest{ProjectID: p.ID}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("title required", err)
	}
	if err := st.CreateChangeRequest(ctx, ten.ID, &model.ChangeRequest{ProjectID: p.ID, Title: "x", Category: "whim"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("category", err)
	}
	bad := []model.ChangeRequestItem{
		{EntityType: model.KindRequirement, EntityID: rq.ID, Proposed: map[string]json.RawMessage{"key": raw(`"X"`)}},
		{EntityType: model.KindRequirement, EntityID: rq.ID, Proposed: map[string]json.RawMessage{}},
		{EntityType: model.KindRequirement, EntityID: foreign.ID, Proposed: map[string]json.RawMessage{"title": raw(`"Y"`)}},
		{EntityType: "risk", EntityID: rq.ID, Proposed: map[string]json.RawMessage{"title": raw(`"Y"`)}},
	}
	for _, it := range bad {
		err := st.CreateChangeRequest(ctx, ten.ID, &model.ChangeRequest{ProjectID: p.ID, Title: "bad", Items: []model.ChangeRequestItem{it}})
		if !errors.Is(err, store.ErrInvalid) && !errors.Is(err, store.ErrNotFound) {
			t.Errorf("%+v: %v", it, err)
		}
	}
	cr := &model.ChangeRequest{ProjectID: p.ID, Title: "Fix wording", Reason: "Ambiguous", Category: "correction", CreatedBy: "a@x.org",
		Links: model.ChangeRequestLinks{DefectIDs: []string{d.ID}, Issues: []string{"JIRA-1", "JIRA-1"}},
		Items: []model.ChangeRequestItem{{EntityType: model.KindRequirement, EntityID: rq.ID, Proposed: map[string]json.RawMessage{"title": raw(` "Log in" `), "tags": raw(`["auth"]`)}}}}
	must(t, st.CreateChangeRequest(ctx, ten.ID, cr))
	if cr.Key != "CR-CR-1" {
		t.Fatal(cr.Key)
	}
	got, err := st.GetChangeRequest(ctx, ten.ID, cr.ID)
	must(t, err)
	if len(got.Items) != 1 || got.Items[0].Key != rq.Key || got.Items[0].BaseVersion != rq.Version || string(got.Items[0].Proposed["title"]) != `"Log in"` ||
		string(got.Items[0].Current["title"]) != `"Login"` || got.Items[0].Outdated || len(got.Defects) != 1 || len(got.Links.Issues) != 1 || len(got.Events) != 1 || got.ContentHash == "" {
		t.Fatalf("%+v", got)
	}
	hash := got.ContentHash
	// add a test and replace the requirement's proposal
	must(t, st.PutChangeRequestItems(ctx, ten.ID, cr.ID, []model.ChangeRequestItem{
		{EntityType: model.KindTest, EntityID: tc.ID, Proposed: map[string]json.RawMessage{"status": raw(`"ready"`)}},
		{EntityType: model.KindRequirement, EntityID: rq.ID, Proposed: map[string]json.RawMessage{"title": raw(`"Sign in"`)}},
	}))
	got, err = st.GetChangeRequest(ctx, ten.ID, cr.ID)
	must(t, err)
	if len(got.Items) != 2 || string(got.Items[0].Proposed["title"]) != `"Sign in"` || got.ContentHash == hash {
		t.Fatalf("%+v", got.Items)
	}
	open, err := st.OpenChangeRequestsByItem(ctx, ten.ID, p.ID)
	must(t, err)
	if len(open["requirement:"+rq.ID]) != 1 || len(open["test:"+tc.ID]) != 1 {
		t.Fatal(open)
	}
	byItem, err := st.ListChangeRequests(ctx, ten.ID, p.ID, store.CRFilter{EntityType: model.KindTest, EntityID: tc.ID, Open: true})
	must(t, err)
	if len(byItem) != 1 {
		t.Fatal(len(byItem))
	}
	must(t, st.RemoveChangeRequestItem(ctx, ten.ID, cr.ID, got.Items[1].ID))
	if err := st.RemoveChangeRequestItem(ctx, ten.ID, cr.ID, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}

	// workflow
	if err := st.SetChangeRequestStatus(ctx, ten.ID, cr.ID, store.CRTransition{From: []string{"assessed"}, To: "in_review", By: "a@x.org"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("not assessed", err)
	}
	notes, effort := "Two tests to update", "2 days"
	must(t, st.SetChangeRequestStatus(ctx, ten.ID, cr.ID, store.CRTransition{From: []string{"draft"}, To: "submitted", By: "a@x.org"}))
	must(t, st.SetChangeRequestStatus(ctx, ten.ID, cr.ID, store.CRTransition{From: []string{"submitted"}, To: "assessed", By: "b@x.org",
		Impact: &model.ChangeImpact{Total: 3}, ImpactNotes: &notes, Effort: &effort}))
	if err := st.UpdateChangeRequest(ctx, ten.ID, got); !errors.Is(err, store.ErrConflict) {
		t.Fatal("only drafts change", err)
	}
	if err := st.PutChangeRequestItems(ctx, ten.ID, cr.ID, got.Items[:1]); !errors.Is(err, store.ErrConflict) {
		t.Fatal("only drafts change items", err)
	}
	must(t, st.SetChangeRequestStatus(ctx, ten.ID, cr.ID, store.CRTransition{From: []string{"assessed"}, To: "in_review", By: "b@x.org"}))
	got, err = st.GetChangeRequest(ctx, ten.ID, cr.ID)
	must(t, err)
	if got.Impact == nil || got.Impact.Total != 3 || got.Effort != "2 days" || got.AssessedBy != "b@x.org" || got.AssessedAt == nil {
		t.Fatalf("%+v", got)
	}
	a := &model.ChangeRequestSignoff{UserID: "u1", UserEmail: "c@x.org", Decision: "maybe"}
	if err := st.SignChangeRequest(ctx, ten.ID, got, a); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	a.Decision = "reject"
	if err := st.SignChangeRequest(ctx, ten.ID, got, a); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("reject needs a comment", err)
	}
	a.Decision = "approve"
	must(t, st.SignChangeRequest(ctx, ten.ID, got, a))
	if err := st.SignChangeRequest(ctx, ten.ID, got, &model.ChangeRequestSignoff{UserID: "u1", UserEmail: "c@x.org", Decision: "approve"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("signed twice", err)
	}
	must(t, st.SetChangeRequestStatus(ctx, ten.ID, cr.ID, store.CRTransition{From: []string{"in_review"}, To: "approved", By: "c@x.org", Comment: "ok"}))
	got, err = st.GetChangeRequest(ctx, ten.ID, cr.ID)
	must(t, err)
	if len(got.Approvals) != 1 || !got.Approvals[0].Valid || got.DecidedBy != "c@x.org" || got.Outdated {
		t.Fatalf("%+v", got)
	}
	if err := st.DeleteChangeRequest(ctx, ten.ID, cr.ID); !errors.Is(err, store.ErrConflict) {
		t.Fatal("approved is not deleted", err)
	}
	// the item changes outside: the change request is outdated
	rq.Title = "Changed elsewhere"
	must(t, st.UpdateRequirement(ctx, ten.ID, rq, false))
	got, err = st.GetChangeRequest(ctx, ten.ID, cr.ID)
	must(t, err)
	if !got.Outdated || !got.Items[0].Outdated {
		t.Fatal("outdated")
	}
	must(t, st.SetChangeRequestItemApplied(ctx, got.Items[0].ID, 5))
	for _, to := range []string{"implemented", "verified", "closed"} {
		from := map[string]string{"implemented": "approved", "verified": "implemented", "closed": "verified"}[to]
		must(t, st.SetChangeRequestStatus(ctx, ten.ID, cr.ID, store.CRTransition{From: []string{from}, To: to, By: "d@x.org", BaselineID: "b1"}))
	}
	got, err = st.GetChangeRequest(ctx, ten.ID, cr.ID)
	must(t, err)
	if got.Status != "closed" || got.AppliedBy != "d@x.org" || got.VerifiedAt == nil || got.BaselineID != "b1" || got.Items[0].AppliedVersion != 5 || got.Items[0].Outdated {
		t.Fatalf("%+v", got)
	}
	if open, _ = st.OpenChangeRequestsByItem(ctx, ten.ID, p.ID); len(open) != 0 {
		t.Fatal("closed is not open", open)
	}
	ids, err := st.ChangeRequestIDsInStatus(ctx, ten.ID, "closed")
	must(t, err)
	if len(ids) != 1 {
		t.Fatal(ids)
	}
	// a draft is deleted
	dr := &model.ChangeRequest{ProjectID: p.ID, Title: "Later"}
	must(t, st.CreateChangeRequest(ctx, ten.ID, dr))
	if x, err := st.GetChangeRequestByKey(ctx, ten.ID, p.ID, dr.Key); err != nil || x.ID != dr.ID {
		t.Fatal(err)
	}
	must(t, st.DeleteChangeRequest(ctx, ten.ID, dr.ID))
	if _, err := st.GetChangeRequest(ctx, ten.ID, dr.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	pid, version, status, key, err := st.ItemState(ctx, ten.ID, model.KindRequirement, rq.ID)
	if err != nil || pid != p.ID || version < 2 || status != "approved" || key != rq.Key {
		t.Fatal(pid, version, status, key, err)
	}
}

func TestSignedBaselinesOf(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "SB", Name: "Signed"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	rq := &model.Requirement{ProjectID: p.ID, Title: "R"}
	must(t, st.CreateRequirement(ctx, ten.ID, rq))
	b := &model.Baseline{ProjectID: p.ID, Name: "v1"}
	must(t, st.CreateBaseline(ctx, ten.ID, b, []model.BaselineItem{{EntityType: model.KindRequirement, EntityID: rq.ID, Key: rq.Key, Title: rq.Title, Version: 1, Snapshot: json.RawMessage(`{}`)}}))
	names, err := st.SignedBaselinesOf(ctx, ten.ID, model.KindRequirement, rq.ID)
	must(t, err)
	if len(names) != 0 {
		t.Fatal("unsigned", names)
	}
	must(t, st.SignBaseline(ctx, ten.ID, &model.Signoff{BaselineID: b.ID, UserID: "u", UserEmail: "u@x.org", Meaning: "approved", ContentHash: "h"}))
	names, err = st.SignedBaselinesOf(ctx, ten.ID, model.KindRequirement, rq.ID)
	must(t, err)
	if len(names) != 1 || names[0] != "v1" {
		t.Fatal(names)
	}
}
