package store

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/verification"
)

// Verification correlations: the criteria a trace link verifies
// (link_criteria), verification records (evidence other than test
// results), the project's verification settings, and the computation of
// "is requirement R verified, and why" (package verification).

// ---------------------------------------------------------------- link criteria

// linkCriteria returns link id → its criterion mappings for the links of
// a project (or one link), in criterion order.
func (s *Store) linkCriteria(ctx context.Context, where string, args ...any) (map[string][]model.LinkCriterion, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT lc.link_id, lc.criterion_id, lc.step_id FROM link_criteria lc
		JOIN trace_links l ON l.id = lc.link_id JOIN evaluation_criteria c ON c.id = lc.criterion_id
		WHERE `+where+` ORDER BY c.position`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]model.LinkCriterion{}
	for rows.Next() {
		var id string
		var lc model.LinkCriterion
		if err := rows.Scan(&id, &lc.CriterionID, &lc.StepID); err != nil {
			return nil, err
		}
		out[id] = append(out[id], lc)
	}
	return out, rows.Err()
}

// validLinkCriteria checks mappings against the requirement's criteria
// and the test's steps; duplicates are dropped.
func validLinkCriteria(r *model.Requirement, t *model.TestCase, in []model.LinkCriterion) ([]model.LinkCriterion, error) {
	out := []model.LinkCriterion{}
	seen := map[string]bool{}
	for _, lc := range in {
		if lc.CriterionID == "" || seen[lc.CriterionID] {
			continue
		}
		if !slices.ContainsFunc(r.Criteria, func(c model.EvaluationCriterion) bool { return c.ID == lc.CriterionID }) {
			return nil, Invalidf("criterion does not belong to requirement")
		}
		if lc.StepID != "" && !slices.ContainsFunc(t.Steps, func(st model.TestStep) bool { return st.ID == lc.StepID }) {
			return nil, Invalidf("step does not belong to the test")
		}
		seen[lc.CriterionID] = true
		out = append(out, lc)
	}
	return out, nil
}

func writeLinkCriteria(ctx context.Context, c conn, linkID string, list []model.LinkCriterion) error {
	if _, err := c.exec(ctx, `DELETE FROM link_criteria WHERE link_id = ?`, linkID); err != nil {
		return err
	}
	for _, lc := range list {
		if _, err := c.exec(ctx, `INSERT INTO link_criteria (link_id, criterion_id, step_id) VALUES (?, ?, ?)`, linkID, lc.CriterionID, lc.StepID); err != nil {
			return mapErr(err, "link criterion")
		}
	}
	var first any
	if len(list) > 0 {
		first = list[0].CriterionID
	}
	_, err := c.exec(ctx, `UPDATE trace_links SET criterion_id = ? WHERE id = ?`, first, linkID)
	return err
}

// SetLinkCriteria replaces the criteria a trace link verifies (each
// optionally through one step of the test). An empty list makes it a
// plain link again.
func (s *Store) SetLinkCriteria(ctx context.Context, tenantID, linkID string, list []model.LinkCriterion) (*model.TraceLink, error) {
	l, err := s.GetLink(ctx, tenantID, linkID)
	if err != nil {
		return nil, err
	}
	r, err := s.GetRequirement(ctx, tenantID, l.RequirementID)
	if err != nil {
		return nil, err
	}
	t, err := s.GetTest(ctx, tenantID, l.TestCaseID)
	if err != nil {
		return nil, err
	}
	list, err = validLinkCriteria(r, t, list)
	if err != nil {
		return nil, err
	}
	if err := s.tx(ctx, func(c conn) error { return writeLinkCriteria(ctx, c, linkID, list) }); err != nil {
		return nil, err
	}
	return s.GetLink(ctx, tenantID, linkID)
}

// ---------------------------------------------------------------- settings

// VerificationSettings returns a project's verification settings.
func (s *Store) VerificationSettings(ctx context.Context, tenantID, projectID string) (*model.VerificationSettings, error) {
	var v model.VerificationSettings
	var at sql.NullTime
	err := s.c(ctx).row(ctx, `SELECT strict_criteria, updated_by, updated_at FROM verification_settings WHERE project_id = ? AND tenant_id = ?`, projectID, tenantID).
		Scan(&v.StrictCriteria, &v.UpdatedBy, &at)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	v.UpdatedAt = timePtr(at)
	return &v, nil
}

// SetVerificationSettings stores a project's verification settings.
func (s *Store) SetVerificationSettings(ctx context.Context, tenantID, projectID, user string, v *model.VerificationSettings) error {
	if _, err := s.GetProject(ctx, tenantID, projectID); err != nil {
		return err
	}
	now := s.now()
	v.UpdatedBy, v.UpdatedAt = user, &now
	_, err := s.c(ctx).exec(ctx, `INSERT INTO verification_settings (project_id, tenant_id, strict_criteria, updated_by, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (project_id) DO UPDATE SET strict_criteria = excluded.strict_criteria, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		projectID, tenantID, v.StrictCriteria, user, now)
	return err
}

