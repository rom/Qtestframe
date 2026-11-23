package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// Change and defect correlations (see model.CorrelationSettings):
//
//   - change impact: when the substance of a requirement changes, the
//     requirements related to it along the configured relation types become
//     suspect (the relation is marked), with their verifying tests (their
//     trace links are marked, see MarkLinksSuspect); its tracker issues are
//     flagged out of date and the generated documents containing it are
//     outdated. The change travels on transitively (impact.go) up to the
//     project's impact depth; ImpactIndex previews all of it before a
//     change is saved.
//   - result staleness: staleness.go.
//   - blocking defects: DefectBlocks.
//   - regressions: regressions.go.

// ---------------------------------------------------------------- settings

// CorrelationSettings returns a project's settings (the defaults where it
// has none).
func (s *Store) CorrelationSettings(ctx context.Context, tenantID, projectID string) (*model.CorrelationSettings, error) {
	var raw string
	if err := s.c(ctx).row(ctx, `SELECT correlation_settings FROM projects WHERE id = ? AND tenant_id = ?`, projectID, tenantID).Scan(&raw); err != nil {
		return nil, mapErr(err, "project")
	}
	set := model.DefaultCorrelationSettings()
	if strings.TrimSpace(raw) != "" && raw != "{}" {
		// stored settings always list every relation type (see Save), so
		// merging into the default map loses nothing
		_ = json.Unmarshal([]byte(raw), &set)
	}
	if set.BlockingSeverities == nil {
		set.BlockingSeverities = []string{}
	}
	if set.WarningSeverities == nil {
		set.WarningSeverities = []string{}
	}
	fillImpactRules(&set)
	set.ApplicabilityImpact = set.ApplicabilityMode()
	return &set, nil
}

// MaxStaleAfterDays bounds CorrelationSettings.StaleAfterDays.
const MaxStaleAfterDays = 3650

