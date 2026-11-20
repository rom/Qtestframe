package api

import (
	"net/http"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// Review decisions: setting a requirement to approved or rejected, or a test
// to ready, needs the review permission of its kind (requirement.review,
// test.review). A reviewer without write permission may change the status
// alone; any other change needs the write permission.
var reviewStatuses = map[string]map[string]bool{
	model.KindRequirement: {"approved": true, "rejected": true},
	model.KindTest:        {model.ReadyTestStatus: true},
}

func writePerm(kind string) rbac.Permission {
	if kind == model.KindRequirement {
		return rbac.RequirementWr
	}
	return rbac.TestWrite
}

func reviewPerm(kind string) rbac.Permission {
	if kind == model.KindRequirement {
		return rbac.RequirementRev
	}
	return rbac.TestReview
}

// isReviewDecision reports whether moving from status old to next is a
// review decision for kind.
func isReviewDecision(kind, old, next string) bool {
	return next != old && reviewStatuses[kind][next]
}

// authorizeChange checks an update of an item of kind: statusOnly reports
// that nothing but the status is changed. It writes 403 and returns false
// when a permission is missing.
func authorizeChange(w http.ResponseWriter, p *Principal, kind string, statusOnly bool, old, next string) bool {
	need := []rbac.Permission{}
	switch {
	case isReviewDecision(kind, old, next) && statusOnly:
		need = append(need, reviewPerm(kind))
	case isReviewDecision(kind, old, next):
		need = append(need, reviewPerm(kind), writePerm(kind))
	default:
		need = append(need, writePerm(kind))
	}
	for _, perm := range need {
		if !p.Can(perm) {
			msg := "permission " + string(perm) + " required"
			if perm == reviewPerm(kind) {
				msg += " to set the status to " + next
			}
			writeError(w, http.StatusForbidden, "forbidden", msg)
			return false
		}
	}
	return true
}

// reviewAction is the audit action of a review decision ("" when none).
func reviewAction(kind, old, next string) string {
	if !isReviewDecision(kind, old, next) {
		return ""
	}
	return kind + ".review"
}

// onlyKeys reports whether the patch has no keys other than allowed.
func onlyKeys(keys map[string]bool, allowed ...string) bool {
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	for k := range keys {
		if !ok[k] {
			return false
		}
	}
	return true
}