// ---------------------------------------------------------------- records

const recordCols = `v.id, v.project_id, v.requirement_id, v.criterion_id, v.method, v.result, v.performed_by, v.performed_on, v.note,
	v.valid_until, v.until_change, v.invalidated_at, v.invalidated_reason, v.signoff_by, v.signoff_name, v.signoff_comment, v.signoff_at,
	v.created_by, v.created_at, v.updated_at, r.key, COALESCE(c.description, '')`

const recordFrom = ` FROM verification_records v JOIN requirements r ON r.id = v.requirement_id LEFT JOIN evaluation_criteria c ON c.id = v.criterion_id`

func scanRecord(row interface{ Scan(...any) error }) (*model.VerificationRecord, error) {
	var v model.VerificationRecord
	var crit sql.NullString
	var inv, signAt sql.NullTime
	var sBy, sName, sComment string
	if err := row.Scan(&v.ID, &v.ProjectID, &v.RequirementID, &crit, &v.Method, &v.Result, &v.PerformedBy, &v.PerformedOn, &v.Note,
		&v.ValidUntil, &v.UntilChange, &inv, &v.InvalidatedReason, &sBy, &sName, &sComment, &signAt,
		&v.CreatedBy, &v.CreatedAt, &v.UpdatedAt, &v.RequirementKey, &v.CriterionText); err != nil {
		return nil, err
	}
	v.CriterionID = strPtr(crit)
	v.InvalidatedAt = timePtr(inv)
	if sBy != "" && signAt.Valid {
		v.Signoff = &model.RecordSignoff{By: sBy, Name: sName, Comment: sComment, At: signAt.Time}
	}
	v.FileIDs, v.Files = []string{}, []model.FileRef{}
	return &v, nil
}

// today is the store's current day (YYYY-MM-DD, UTC).
func (s *Store) today() string { return s.now().UTC().Format("2006-01-02") }

func validDate(d string) bool {
	if d == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", d)
	return err == nil && len(d) == 10
}

func (s *Store) validateRecord(ctx context.Context, tenantID string, v *model.VerificationRecord) (*model.Requirement, error) {
	r, err := s.GetRequirement(ctx, tenantID, v.RequirementID)
	if err != nil {
		return nil, err
	}
	if v.CriterionID != nil && *v.CriterionID == "" {
		v.CriterionID = nil
	}
	if v.CriterionID != nil && !slices.ContainsFunc(r.Criteria, func(c model.EvaluationCriterion) bool { return c.ID == *v.CriterionID }) {
		return nil, Invalidf("criterion does not belong to requirement")
	}
	if v.Method == "" && v.CriterionID != nil {
		for _, c := range r.Criteria {
			if c.ID == *v.CriterionID {
				v.Method = c.Method
			}
		}
	}
	if !model.Valid(model.EvaluationMethods, v.Method) {
		return nil, Invalidf("method must be one of %s", strings.Join(model.EvaluationMethods, ", "))
	}
	if !model.Valid(model.RecordResults, v.Result) {
		return nil, Invalidf("result must be one of %s", strings.Join(model.RecordResults, ", "))
	}
	v.PerformedBy = strings.TrimSpace(v.PerformedBy)
	if len([]rune(v.PerformedBy)) > 200 {
		return nil, Invalidf("performed_by may have at most 200 characters")
	}
	if v.PerformedOn == "" {
		v.PerformedOn = s.today()
	}
	if !validDate(v.PerformedOn) || !validDate(v.ValidUntil) {
		return nil, Invalidf("dates must be YYYY-MM-DD")
	}
	if v.ValidUntil != "" && v.ValidUntil < v.PerformedOn {
		return nil, Invalidf("valid_until must not be before performed_on")
	}
	if len(v.Note) > 20000 {
		return nil, Invalidf("note may have at most 20000 characters")
	}
	return r, nil
}