// NormalizeCorrelationSettings validates settings and lists every relation
// type in RelationImpact.
func NormalizeCorrelationSettings(set *model.CorrelationSettings) error {
	impact := map[string]string{}
	for typ, dir := range set.RelationImpact {
		if !slices.Contains(model.RelationTypes, typ) {
			return Invalidf("unknown relation type %q", typ)
		}
		if dir == "" {
			dir = model.ImpactOff
		}
		if !slices.Contains(model.ImpactDirections, dir) {
			return Invalidf("relation_impact of %s must be one of %s", typ, strings.Join(model.ImpactDirections, ", "))
		}
		impact[typ] = dir
	}
	for _, typ := range model.RelationTypes {
		if _, ok := impact[typ]; !ok {
			impact[typ] = model.ImpactOff
		}
	}
	set.RelationImpact = impact
	if set.StaleAfterDays < 0 || set.StaleAfterDays > MaxStaleAfterDays {
		return Invalidf("stale_after_days must be between 0 (off) and %d", MaxStaleAfterDays)
	}
	clean := func(list []string) ([]string, error) {
		out := []string{}
		for _, v := range list {
			if !slices.Contains(model.DefectSeverities, v) {
				return nil, Invalidf("unknown defect severity %q", v)
			}
			if !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
		return out, nil
	}
	var err error
	if set.BlockingSeverities, err = clean(set.BlockingSeverities); err != nil {
		return err
	}
	if set.WarningSeverities, err = clean(set.WarningSeverities); err != nil {
		return err
	}
	for _, v := range set.WarningSeverities {
		if slices.Contains(set.BlockingSeverities, v) {
			return Invalidf("severity %s cannot both block and warn", v)
		}
	}
	if set.RegressionAction == "" {
		set.RegressionAction = model.RegressionReopen
	}
	if !slices.Contains(model.RegressionActions, set.RegressionAction) {
		return Invalidf("regression_action must be one of %s", strings.Join(model.RegressionActions, ", "))
	}
	if set.ImpactDepth == 0 {
		set.ImpactDepth = model.DefaultImpactDepth
	}
	if set.ImpactDepth < 1 || set.ImpactDepth > model.MaxImpactDepth {
		return Invalidf("impact_depth must be between 1 and %d", model.MaxImpactDepth)
	}
	modes := map[string]string{}
	for typ, m := range set.RelationModes {
		if !slices.Contains(model.RelationTypes, typ) {
			return Invalidf("unknown relation type %q", typ)
		}
		if m == "" {
			m = model.ModeSuspect
		}
		if !slices.Contains(model.ImpactModes, m) {
			return Invalidf("relation_modes of %s must be one of %s", typ, strings.Join(model.ImpactModes, ", "))
		}
		modes[typ] = m
	}
	for _, typ := range model.RelationTypes {
		if _, ok := modes[typ]; !ok {
			modes[typ] = model.ModeSuspect
		}
	}
	set.RelationModes = modes
	for edge, r := range set.ImpactRules {
		if !slices.Contains(model.ImpactEdgeKinds, edge) {
			return Invalidf("unknown impact rule %q (one of %s)", edge, strings.Join(model.ImpactEdgeKinds, ", "))
		}
		if r.Direction == "" {
			r.Direction = model.ImpactOff
		}
		if !slices.Contains(model.ImpactDirections, r.Direction) {
			return Invalidf("impact_rules.%s.direction must be one of %s", edge, strings.Join(model.ImpactDirections, ", "))
		}
		if r.Mode == "" {
			r.Mode = model.ModeReview
		}
		if !slices.Contains(model.ImpactModes, r.Mode) {
			return Invalidf("impact_rules.%s.mode must be one of %s", edge, strings.Join(model.ImpactModes, ", "))
		}
		if r.Mode == model.ModeSuspect && !model.Markable(edge) {
			return Invalidf("impact_rules.%s can only recommend a review (mode review)", edge)
		}
		set.ImpactRules[edge] = r
	}
	if set.ApplicabilityImpact == "" {
		set.ApplicabilityImpact = model.ModeReview
	}
	if !slices.Contains(model.ApplicabilityImpacts, set.ApplicabilityImpact) {
		return Invalidf("applicability_impact must be one of %s", strings.Join(model.ApplicabilityImpacts, ", "))
	}
	fillImpactRules(set)
	return nil
}

// fillImpactRules gives settings stored before the transitive impact
// their default rules and relation modes.
func fillImpactRules(set *model.CorrelationSettings) {
	if set.ImpactRules == nil {
		set.ImpactRules = map[string]model.ImpactRule{}
	}
	for edge, r := range model.DefaultImpactRules() {
		if _, ok := set.ImpactRules[edge]; !ok {
			set.ImpactRules[edge] = r
		}
	}
	if set.RelationModes == nil {
		set.RelationModes = map[string]string{}
	}
	for _, typ := range model.RelationTypes {
		if set.RelationModes[typ] == "" {
			set.RelationModes[typ] = model.ModeSuspect
		}
	}
	if set.ImpactDepth <= 0 {
		set.ImpactDepth = model.DefaultImpactDepth
	}
}

// SaveCorrelationSettings validates and stores a project's settings.
func (s *Store) SaveCorrelationSettings(ctx context.Context, tenantID, projectID string, set *model.CorrelationSettings) error {
	if err := NormalizeCorrelationSettings(set); err != nil {
		return err
	}
	b, _ := json.Marshal(set)
	res, err := s.c(ctx).exec(ctx, `UPDATE projects SET correlation_settings = ? WHERE id = ? AND tenant_id = ?`, string(b), projectID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "project")
	}
	return nil
}

// ---------------------------------------------------------------- substance changes

// MarkContentChanged records that the substance of a requirement or test
// changed now (results before it become stale, documents outdated).
func (s *Store) MarkContentChanged(ctx context.Context, tenantID, kind, id string) error {
	table := "requirements"
	if kind == model.KindTest {
		table = "test_cases"
	}
	_, err := s.c(ctx).exec(ctx, `UPDATE `+table+` SET content_changed_at = ? WHERE id = ? AND tenant_id = ?`, s.now(), id, tenantID)
	return err
}

// ---------------------------------------------------------------- suspect relations

// ImpactedRelation is a relation along which a change of a requirement
// travels, and the requirement at the other end, which becomes suspect.
type ImpactedRelation struct {
	Relation model.Relation
	// OtherID/Key/Title: the requirement that becomes suspect.
	OtherID, OtherKey, OtherTitle string
	// Direction of the relation seen from the changed requirement:
	// incoming (the other is the source) | outgoing (the other is the target).
	Direction string
	// End: the suspect end of the relation, source | target.
	End string
}

