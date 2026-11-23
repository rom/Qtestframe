package api

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/interop"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqimport"
	"github.com/rom/qtestframe/internal/tracker"
)

// The field values of the two-way sync (reqsync.go). Both sides are
// compared in the requirement's terms: a tracker value is first mapped to
// what it would be as a requirement value (the status "In Progress"
// becomes approved), a requirement value is mapped to the tracker's value
// only when it is written. Values are canonical text: titles trimmed,
// tags sorted and joined with ", ", acceptance criteria one per line.

// defaultSyncFields is the default field mapping of a system.
func defaultSyncFields(system string) []model.ReqSyncField {
	if system == tracker.SourceAzure {
		return []model.ReqSyncField{
			{Field: "title", Remote: tracker.AzureTitle, Direction: model.SyncBoth},
			{Field: "description", Remote: "System.Description", Direction: model.SyncBoth},
			{Field: "status", Remote: "System.State", Direction: model.SyncBoth,
				Values:     map[string]string{"New": "draft", "Active": "approved", "Resolved": "implemented", "Closed": "verified", "Removed": "obsolete"},
				PushValues: map[string]string{"draft": "New", "review": "New", "approved": "Active", "implemented": "Resolved", "verified": "Closed", "rejected": "Removed", "obsolete": "Removed"}},
			{Field: "priority", Remote: "Microsoft.VSTS.Common.Priority", Direction: model.SyncBoth,
				Values:     map[string]string{"1": "critical", "2": "high", "3": "medium", "4": "low"},
				PushValues: map[string]string{"critical": "1", "high": "2", "medium": "3", "low": "4"}},
			{Field: "tags", Remote: "System.Tags", Direction: model.SyncBoth},
			{Field: "criteria", Remote: tracker.AzureCriteria, Direction: model.SyncBoth},
		}
	}
	return []model.ReqSyncField{
		{Field: "title", Remote: tracker.JiraSummary, Direction: model.SyncBoth},
		{Field: "description", Remote: tracker.JiraDescription, Direction: model.SyncBoth},
		{Field: "status", Remote: "status", Direction: model.SyncBoth,
			Values:     map[string]string{"To Do": "draft", "In Progress": "approved", "Done": "implemented"},
			PushValues: map[string]string{"draft": "To Do", "review": "To Do", "approved": "In Progress", "implemented": "Done", "verified": "Done", "rejected": "Done", "obsolete": "Done"}},
		{Field: "priority", Remote: "priority", Direction: model.SyncBoth,
			Values:     map[string]string{"Highest": "critical", "High": "high", "Medium": "medium", "Low": "low", "Lowest": "low"},
			PushValues: map[string]string{"critical": "Highest", "high": "High", "medium": "Medium", "low": "Low"}},
		{Field: "tags", Remote: "labels", Direction: model.SyncBoth},
	}
}

// DefaultSyncSettings are the settings of a source without a
// configuration (seed-demo starts from them).
func DefaultSyncSettings(system string) model.ReqSyncSettings { return defaultSyncSettings(system) }

// defaultSyncSettings are the settings of a source without a configuration.
func defaultSyncSettings(system string) model.ReqSyncSettings {
	return model.ReqSyncSettings{Fields: defaultSyncFields(system), ConflictPolicy: model.SyncManual, Impact: model.ImpactApplySuspect, MaxWritesPerMinute: 60}
}

func normText(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"))
}