func (s *Store) writeRecordFiles(ctx context.Context, c conn, tenantID, projectID, recordID string, ids []string) error {
	if _, err := c.exec(ctx, `DELETE FROM verification_record_files WHERE record_id = ?`, recordID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		var pid sql.NullString
		if err := c.row(ctx, `SELECT project_id FROM files WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&pid); err != nil {
			return mapErr(err, "file")
		}
		if pid.String != projectID {
			return Invalidf("the file is not in the project's archive")
		}
		if _, err := c.exec(ctx, `INSERT INTO verification_record_files (record_id, file_id) VALUES (?, ?)`, recordID, id); err != nil {
			return mapErr(err, "record file")
		}
	}
	return nil
}

// CreateRecord adds a verification record.
func (s *Store) CreateRecord(ctx context.Context, tenantID string, v *model.VerificationRecord) error {
	r, err := s.validateRecord(ctx, tenantID, v)
	if err != nil {
		return err
	}
	v.ID = newID()
	v.ProjectID = r.ProjectID
	v.CreatedAt = s.now()
	v.UpdatedAt = v.CreatedAt
	v.InvalidatedAt, v.InvalidatedReason, v.Signoff = nil, "", nil
	if v.PerformedBy == "" {
		v.PerformedBy = v.CreatedBy
	}
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `INSERT INTO verification_records (id, tenant_id, project_id, requirement_id, criterion_id, method, result, performed_by, performed_on, note, valid_until, until_change, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			v.ID, tenantID, v.ProjectID, v.RequirementID, nullStr(v.CriterionID), v.Method, v.Result, v.PerformedBy, v.PerformedOn, v.Note,
			v.ValidUntil, v.UntilChange, v.CreatedBy, v.CreatedAt, v.UpdatedAt); err != nil {
			return mapErr(err, "verification record")
		}
		return s.writeRecordFiles(ctx, c, tenantID, v.ProjectID, v.ID, v.FileIDs)
	})
}