// ImpactedRelations returns the relations of rels along which a change of
// requirement reqID travels under the settings.
func ImpactedRelations(rels []model.Relation, reqID string, set *model.CorrelationSettings) []ImpactedRelation {
	var out []ImpactedRelation
	for _, r := range rels {
		dir := set.ImpactDirection(r.Type)
		if r.TargetID == reqID && r.SourceID != reqID && (dir == model.ImpactIncoming || dir == model.ImpactBoth) {
			out = append(out, ImpactedRelation{Relation: r, OtherID: r.SourceID, OtherKey: r.SourceKey, OtherTitle: r.SourceTitle, Direction: "incoming", End: "source"})
		}
		if r.SourceID == reqID && r.TargetID != reqID && (dir == model.ImpactOutgoing || dir == model.ImpactBoth) {
			out = append(out, ImpactedRelation{Relation: r, OtherID: r.TargetID, OtherKey: r.TargetKey, OtherTitle: r.TargetTitle, Direction: "outgoing", End: "target"})
		}
	}
	return out
}

// ConfirmRelations clears the suspect mark of relations: by ID, or all
// suspect relations of a requirement. Returns the confirmed relations.
func (s *Store) ConfirmRelations(ctx context.Context, tenantID string, ids []string, reqID string) ([]model.Relation, error) {
	where := `tenant_id = ? AND suspect_since IS NOT NULL`
	args := []any{tenantID}
	switch {
	case len(ids) > 0:
		where += ` AND id IN (` + placeholders(len(ids)) + `)`
		args = append(args, anySlice(ids)...)
	case reqID != "":
		where += ` AND (source_id = ? OR target_id = ?)`
		args = append(args, reqID, reqID)
	default:
		return nil, Invalidf("nothing to confirm")
	}
	rows, err := s.c(ctx).query(ctx, `SELECT id FROM requirement_relations WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	found, err := scanStrings(rows)
	if err != nil {
		return nil, err
	}
	out := []model.Relation{}
	for _, id := range found {
		rel, err := s.GetRelation(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *rel)
	}
	if len(found) == 0 {
		return out, nil
	}
	_, err = s.c(ctx).exec(ctx, `UPDATE requirement_relations SET suspect_since = NULL, suspect_reason = '', suspect_end = '' WHERE `+where, args...)
	return out, err
}

// SuspectRelationCounts returns, per requirement of a project, the number
// of its relations that are suspect with it as the suspect end.
func (s *Store) SuspectRelationCounts(ctx context.Context, tenantID, projectID string) (map[string]int, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT CASE WHEN suspect_end = 'target' THEN target_id ELSE source_id END FROM requirement_relations
		WHERE tenant_id = ? AND project_id = ? AND suspect_since IS NOT NULL`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	ids, err := scanStrings(rows)
	out := map[string]int{}
	for _, id := range ids {
		out[id]++
	}
	return out, err
}

// ---------------------------------------------------------------- tracker issues

// ClearIssueOutdated clears the out-of-date flag of an issue link (the
// issue was updated from its requirement, or someone confirmed it).
func (s *Store) ClearIssueOutdated(ctx context.Context, tenantID, linkID string) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE item_links SET issue_outdated_since = NULL, issue_outdated_reason = '' WHERE tenant_id = ? AND id = ?`, tenantID, linkID)
	return err
}

// ---------------------------------------------------------------- documents

// DocumentItem is a requirement or test contained in a generated document.
type DocumentItem struct {
	EntityType, EntityID string
	Version              int
}

// SetDocumentItems records what a generated document contains.
func (s *Store) SetDocumentItems(ctx context.Context, fileID string, items []DocumentItem) error {
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM document_items WHERE file_id = ?`, fileID); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, it := range items {
			k := it.EntityType + ":" + it.EntityID
			if seen[k] {
				continue
			}
			seen[k] = true
			if _, err := c.exec(ctx, `INSERT INTO document_items (file_id, entity_type, entity_id, version) VALUES (?, ?, ?, ?)`, fileID, it.EntityType, it.EntityID, it.Version); err != nil {
				return err
			}
		}
		return nil
	})
}

// documentEntry is one item of a generated document as the impact sees it.
type documentEntry struct {
	fileID, fileName       string
	kind, id, key          string
	created                time.Time
	changed                sql.NullTime
	testChanged            sql.NullTime
	itemVersion, atVersion int
}

