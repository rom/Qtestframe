package app

import (
	"context"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// seedExploratory shows exploratory testing in the demo project: a
// debriefed session that found a bug (with a defect raised from a note), a
// debriefed session that passed, a session under way and a planned one
// without tester, scoped to areas (folders), requirements and tests.
func seedExploratory(ctx context.Context, st *store.Store, clock *seedClock, tenantID string, p *model.Project, areas []*model.Folder, reqs []*model.Requirement, testIDs []string) error {
	if len(reqs) < 6 || len(testIDs) < 2 || len(areas) < 3 {
		return nil
	}
	// the sessions took place over the last days
	clock.at = clock.live.Now().Add(-100 * time.Hour).Truncate(time.Hour)
	defer func() { clock.at = time.Time{} }()
	scope := func(items ...[2]string) []model.ExploratoryScopeItem {
		out := []model.ExploratoryScopeItem{}
		for _, it := range items {
			out = append(out, model.ExploratoryScopeItem{Type: it[0], ID: it[1]})
		}
		return out
	}
	session := func(title, mission, tester string, box int, sc []model.ExploratoryScopeItem) (*model.ExploratorySession, error) {
		x := &model.ExploratorySession{ProjectID: p.ID, Title: title, Mission: mission, Tester: tester, TimeboxMinutes: box, Scope: sc, CreatedBy: "seed"}
		return x, st.CreateExploratorySession(ctx, tenantID, x)
	}
	note := func(x *model.ExploratorySession, typ, body string, after time.Duration) (*model.ExploratoryNote, error) {
		clock.advance(after)
		n := &model.ExploratoryNote{SessionID: x.ID, Type: typ, Body: body, CreatedBy: x.Tester}
		return n, st.AddExploratoryNote(ctx, tenantID, n)
	}
	timer := func(x *model.ExploratorySession, action string) error {
		_, err := st.ExploratoryTimer(ctx, tenantID, x.ID, action)
		return err
	}

	// 1. sign-in: found a bug, debriefed as failed
	login, err := session("Explore sign-in lockout with unusual input",
		"Explore the sign-in and the account lockout with unusual input (very long e-mail addresses, Unicode, pasted passwords, parallel tabs) to discover messages that reveal accounts or lockouts that do not hold.",
		DemoMemberEmail, 60, scope([2]string{"folder", areas[0].ID}, [2]string{model.KindRequirement, reqs[0].ID}, [2]string{model.KindRequirement, reqs[1].ID}, [2]string{model.KindTest, testIDs[0]}))
	if err != nil {
		return err
	}
	if err := timer(login, store.TimerStart); err != nil {
		return err
	}
	if _, err := note(login, "observation", "Set-up: test accounts unlocked, two browsers side by side.", 4*time.Minute); err != nil {
		return err
	}
	if _, err := note(login, "question", "Should a 300-character e-mail address be cut, or rejected with a message?", 9*time.Minute); err != nil {
		return err
	}
	bug, err := note(login, "issue", "Lockout counter resets when the same account signs in from a second tab\nThe 11th attempt in the second tab is accepted while the first tab shows the lockout.", 12*time.Minute)
	if err != nil {
		return err
	}
	if _, err := note(login, "idea", "Add a test case for parallel sessions to the lockout suite.", 6*time.Minute); err != nil {
		return err
	}
	clock.advance(9 * time.Minute)
	if err := timer(login, store.TimerStop); err != nil {
		return err
	}
	x, err := st.GetExploratorySession(ctx, tenantID, login.ID)
	if err != nil {
		return err
	}
	n, err := st.GetExploratoryNote(ctx, tenantID, login.ID, bug.ID)
	if err != nil {
		return err
	}
	d := &model.Defect{ProjectID: p.ID, Title: "Lockout counter resets in a second tab", Severity: "major", Priority: "high", FoundIn: "test", CreatedBy: DemoMemberEmail,
		Description: n.Body + "\n\n---\nFound in exploratory session " + x.Key + " " + x.Title + ".",
		Links:       []model.DefectLink{{EntityType: model.KindExploratory, EntityID: x.ID}, {EntityType: model.KindRequirement, EntityID: reqs[1].ID}}}
	if err := st.CreateDefect(ctx, tenantID, d); err != nil {
		return err
	}
	if err := st.SetNoteDefect(ctx, tenantID, n.ID, d.ID); err != nil {
		return err
	}
	n.Type = "bug"
	if err := st.UpdateExploratoryNote(ctx, tenantID, n); err != nil {
		return err
	}
	if _, err := st.DebriefExploratorySession(ctx, tenantID, login.ID, &model.ExploratoryDebrief{
		Summary:     "Lockout does not hold across parallel tabs (defect raised). Long and Unicode e-mail addresses are handled; the message for a cut address is unclear.",
		CoveragePct: 70, SetupPct: 10, TestingPct: 55, BugPct: 35, Outcome: "failed", By: "seed", Name: "Test lead",
		Comment: "Agreed: rerun after the fix, with a charter on session handling."}); err != nil {
		return err
	}

	// 2. statements: passed
	clock.advance(26 * time.Hour)
	stmt, err := session("Explore statement export formats",
		"Explore the statement export (PDF and CSV) with long periods, many transactions and special characters in payees to discover formatting and encoding problems.",
		DemoMemberEmail, 90, scope([2]string{"folder", areas[1].ID}, [2]string{model.KindRequirement, reqs[5].ID}))
	if err != nil {
		return err
	}
	if err := timer(stmt, store.TimerStart); err != nil {
		return err
	}
	if _, err := note(stmt, "observation", "CSV uses UTF-8 with a BOM; Excel opens it correctly.", 18*time.Minute); err != nil {
		return err
	}
	if _, err := note(stmt, "idea", "Offer a semicolon separator for Swedish locales.", 22*time.Minute); err != nil {
		return err
	}
	clock.advance(25 * time.Minute)
	if err := timer(stmt, store.TimerPause); err != nil {
		return err
	}
	clock.advance(time.Hour)
	if err := timer(stmt, store.TimerStart); err != nil {
		return err
	}
	clock.advance(15 * time.Minute)
	if err := timer(stmt, store.TimerStop); err != nil {
		return err
	}
	if _, err := st.DebriefExploratorySession(ctx, tenantID, stmt.ID, &model.ExploratoryDebrief{
		Summary:     "PDF and CSV exports are correct for 13 months and 5,000 transactions; payees with emoji and Å/Ä/Ö survive both formats.",
		CoveragePct: 85, SetupPct: 15, TestingPct: 80, BugPct: 5, Outcome: "passed", By: DemoMemberEmail, Name: "Dana Lind"}); err != nil {
		return err
	}

	// 3. transfer limits: under way
	clock.advance(20 * time.Hour)
	limits, err := session("Explore transfer limits around midnight",
		"Explore daily transfer limits around midnight and time-zone changes to discover limits that reset early or late.",
		DemoMemberEmail, 60, scope([2]string{"folder", areas[1].ID}, [2]string{model.KindRequirement, reqs[3].ID}, [2]string{model.KindTest, testIDs[1]}))
	if err != nil {
		return err
	}
	if err := timer(limits, store.TimerStart); err != nil {
		return err
	}
	if _, err := note(limits, "question", "Which time zone defines the day of the limit: the bank's or the customer's?", 7*time.Minute); err != nil {
		return err
	}
	if err := timer(limits, store.TimerPause); err != nil {
		return err
	}

	// 4. operations: planned, not assigned
	_, err = session("Explore audit logging under load",
		"Explore audit logging while the log server is slow or down to discover lost or duplicated audit events.",
		"", 30, scope([2]string{"folder", areas[2].ID}, [2]string{model.KindRequirement, reqs[6].ID}))
	return err
}