// GetRecord returns a verification record with its files.
func (s *Store) GetRecord(ctx context.Context, tenantID, id string) (*model.VerificationRecord, error) {
	v, err := scanRecord(s.c(ctx).row(ctx, `SELECT `+recordCols+recordFrom+` WHERE v.id = ? AND v.tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "verification record")
	}
	list := []model.VerificationRecord{*v}
	if err := s.recordFiles(ctx, list); err != nil {
		return nil, err
	}
	list[0].Validity = verification.Validity(&list[0], s.today())
	return &list[0], nil
}

// ListRecords returns the verification records of a project, or of one
// requirement, newest first.
func (s *Store) ListRecords(ctx context.Context, tenantID, projectID, requirementID string) ([]model.VerificationRecord, error) {
	q := `SELECT ` + recordCols + recordFrom + ` WHERE v.tenant_id = ?`
	args := []any{tenantID}
	if projectID != "" {
		q += ` AND v.project_id = ?`
		args = append(args, projectID)
	}
	if requirementID != "" {
		q += ` AND v.requirement_id = ?`
		args = append(args, requirementID)
	}
	rows, err := s.c(ctx).query(ctx, q+` ORDER BY v.performed_on DESC, v.created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	out := []model.VerificationRecord{}
	for rows.Next() {
		v, err := scanRecord(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.recordFiles(ctx, out); err != nil {
		return nil, err
	}
	today := s.today()
	for i := range out {
		out[i].Validity = verification.Validity(&out[i], today)
	}
	return out, nil
}

func (s *Store) recordFiles(ctx context.Context, list []model.VerificationRecord) error {
	if len(list) == 0 {
		return nil
	}
	idx := map[string]int{}
	ids := make([]string, len(list))
	for i, v := range list {
		idx[v.ID] = i
		ids[i] = v.ID
	}
	for start := 0; start < len(ids); start += 500 {
		chunk := ids[start:min(start+500, len(ids))]
		rows, err := s.c(ctx).query(ctx, `SELECT vf.record_id, f.id, f.name, f.kind FROM verification_record_files vf JOIN files f ON f.id = vf.file_id
			WHERE vf.record_id IN (`+placeholders(len(chunk))+`) ORDER BY f.name`, anySlice(chunk)...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var rid string
			var f model.FileRef
			if err := rows.Scan(&rid, &f.ID, &f.Name, &f.Kind); err != nil {
				rows.Close()
				return err
			}
			v := &list[idx[rid]]
			v.FileIDs = append(v.FileIDs, f.ID)
			v.Files = append(v.Files, f)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

// UpdateRecord changes a record. A change of the evidence (method,
// result, criterion, dates, performer) removes the sign-off; a record
// that was invalidated counts again only when created anew.
func (s *Store) UpdateRecord(ctx context.Context, tenantID string, v *model.VerificationRecord) error {
	old, err := s.GetRecord(ctx, tenantID, v.ID)
	if err != nil {
		return err
	}
	v.RequirementID = old.RequirementID
	if _, err := s.validateRecord(ctx, tenantID, v); err != nil {
		return err
	}
	if v.PerformedBy == "" {
		v.PerformedBy = old.PerformedBy
	}
	signoff := old.Signoff
	if v.Method != old.Method || v.Result != old.Result || !sameStr(v.CriterionID, old.CriterionID) || v.PerformedOn != old.PerformedOn ||
		v.PerformedBy != old.PerformedBy || v.ValidUntil != old.ValidUntil || v.UntilChange != old.UntilChange {
		signoff = nil
	}
	v.UpdatedAt = s.now()
	return s.tx(ctx, func(c conn) error {
		sBy, sName, sComment := "", "", ""
		var sAt *time.Time
		if signoff != nil {
			sBy, sName, sComment, sAt = signoff.By, signoff.Name, signoff.Comment, &signoff.At
		}
		if _, err := c.exec(ctx, `UPDATE verification_records SET criterion_id = ?, method = ?, result = ?, performed_by = ?, performed_on = ?, note = ?, valid_until = ?, until_change = ?,
			signoff_by = ?, signoff_name = ?, signoff_comment = ?, signoff_at = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
			nullStr(v.CriterionID), v.Method, v.Result, v.PerformedBy, v.PerformedOn, v.Note, v.ValidUntil, v.UntilChange,
			sBy, sName, sComment, nullTime(sAt), v.UpdatedAt, v.ID, tenantID); err != nil {
			return err
		}
		if v.FileIDs != nil {
			return s.writeRecordFiles(ctx, c, tenantID, old.ProjectID, v.ID, v.FileIDs)
		}
		return nil
	})
}

func sameStr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// DeleteRecord removes a verification record.
func (s *Store) DeleteRecord(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM verification_records WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "verification record")
	}
	return nil
}

// SignRecord records the sign-off of a second person: not the author
// and not the performer. An empty user removes the sign-off.
func (s *Store) SignRecord(ctx context.Context, tenantID, id, userEmail, userName, comment string) (*model.VerificationRecord, error) {
	v, err := s.GetRecord(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if userEmail == "" {
		_, err = s.c(ctx).exec(ctx, `UPDATE verification_records SET signoff_by = '', signoff_name = '', signoff_comment = '', signoff_at = NULL, updated_at = ? WHERE id = ?`, s.now(), id)
		if err != nil {
			return nil, err
		}
		return s.GetRecord(ctx, tenantID, id)
	}
	if strings.EqualFold(userEmail, v.CreatedBy) || strings.EqualFold(userEmail, v.PerformedBy) || (userName != "" && strings.EqualFold(userName, v.PerformedBy)) {
		return nil, Conflictf("the sign-off must be by a second person, not the author or the performer")
	}
	if len(comment) > 2000 {
		return nil, Invalidf("comment may have at most 2000 characters")
	}
	now := s.now()
	if _, err := s.c(ctx).exec(ctx, `UPDATE verification_records SET signoff_by = ?, signoff_name = ?, signoff_comment = ?, signoff_at = ?, updated_at = ? WHERE id = ?`,
		userEmail, userName, comment, now, now, id); err != nil {
		return nil, err
	}
	return s.GetRecord(ctx, tenantID, id)
}

// InvalidateRecords marks the valid "until change" records of a
// requirement invalidated (reason: what changed). With criterionID only
// the records of that criterion are, and regardless of until_change (the
// criterion was removed). Returns the number invalidated.
func (s *Store) InvalidateRecords(ctx context.Context, tenantID, requirementID, criterionID, reason string) (int, error) {
	return invalidateRecords(ctx, s.c(ctx), s.now(), tenantID, requirementID, criterionID, reason)
}

func invalidateRecords(ctx context.Context, c conn, now time.Time, tenantID, requirementID, criterionID, reason string) (int, error) {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	q := `UPDATE verification_records SET invalidated_at = ?, invalidated_reason = ? WHERE tenant_id = ? AND requirement_id = ? AND invalidated_at IS NULL`
	args := []any{now, reason, tenantID, requirementID}
	if criterionID != "" {
		q += ` AND criterion_id = ?`
		args = append(args, criterionID)
	} else {
		q += ` AND until_change = ?`
		args = append(args, true)
	}
	res, err := c.exec(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ---------------------------------------------------------------- step results

// LastStepResults returns test id → step id → result in the execution
// that gave the test its last result (LastResults); a step executed in
// several iterations is failed when any failed, passed when all passed.
// It also returns when that execution ran.
func (s *Store) LastStepResults(ctx context.Context, tenantID, projectID string) (map[string]map[string]string, map[string]time.Time, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT e.id, e.test_case_id, e.finished_at, e.started_at, r.created_at FROM test_executions e JOIN test_runs r ON r.id = e.run_id
		WHERE r.project_id = ? AND e.tenant_id = ? AND e.status <> 'not_run'`, projectID, tenantID)
	if err != nil {
		return nil, nil, err
	}
	type best struct {
		at time.Time
		id string
	}
	m := map[string]best{}
	for rows.Next() {
		var eid, tid string
		var fin, start sql.NullTime
		var created time.Time
		if err := rows.Scan(&eid, &tid, &fin, &start, &created); err != nil {
			rows.Close()
			return nil, nil, err
		}
		at := created
		if start.Valid {
			at = start.Time
		}
		if fin.Valid {
			at = fin.Time
		}
		if b, ok := m[tid]; !ok || at.After(b.at) {
			m[tid] = best{at, eid}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	when := map[string]time.Time{}
	exec2test := map[string]string{}
	var ids []string
	for tid, b := range m {
		when[tid] = b.at
		exec2test[b.id] = tid
		ids = append(ids, b.id)
	}
	out := map[string]map[string]string{}
	for start := 0; start < len(ids); start += 500 {
		chunk := ids[start:min(start+500, len(ids))]
		rows, err := s.c(ctx).query(ctx, `SELECT execution_id, step_id, status FROM step_results WHERE step_id <> '' AND execution_id IN (`+placeholders(len(chunk))+`)`, anySlice(chunk)...)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			var eid, sid, st string
			if err := rows.Scan(&eid, &sid, &st); err != nil {
				rows.Close()
				return nil, nil, err
			}
			tid := exec2test[eid]
			if out[tid] == nil {
				out[tid] = map[string]string{}
			}
			out[tid][sid] = combineStep(out[tid][sid], st)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, nil, err
		}
	}
	return out, when, nil
}

func combineStep(prev, st string) string {
	switch {
	case prev == "":
		return st
	case prev == model.ResultFailed || st == model.ResultFailed:
		return model.ResultFailed
	case prev == model.ResultBlocked || st == model.ResultBlocked:
		return model.ResultBlocked
	case prev == model.ResultPassed && st == model.ResultPassed:
		return model.ResultPassed
	}
	return model.ResultNotRun
}

// ---------------------------------------------------------------- computation

// VerificationInput loads what package verification needs for a project.
// last may be passed when the caller has it already (nil: loaded).
func (s *Store) VerificationInput(ctx context.Context, tenantID, projectID string, last map[string]string) (*verification.Input, error) {
	reqs, err := s.verificationRequirements(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	links, err := s.ListLinks(ctx, tenantID, projectID, "", "")
	if err != nil {
		return nil, err
	}
	in := &verification.Input{Requirements: reqs, Links: links, Today: s.today()}
	if in.Tests, err = s.verificationTests(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	if last == nil {
		if last, err = s.LastResults(ctx, tenantID, projectID); err != nil {
			return nil, err
		}
	}
	in.Last = last
	if in.Steps, in.LastAt, err = s.LastStepResults(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	if in.Records, err = s.ListRecords(ctx, tenantID, projectID, ""); err != nil {
		return nil, err
	}
	if in.Relations, err = s.ListRelations(ctx, tenantID, projectID, ""); err != nil {
		return nil, err
	}
	st, err := s.VerificationSettings(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	in.Strict = st.StrictCriteria
	return in, nil
}

// ProjectVerification computes the verification of every requirement of
// a project: requirement id → whether it is verified, and why.
func (s *Store) ProjectVerification(ctx context.Context, tenantID, projectID string) (verification.Result, error) {
	in, err := s.VerificationInput(ctx, tenantID, projectID, nil)
	if err != nil {
		return nil, err
	}
	return verification.Compute(*in), nil
}

// RequirementVerification answers "is requirement R verified, and why".
func (s *Store) RequirementVerification(ctx context.Context, tenantID, requirementID string) (*model.RequirementVerification, error) {
	r, err := s.GetRequirement(ctx, tenantID, requirementID)
	if err != nil {
		return nil, err
	}
	res, err := s.ProjectVerification(ctx, tenantID, r.ProjectID)
	if err != nil {
		return nil, err
	}
	v := res[r.ID]
	if v == nil {
		return nil, mapErr(sql.ErrNoRows, "requirement")
	}
	return v, nil
}

// IsVerified reports whether a requirement is verified (rolled up with its
// children) and, when not, why in one English sentence.
func (s *Store) IsVerified(ctx context.Context, tenantID, requirementID string) (bool, string, error) {
	v, err := s.RequirementVerification(ctx, tenantID, requirementID)
	if err != nil {
		return false, "", err
	}
	return v.Verified, verification.Why(v), nil
}

// verificationRequirements loads what the verification needs of the
// requirements of a project: key, title, status and criteria (lighter
// than ListRequirements).
func (s *Store) verificationRequirements(ctx context.Context, tenantID, projectID string) ([]model.Requirement, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, project_id, key, title, status FROM requirements WHERE tenant_id = ? AND project_id = ? ORDER BY position, seq`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := []model.Requirement{}
	for rows.Next() {
		var r model.Requirement
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Key, &r.Title, &r.Status); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	crit, err := s.listCriteria(ctx, s.c(ctx), `WHERE requirement_id IN (SELECT id FROM requirements WHERE project_id = ?)`, projectID)
	if err != nil {
		return nil, err
	}
	byReq := map[string][]model.EvaluationCriterion{}
	for _, c := range crit {
		byReq[c.RequirementID] = append(byReq[c.RequirementID], c)
	}
	for i := range out {
		out[i].Criteria = byReq[out[i].ID]
	}
	return out, nil
}

// verificationTests returns test id → key, title and step positions.
func (s *Store) verificationTests(ctx context.Context, tenantID, projectID string) (map[string]verification.TestInfo, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, key, title FROM test_cases WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := map[string]verification.TestInfo{}
	for rows.Next() {
		var id string
		ti := verification.TestInfo{StepPos: map[string]int{}}
		if err := rows.Scan(&id, &ti.Key, &ti.Title); err != nil {
			rows.Close()
			return nil, err
		}
		out[id] = ti
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = s.c(ctx).query(ctx, `SELECT s.id, s.test_case_id, s.position FROM test_steps s JOIN test_cases t ON t.id = s.test_case_id WHERE t.project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sid, tid string
		var pos int
		if err := rows.Scan(&sid, &tid, &pos); err != nil {
			return nil, err
		}
		if ti, ok := out[tid]; ok {
			ti.StepPos[sid] = pos + 1
		}
	}
	return out, rows.Err()
}