func (s *Store) documentEntries(ctx context.Context, tenantID, projectID string) ([]documentEntry, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT d.file_id, f.name, f.created_at, d.entity_type, d.entity_id, d.version,
		COALESCE(r.key, t.key, ''), r.content_changed_at, t.content_changed_at, COALESCE(r.version, t.version, 0)
		FROM document_items d JOIN files f ON f.id = d.file_id
		LEFT JOIN requirements r ON d.entity_type = 'requirement' AND r.id = d.entity_id
		LEFT JOIN test_cases t ON d.entity_type = 'test' AND t.id = d.entity_id
		WHERE f.tenant_id = ? AND f.project_id = ? ORDER BY f.created_at, f.name`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []documentEntry
	for rows.Next() {
		var e documentEntry
		if err := rows.Scan(&e.fileID, &e.fileName, &e.created, &e.kind, &e.id, &e.atVersion, &e.key, &e.changed, &e.testChanged, &e.itemVersion); err != nil {
			return nil, err
		}
		if !e.changed.Valid {
			e.changed = e.testChanged
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// outdated: the item changed in substance after the document was generated.
func (e documentEntry) outdated() bool {
	return e.changed.Valid && e.changed.Time.After(e.created)
}

// OutdatedDocuments returns, per generated document of a project that is
// outdated, the keys of the items that changed after it was generated.
func (s *Store) OutdatedDocuments(ctx context.Context, tenantID, projectID string) (map[string][]string, error) {
	entries, err := s.documentEntries(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, e := range entries {
		if e.outdated() {
			out[e.fileID] = append(out[e.fileID], e.key)
		}
	}
	for k := range out {
		sort.Slice(out[k], func(i, j int) bool { return naturalLess(out[k][i], out[k][j]) })
	}
	return out, nil
}

// ---------------------------------------------------------------- blocking defects

// DefectBlocks returns, per requirement of a project, the open defects
// that block it or warn about it under the settings: defects linked to the
// requirement, to a test verifying it or to an execution of such a test.
// The strongest level per requirement and defect is kept; blockers first.
func (s *Store) DefectBlocks(ctx context.Context, tenantID, projectID string, set *model.CorrelationSettings) (map[string][]model.DefectBlock, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT d.id, d.key, d.title, d.severity, d.status, l.entity_type,
		COALESCE(tl.requirement_id, el.requirement_id, l.entity_id), COALESCE(t1.key, t2.key, '')
		FROM defects d JOIN defect_links l ON l.defect_id = d.id
		LEFT JOIN trace_links tl ON l.entity_type = 'test' AND tl.test_case_id = l.entity_id
		LEFT JOIN test_cases t1 ON l.entity_type = 'test' AND t1.id = l.entity_id
		LEFT JOIN test_executions e ON l.entity_type = 'execution' AND e.id = l.entity_id
		LEFT JOIN trace_links el ON el.test_case_id = e.test_case_id
		LEFT JOIN test_cases t2 ON t2.id = e.test_case_id
		WHERE d.tenant_id = ? AND d.project_id = ? AND d.status IN ('open', 'in_progress')
		AND (l.entity_type = 'requirement' OR tl.requirement_id IS NOT NULL OR el.requirement_id IS NOT NULL)`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	best := map[string]map[string]model.DefectBlock{}
	for rows.Next() {
		var b model.DefectBlock
		var typ, reqID string
		if err := rows.Scan(&b.DefectID, &b.Key, &b.Title, &b.Severity, &b.Status, &typ, &reqID, &b.Via); err != nil {
			return nil, err
		}
		if b.Level = set.DefectBlockLevel(b.Severity); b.Level == "" {
			continue
		}
		if typ == model.KindRequirement {
			b.Via = model.KindRequirement
		}
		if best[reqID] == nil {
			best[reqID] = map[string]model.DefectBlock{}
		}
		cur, ok := best[reqID][b.DefectID]
		if !ok || (cur.Level == model.DefectWarns && b.Level == model.DefectBlocks) || (b.Via == model.KindRequirement && cur.Via != model.KindRequirement) {
			best[reqID][b.DefectID] = b
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := map[string][]model.DefectBlock{}
	for rid, m := range best {
		list := make([]model.DefectBlock, 0, len(m))
		for _, b := range m {
			list = append(list, b)
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Level != list[j].Level {
				return list[i].Level == model.DefectBlocks
			}
			return naturalLess(list[i].Key, list[j].Key)
		})
		out[rid] = list
	}
	return out, nil
}

// BlockCounts splits the blocks of a requirement into blockers and warnings.
func BlockCounts(blocks []model.DefectBlock) (blocking, warnings int) {
	for _, b := range blocks {
		if b.Level == model.DefectBlocks {
			blocking++
		} else {
			warnings++
		}
	}
	return
}