func normTags(list []string) string {
	seen := map[string]bool{}
	var out []string
	for _, t := range list {
		t = strings.TrimSpace(t)
		if k := strings.ToLower(t); t != "" && !seen[k] {
			seen[k] = true
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return strings.Join(out, ", ")
}

// normCriteria makes criteria text canonical: list markers removed, one
// criterion per line in the form interop.CriteriaText writes.
func normCriteria(raw string) string {
	var ls []string
	for _, l := range strings.Split(normText(raw), "\n") {
		if l = strings.TrimSpace(bulletRe.ReplaceAllString(l, "")); l != "" {
			ls = append(ls, l)
		}
	}
	var cs []model.EvaluationCriterion
	for _, c := range reqimport.ParseCriteria(strings.Join(ls, "\n")) {
		cs = append(cs, model.EvaluationCriterion{Description: c.Description, Method: c.Method, Threshold: c.Threshold})
	}
	return interop.CriteriaText(cs)
}

// localSyncValue is a requirement's value of a field.
func localSyncValue(rq *model.Requirement, field string) string {
	switch field {
	case "title":
		return strings.TrimSpace(rq.Title)
	case "description":
		return normText(rq.Description)
	case "rationale":
		return normText(rq.Rationale)
	case "status":
		return rq.Status
	case "priority":
		return rq.Priority
	case "tags":
		return normTags(rq.Tags)
	case "criteria":
		return interop.CriteriaText(rq.Criteria)
	}
	return ""
}

// lookupFold finds a key case-insensitively.
func lookupFold(m map[string]string, k string) (string, bool) {
	if v, ok := m[k]; ok {
		return v, true
	}
	for x, v := range m {
		if strings.EqualFold(strings.TrimSpace(x), strings.TrimSpace(k)) {
			return v, true
		}
	}
	return "", false
}

// remoteSyncValue is a tracker item's value of a mapped field as a
// requirement value; ok is false when the value has no requirement value
// (a status nobody mapped): it is then compared as it is but never pulled.
func remoteSyncValue(it *tracker.SourceItem, f *model.ReqSyncField) (string, bool) {
	raw := it.Values[f.Remote]
	switch f.Field {
	case "title":
		return strings.Join(strings.Fields(raw), " "), true
	case "description", "rationale":
		return normText(raw), true
	case "status":
		if v, ok := lookupFold(f.Values, raw); ok && v != "" {
			return v, true
		}
		if raw == "" {
			return "", false
		}
		if v, ok := sourceStatus(raw, it.Category); ok {
			return v, true
		}
		return "?" + raw, false
	case "priority":
		if v, ok := lookupFold(f.Values, raw); ok && v != "" {
			return v, true
		}
		if raw == "" {
			return "", true
		}
		if v, ok := reqimport.ParsePriority(raw); ok && v != "" {
			return v, true
		}
		return "?" + raw, false
	case "tags":
		return normTags(strings.Split(raw, ",")), true
	case "criteria":
		return normCriteria(raw), true
	}
	return raw, true
}

// pushSyncValue is the tracker write of a requirement value.
func pushSyncValue(f *model.ReqSyncField, local string) tracker.FieldWrite {
	w := tracker.FieldWrite{ID: f.Remote, Kind: tracker.WriteText, Value: local}
	switch f.Field {
	case "description", "rationale":
		w.Kind = tracker.WriteMarkdown
	case "criteria":
		w.Kind = tracker.WriteMarkdown
		var ls []string
		for _, l := range strings.Split(local, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				ls = append(ls, "- "+l)
			}
		}
		w.Value = strings.Join(ls, "\n")
	case "tags":
		w.Kind = tracker.WriteList
	case "status", "priority":
		w.Kind = tracker.WriteOption
		if f.Field == "status" {
			w.Kind = tracker.WriteStatus
		}
		if v, ok := lookupFold(f.PushValues, local); ok && v != "" {
			w.Value = v
			break
		}
		// the first tracker value that maps to it
		keys := make([]string, 0, len(f.Values))
		for k := range f.Values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if f.Values[k] == local {
				w.Value = k
				break
			}
		}
	}
	return w
}

// localBody is the PATCH body that gives a requirement the values.
func localBody(rq *model.Requirement, values map[string]string) map[string]any {
	b := map[string]any{}
	for field, v := range values {
		switch field {
		case "tags":
			b["tags"] = interop.SplitList(v)
		case "criteria":
			out := []model.EvaluationCriterion{}
			for i, c := range reqimport.ParseCriteria(v) {
				cr := model.EvaluationCriterion{Description: c.Description, Method: c.Method, Threshold: c.Threshold}
				if rq != nil && i < len(rq.Criteria) {
					cr.ID = rq.Criteria[i].ID
				}
				out = append(out, cr)
			}
			b["criteria"] = out
		default:
			b[field] = v
		}
	}
	return b
}

// syncHash is the hash of mapped values.
func syncHash(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k + "=" + values[k] + "\x00"))
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// shorten cuts a value for the log.
func shorten(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
