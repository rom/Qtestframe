package app

import (
	"context"
	"strings"

	"github.com/rom/qtestframe/internal/catalog"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// seedCatalogRisks adds risks of the built-in catalogue to the demo
// project, as a catalogue import would: each with its catalogue origin,
// linked to demo requirements that mitigate it, and one catalogue
// requirement (with its origin) that mitigates the first of them. The
// risk register can then filter the heat map by catalogue area.
func seedCatalogRisks(ctx context.Context, st *store.Store, tenantID, projectID string, folder *model.Folder, reqs []*model.Requirement) error {
	c, err := catalog.Default()
	if err != nil {
		return err
	}
	byTitle := map[string]string{}
	for _, r := range reqs {
		byTitle[r.Title] = r.ID
	}
	// a catalogue requirement that mitigates credential stuffing
	var extra []string
	if rq := c.Requirements["AUTH-02"]; rq != nil {
		x := rq.Localized("en")
		m := &model.Requirement{ProjectID: projectID, FolderID: &folder.ID, Title: x.Title, Description: x.Description, Obligation: rq.Obligation,
			Type: rq.Type, Priority: rq.Priority, Status: "draft", Rationale: x.Rationale, Source: strings.Join(rq.Refs, "; "),
			Tags: append([]string{}, rq.Tags...), CatalogRef: rq.ID, CreatedBy: "seed"}
		for _, cr := range x.Criteria {
			m.Criteria = append(m.Criteria, model.EvaluationCriterion{Description: cr.Text, Method: cr.Method, Threshold: cr.Threshold})
		}
		if err := st.CreateRequirement(ctx, tenantID, m); err != nil {
			return err
		}
		extra = append(extra, m.ID)
	}
	for _, s := range []struct {
		id, status string
		reqs       []string
	}{
		{"AUTH-R01", "mitigating", []string{"Account lockout", "Password login"}},
		{"PAY-R03", "identified", []string{"Transfer limits"}},
		{"SLOG-R01", "mitigating", []string{"Central logging"}},
		{"AVL-R01", "identified", nil},
	} {
		k := c.Risks[s.id]
		if k == nil {
			continue
		}
		x := k.Localized("en")
		cat := k.Category
		if rc := catalog.RiskCategory(k.Category); rc != nil {
			cat = rc.Name.Get("en")
		}
		r := &model.Risk{ProjectID: projectID, Title: x.Title, Description: x.Description, Mitigation: x.Mitigation, Category: cat,
			Likelihood: k.Likelihood, Impact: k.Impact, Status: s.status, CatalogRef: k.ID, CreatedBy: "seed"}
		for _, t := range s.reqs {
			if id := byTitle[t]; id != "" {
				r.RequirementIDs = append(r.RequirementIDs, id)
			}
		}
		if s.id == "AUTH-R01" {
			r.RequirementIDs = append(r.RequirementIDs, extra...)
		}
		if err := st.CreateRisk(ctx, tenantID, r); err != nil {
			return err
		}
	}
	return nil
}
