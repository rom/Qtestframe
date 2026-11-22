package app

import (
	"context"
	"encoding/json"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// seedDashboards gives the demo project a shared dashboard "Release health"
// next to the built-in ones: the readiness of the active release, the
// burn-down of the active test cycle, saved queries, traceability gaps,
// verification, framework coverage and a note.
func seedDashboards(ctx context.Context, st *store.Store, tenantID string, p *model.Project) error {
	w := func(id, typ string, x, y, ww, h int, title string, settings map[string]any) model.DashboardWidget {
		out := model.DashboardWidget{ID: id, Type: typ, Title: title, X: x, Y: y, W: ww, H: h}
		if len(settings) > 0 {
			out.Settings = map[string]json.RawMessage{}
			for k, v := range settings {
				b, _ := json.Marshal(v)
				out.Settings[k] = b
			}
		}
		return out
	}
	d := &model.Dashboard{
		ProjectID: p.ID,
		Name:      "Release health",
		CreatedBy: "seed",
		Widgets: []model.DashboardWidget{
			w("rh-note", "note", 0, 0, 4, 5, "About this dashboard", map[string]any{"text": "A **shared** dashboard of the demo project.\n\n" +
				"- *Customise* changes it (project administrators) or saves your own copy.\n" +
				"- The widgets show only what you may see.\n- Export it as JSON from the menu."}),
			w("rh-ready", "release_readiness", 4, 0, 4, 5, "", nil),
			w("rh-burndown", "cycle_burndown", 8, 0, 4, 5, "", nil),
			w("rh-must", "saved_query", 0, 5, 6, 4, "MUST requirements without passing tests", map[string]any{"entity": "requirements", "query": "obligation:must -coverage:passed", "limit": 8}),
			w("rh-defects", "saved_query", 6, 5, 3, 4, "Open critical and major defects", map[string]any{"entity": "defects", "query": "open:true (severity:critical OR severity:major)", "display": "count"}),
			w("rh-gaps", "traceability_gaps", 9, 5, 3, 4, "", nil),
			w("rh-verif", "verification_status", 0, 9, 4, 4, "", map[string]any{"chart": "bar"}),
			w("rh-defstatus", "breakdown", 4, 9, 4, 4, "", map[string]any{"dimension": "defect_status", "chart": "doughnut"}),
			w("rh-attack", "framework_coverage", 8, 9, 4, 4, "", map[string]any{"framework": "attack"}),
		},
	}
	return st.CreateDashboard(ctx, tenantID, d)
}
