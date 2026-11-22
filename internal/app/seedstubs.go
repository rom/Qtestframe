package app

import (
	"context"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/stubgen"
)

// seedStubTests adds two automated API tests for "Generate test code": a
// data-driven one without a script (what `has:!script` finds) and one whose
// script already refers to its generated pytest stub in the repository.
func seedStubTests(ctx context.Context, st *store.Store, tenantID string, p *model.Project, reqs []*model.Requirement) error {
	open := &model.TestCase{ProjectID: p.ID, Title: "Transfer API rejects invalid amounts", ExecutionType: model.ExecAutomated, Status: "ready", Priority: "high",
		CreatedBy: "seed", Tags: []string{model.ExecAutomated, "api"},
		Description:    "Automated in the API test suite; generate its code with Generate test code.",
		Preconditions:  "A customer with two accounts and a daily limit of 1000 is logged in.",
		ExpectedResult: "Invalid amounts are rejected with HTTP 422 and no money moves.",
		Steps: []model.TestStep{
			{Action: "POST /transfers with the amount {{amount}}", Expected: "HTTP 422 with the error {{error}}"},
			{Action: "GET /accounts", Expected: "Both balances are unchanged"},
		},
		Parameters: []string{"amount", "error"},
		DataRows: []model.DataRow{
			{Name: "negative", Values: map[string]string{"amount": "-10", "error": "amount_negative"}},
			{Name: "zero", Values: map[string]string{"amount": "0", "error": "amount_zero"}},
			{Name: "above limit", Values: map[string]string{"amount": "1500", "error": "daily_limit"}},
		}}
	stub := &model.TestCase{ProjectID: p.ID, Title: "Statement API lists the transactions", ExecutionType: model.ExecAutomated, Status: "ready",
		CreatedBy: "seed", Tags: []string{model.ExecAutomated, "api", "runner:pytest"}, ScriptLanguage: "python",
		Steps: []model.TestStep{{Action: "GET /accounts/{id}/statement", Expected: "The transactions of the month, newest first"}}}
	for i, t := range []*model.TestCase{open, stub} {
		if err := st.CreateTest(ctx, tenantID, t); err != nil {
			return err
		}
		if r := []int{2, 4}[i]; r < len(reqs) {
			if err := st.CreateLink(ctx, tenantID, &model.TraceLink{RequirementID: reqs[r].ID, TestCaseID: t.ID}); err != nil {
				return err
			}
		}
	}
	stub.Script = stubgen.ScriptRef("tests/" + stubgen.FileName("pytest", stub.Key))
	return st.UpdateTest(ctx, tenantID, stub, false)
}
