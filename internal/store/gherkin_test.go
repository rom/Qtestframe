package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

const bddFeature = `@shop
Feature: Checkout
  Background:
    Given a customer with a saved card

  Scenario: Pay
    When the customer pays 25 EUR
    Then the order is "paid"

  Scenario Outline: Pay <amount>
    When the customer pays <amount> EUR
    Then the order is "paid"

    Examples:
      | amount |
      | 5      |
      | 10     |
`

func TestGherkinTestsAndRuns(t *testing.T) {
	st, ten, ctx := setup(t)
	pr := &model.Project{Key: "SHOP", Name: "Shop"}
	must(t, st.CreateProject(ctx, ten.ID, pr, true))

	// formats
	if err := st.CreateTest(ctx, ten.ID, &model.TestCase{ProjectID: pr.ID, Title: "x", Format: "yaml"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	bad := &model.TestCase{ProjectID: pr.ID, Title: "x", Format: model.FormatGherkin, Gherkin: "Feature: a\n  Scenario: s\n    Given x\n    nope\n    nope\n    nope\n    nope\n"}
	err := st.CreateTest(ctx, ten.ID, bad)
	var ge *store.GherkinError
	if !errors.As(err, &ge) || !errors.Is(err, store.ErrInvalid) || ge.Errors[0].Line != 4 || !strings.Contains(err.Error(), "(1 more)") {
		t.Fatal(err)
	}
	if err := st.CreateTest(ctx, ten.ID, &model.TestCase{ProjectID: pr.ID, Title: "x", Format: model.FormatGherkin, Gherkin: strings.Repeat("a", 1<<20+1)}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	draft := &model.TestCase{ProjectID: pr.ID, Title: "draft", Format: model.FormatGherkin}
	must(t, st.CreateTest(ctx, ten.ID, draft))
	plain := &model.TestCase{ProjectID: pr.ID, Title: "plain", Steps: []model.TestStep{{Action: "a"}}}
	must(t, st.CreateTest(ctx, ten.ID, plain))
	if got, _ := st.GetTest(ctx, ten.ID, plain.ID); got.Format != model.FormatSteps {
		t.Fatal(got.Format)
	}
	tc := &model.TestCase{ProjectID: pr.ID, Title: "Checkout", Format: model.FormatGherkin, Gherkin: strings.ReplaceAll(bddFeature, "\n", "\r\n")}
	must(t, st.CreateTest(ctx, ten.ID, tc))
	got, err := st.GetTest(ctx, ten.ID, tc.ID)
	must(t, err)
	if got.Format != model.FormatGherkin || got.Gherkin != bddFeature {
		t.Fatalf("%q", got.Gherkin)
	}

	// a run: one iteration per scenario and Examples row
	run := &model.TestRun{ProjectID: pr.ID, Name: "r"}
	must(t, st.CreateRun(ctx, ten.ID, run, []string{tc.ID, draft.ID, plain.ID}))
	execs, err := st.ListExecutions(ctx, ten.ID, run.ID)
	must(t, err)
	var execID string
	for _, e := range execs {
		if e.TestCaseID == tc.ID {
			execID = e.ID
		}
	}
	e, err := st.GetExecution(ctx, ten.ID, execID)
	must(t, err)
	if len(e.Iterations) != 3 || e.Iterations[0].Name != "Pay" || e.Iterations[1].Name != "Pay 5" || e.Iterations[2].Values["amount"] != "10" {
		t.Fatalf("%+v", e.Iterations)
	}
	if len(e.StepResults) != 9 || e.StepResults[0].Action != "Given a customer with a saved card" || e.StepResults[0].StepID != store.GherkinStepID(4) {
		t.Fatalf("%+v", e.StepResults)
	}
	if e.StepResults[4].Action != "When the customer pays 5 EUR" || e.StepResults[4].StepID != "gherkin:11" {
		t.Fatalf("%+v", e.StepResults[4])
	}

	// no results yet
	if id, res, err := st.LastGherkinResults(ctx, ten.ID, tc.ID); err != nil || id != "" || len(res) != 0 {
		t.Fatal(id, res, err)
	}
	// results map back to the step lines
	var srs []model.StepResult
	for i, sr := range e.StepResults {
		sr.Status = model.ResultPassed
		if i == 7 { // When of the second row (line 11)
			sr.Status = model.ResultFailed
		}
		srs = append(srs, sr)
	}
	_, err = st.UpdateExecution(ctx, ten.ID, execID, "a@x.org", store.ExecutionUpdate{StepResults: srs})
	must(t, err)
	id, res, err := st.LastGherkinResults(ctx, ten.ID, tc.ID)
	must(t, err)
	byLine := map[int]store.GherkinStepResult{}
	for _, r := range res {
		byLine[r.Line] = r
	}
	if id != execID || byLine[4].Results != 3 || byLine[4].Status != model.ResultPassed || byLine[11].Status != model.ResultFailed || byLine[11].Results != 2 {
		t.Fatalf("%+v", res)
	}

	// a test that does not parse any more cannot be added to a run
	if _, err := st.DB.ExecContext(ctx, st.DB.Rebind(`UPDATE test_cases SET gherkin = 'oops' WHERE id = ?`), draft.ID); err != nil {
		t.Fatal(err)
	}
	run2 := &model.TestRun{ProjectID: pr.ID, Name: "r2"}
	if err := st.CreateRun(ctx, ten.ID, run2, []string{draft.ID}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestStepLibrary(t *testing.T) {
	st, ten, ctx := setup(t)
	pr := &model.Project{Key: "SHOP", Name: "Shop"}
	must(t, st.CreateProject(ctx, ten.ID, pr, true))
	tc := &model.TestCase{ProjectID: pr.ID, Title: "Checkout", Format: model.FormatGherkin, Gherkin: bddFeature}
	must(t, st.CreateTest(ctx, ten.ID, tc))
	other := &model.TestCase{ProjectID: pr.ID, Title: "Other", Format: model.FormatGherkin, Gherkin: "Feature: o\n  Scenario: s\n    When the customer pays 3 EUR\n"}
	must(t, st.CreateTest(ctx, ten.ID, other))

	for _, d := range []*model.StepDefinition{
		{ProjectID: pr.ID, Pattern: ""},
		{ProjectID: pr.ID, Pattern: "x {nope}"},
		{ProjectID: pr.ID, Pattern: "x", Keyword: "maybe"},
		{ProjectID: pr.ID, Pattern: "x\ny"},
		{ProjectID: pr.ID, Pattern: strings.Repeat("x", store.MaxStepPattern+1)},
		{ProjectID: pr.ID, Pattern: "x", Description: strings.Repeat("x", store.MaxStepDescription+1)},
	} {
		if err := st.CreateStepDefinition(ctx, ten.ID, d); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("%q: %v", d.Pattern, err)
		}
	}
	if err := st.CreateStepDefinition(ctx, ten.ID, &model.StepDefinition{ProjectID: "missing", Pattern: "x"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	pay := &model.StepDefinition{ProjectID: pr.ID, Pattern: "the customer pays {int} EUR", Keyword: "When", Description: "Pays", CreatedBy: "a@x.org"}
	must(t, st.CreateStepDefinition(ctx, ten.ID, pay))
	if pay.Keyword != "when" || pay.Kind != "cucumber" {
		t.Fatal(pay)
	}
	if err := st.CreateStepDefinition(ctx, ten.ID, &model.StepDefinition{ProjectID: pr.ID, Pattern: pay.Pattern}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	card := &model.StepDefinition{ProjectID: pr.ID, Pattern: `^a customer with a (\w+) card$`, Kind: "regex"}
	must(t, st.CreateStepDefinition(ctx, ten.ID, card))

	lib, err := st.StepLibraryUsage(ctx, ten.ID, pr.ID)
	must(t, err)
	if len(lib.Steps) != 2 || len(lib.Uses[pay.ID]) != 3 || len(lib.Uses[card.ID]) != 1 {
		t.Fatalf("%+v", lib)
	}
	for _, d := range lib.Steps {
		if d.ID == pay.ID && d.UsageCount != 3 {
			t.Fatal(d)
		}
	}
	if len(lib.Undefined) != 1 || lib.Undefined[0].Text != `the order is "paid"` || len(lib.Undefined[0].Uses) != 2 || lib.Undefined[0].Suggestion != "the order is {string}" {
		t.Fatalf("%+v", lib.Undefined)
	}

	// update and errors
	card.Description = "A card"
	must(t, st.UpdateStepDefinition(ctx, ten.ID, card))
	if got, _ := st.GetStepDefinition(ctx, ten.ID, card.ID); got.Description != "A card" {
		t.Fatal(got)
	}
	card.Pattern = pay.Pattern
	if err := st.UpdateStepDefinition(ctx, ten.ID, card); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	if err := st.UpdateStepDefinition(ctx, ten.ID, &model.StepDefinition{ID: "missing", Pattern: "x"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if err := st.UpdateStepDefinition(ctx, ten.ID, &model.StepDefinition{ID: card.ID, Pattern: ""}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}

	// rename across tests
	if _, _, err := st.RenameStepDefinition(ctx, ten.ID, pay.ID, "the customer pays {int} {word}"); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if _, _, err := st.RenameStepDefinition(ctx, ten.ID, pay.ID, "bad {x}"); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if _, _, err := st.RenameStepDefinition(ctx, ten.ID, "missing", "x"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	nd, changed, err := st.RenameStepDefinition(ctx, ten.ID, pay.ID, "the customer pays {int} euro(s)")
	must(t, err)
	if nd.Pattern != "the customer pays {int} euro(s)" || len(changed) != 2 || changed[0].Steps != 2 || changed[1].Steps != 1 {
		t.Fatalf("%+v %+v", nd, changed)
	}
	got, _ := st.GetTest(ctx, ten.ID, tc.ID)
	if !strings.Contains(got.Gherkin, "When the customer pays 25 euros") || !strings.Contains(got.Gherkin, "When the customer pays <amount> euros") || got.Version != 2 {
		t.Fatalf("%d %s", got.Version, got.Gherkin)
	}
	lib, _ = st.StepLibraryUsage(ctx, ten.ID, pr.ID)
	if len(lib.Uses[pay.ID]) != 3 {
		t.Fatalf("%+v", lib.Uses)
	}

	// delete
	must(t, st.DeleteStepDefinition(ctx, ten.ID, card.ID))
	if err := st.DeleteStepDefinition(ctx, ten.ID, card.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.GetStepDefinition(ctx, ten.ID, card.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
}
