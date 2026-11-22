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
	// catalogue requirements, with their D3FEND and EMB3D mappings: one that
	// mitigates credential stuffing, one for the parsers of an embedded device
	extra := map[string]string{}
	for _, id := range []string{"AUTH-02", "EMBS-26"} {
		rq := c.Requirements[id]
		if rq == nil {
			continue
		}
		x := rq.Localized("en")
		m := &model.Requirement{ProjectID: projectID, FolderID: &folder.ID, Title: x.Title, Description: x.Description, Obligation: rq.Obligation,
			Type: rq.Type, Priority: rq.Priority, Status: "draft", Rationale: x.Rationale, Source: strings.Join(rq.Refs, "; "),
			Tags: append([]string{}, rq.Tags...), CatalogRef: rq.ID, CreatedBy: "seed",
			D3fend: append([]string{}, rq.D3fend...), Emb3d: append([]string{}, rq.Emb3d...)}
		for _, cr := range x.Criteria {
			m.Criteria = append(m.Criteria, model.EvaluationCriterion{Description: cr.Text, Method: cr.Method, Threshold: cr.Threshold})
		}
		if err := st.CreateRequirement(ctx, tenantID, m); err != nil {
			return err
		}
		extra[id] = m.ID
	}
	for _, s := range []struct {
		id, status string
		reqs       []string
	}{
		{"AUTH-R01", "mitigating", []string{"Account lockout", "Password login"}},
		{"PAY-R03", "identified", []string{"Transfer limits"}},
		{"SLOG-R01", "mitigating", []string{"Central logging"}},
		{"AVL-R01", "identified", nil},
		{"EMBS-R03", "identified", nil},
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
			Likelihood: k.Likelihood, Impact: k.Impact, Status: s.status, CatalogRef: k.ID, CreatedBy: "seed", Attack: k.Techniques(), Emb3d: append([]string{}, k.Emb3d...)}
		for _, t := range s.reqs {
			if id := byTitle[t]; id != "" {
				r.RequirementIDs = append(r.RequirementIDs, id)
			}
		}
		if id := extra[map[string]string{"AUTH-R01": "AUTH-02", "EMBS-R03": "EMBS-26"}[s.id]]; id != "" {
			r.RequirementIDs = append(r.RequirementIDs, id)
		}
		if err := st.CreateRisk(ctx, tenantID, r); err != nil {
			return err
		}
	}
	return nil
}
