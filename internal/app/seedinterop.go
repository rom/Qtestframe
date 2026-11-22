package app

import (
	"context"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// seedInterop shows the OSLC links in the demo: a requirement implemented
// by a work item of an ALM tool and elaborating a customer requirement
// there, and a test case that tests a change request, as item links
// labelled with the OSLC property (what an OSLC client writes, see
// internal/api/oslc_write.go). The OSLC resources show them as
// oslc_rm:implementedBy, oslc_rm:elaborates and oslc_qm:testsChangeRequest.
func seedInterop(ctx context.Context, st *store.Store, tenantID string, reqs []*model.Requirement, testIDs []string) error {
	if len(reqs) == 0 || len(testIDs) == 0 {
		return nil
	}
	for _, l := range []model.ItemLink{
		{SourceType: model.KindRequirement, SourceID: reqs[0].ID, TargetType: "url", Label: "oslc_rm:implementedBy",
			URL: "https://alm.example.com/ccm/resource/itemName/com.ibm.team.workitem.WorkItem/1042"},
		{SourceType: model.KindRequirement, SourceID: reqs[0].ID, TargetType: "url", Label: "oslc_rm:elaborates",
			URL: "https://alm.example.com/rm/resources/TX_customer-requirement-17"},
		{SourceType: model.KindTest, SourceID: testIDs[0], TargetType: "url", Label: "oslc_qm:testsChangeRequest",
			URL: "https://alm.example.com/ccm/resource/itemName/com.ibm.team.workitem.WorkItem/1043"},
	} {
		l.CreatedBy = "seed"
		if err := st.CreateItemLink(ctx, tenantID, &l); err != nil {
			return err
		}
	}
	return nil
}
