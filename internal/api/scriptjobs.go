package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/runner"
	"github.com/rom/qtestframe/internal/store"
)

// Background jobs for automated tests.
//
// Web tests take seconds to minutes, so they never run inside an HTTP
// request: "Run script" of a web test and "Run automated tests" of a run
// enqueue jobs (table script_jobs) and answer at once; the executions are
// "in progress" until their job ends. A small worker pool per server
// process (runner.web.max_parallel) runs the jobs in queue order; a job
// waits for the jobs of its prerequisites (tests it depends on, in the same
// run) and checks the readiness again just before it starts.
//
// Jobs do not survive a restart. A job belongs to the process (instance)
// that enqueued it, which refreshes the jobs' heartbeat; unfinished jobs
// of another instance whose heartbeat is stale are reset — at start-up all
// of them with SQLite (one process), stale ones with PostgreSQL (several
// instances may share the database). Resetting gives the execution its
// earlier result back with a note in its output.
//
// Scripts of the other languages keep running synchronously with "Run
// script" (they are quick and bounded by runner.timeout); "Run automated
// tests" runs them as jobs too.

// jobManager is the worker pool of this process.
type jobManager struct {
	instance string
	wake     chan struct{}

	mu      sync.Mutex
	owners  map[string]*Principal              // job → who enqueued it
	cancels map[string]context.CancelCauseFunc // running job → stop it
	running int
	started bool
	busy    sync.WaitGroup
}

func newJobManager() *jobManager {
	return &jobManager{instance: uuid.NewString(), wake: make(chan struct{}, 1),
		owners: map[string]*Principal{}, cancels: map[string]context.CancelCauseFunc{}}
}

func (m *jobManager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// jobHeartbeat is how often the pool refreshes its jobs' heartbeat and
// looks for stale jobs of other instances (Server.JobHeartbeat in tests).
const jobHeartbeat = 15 * time.Second

func (s *Server) heartbeat() time.Duration {
	if s.JobHeartbeat > 0 {
		return s.JobHeartbeat
	}
	return jobHeartbeat
}

func (s *Server) maxParallel() int {
	if n := s.conf().Runner.Web.MaxParallel; n > 0 {
		return n
	}
	return 2
}

// StartScriptJobs resets jobs left over by a stopped server and runs the
// worker pool until ctx ends.
func (s *Server) StartScriptJobs(ctx context.Context) {
	m := s.jobs
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()
	s.recoverJobs(ctx, true)
	go func() {
		t := time.NewTicker(s.heartbeat())
		defer t.Stop()
		s.dispatch(ctx)
		for {
			select {
			case <-ctx.Done():
				m.mu.Lock()
				for _, c := range m.cancels {
					c(errServerStopped)
				}
				m.mu.Unlock()
				return
			case <-m.wake:
			case <-t.C:
				if err := s.Store.HeartbeatJobs(ctx, m.instance); err != nil && ctx.Err() == nil {
					s.Log.Warn("job heartbeat failed", "err", err)
				}
				s.recoverJobs(ctx, false)
			}
			s.dispatch(ctx)
		}
	}()
}

// WaitScriptJobs waits until no job of this process is queued or running
// (tests, shutdown) or ctx ends.
func (s *Server) WaitScriptJobs(ctx context.Context) error {
	for {
		list, err := s.Store.InstanceJobs(ctx, s.jobs.instance)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			s.jobs.busy.Wait()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// recoverJobs resets unfinished jobs of stopped instances.
func (s *Server) recoverJobs(ctx context.Context, startup bool) {
	before := s.Store.Now().Add(-4 * s.heartbeat())
	if startup && s.Store.DB.Dialect == db.SQLite {
		before = time.Time{}
	}
	list, err := s.Store.UnfinishedJobs(ctx, s.jobs.instance, before)
	if err != nil {
		if ctx.Err() == nil {
			s.Log.Warn("job recovery failed", "err", err)
		}
		return
	}
	for _, j := range list {
		note := "the server stopped before this automated test finished; its earlier result was restored"
		if j.State == model.JobQueued {
			note = "the server stopped while this automated test was queued; its earlier result was restored"
		}
		if ok, err := s.Store.FinishJob(ctx, j.ID, model.JobInterrupted, note); err != nil || !ok {
			continue
		}
		if _, err := s.Store.RestoreExecution(ctx, j.TenantID, j.ExecutionID, j.Previous, note); err != nil {
			s.Log.Warn("job recovery: restore execution", "job", j.ID, "err", err)
		}
		s.Log.Info("reset interrupted job", "job", j.ID, "execution", j.ExecutionID, "test", j.TestKey)
	}
}

// WaitJobsStopped waits up to timeout for running jobs to end (after the
// context of StartScriptJobs ended, at shutdown).
func (s *Server) WaitJobsStopped(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		s.jobs.busy.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// dispatch starts queued jobs while workers are free.
func (s *Server) dispatch(ctx context.Context) {
	m := s.jobs
	for ctx.Err() == nil {
		m.mu.Lock()
		free := m.running < s.maxParallel()
		m.mu.Unlock()
		if !free {
			return
		}
		list, err := s.Store.InstanceJobs(ctx, m.instance)
		if err != nil {
			if ctx.Err() == nil {
				s.Log.Warn("job dispatch failed", "err", err)
			}
			return
		}
		var waiting []string
		for _, j := range list {
			waiting = append(waiting, j.After...)
		}
		states, err := s.Store.JobStates(ctx, waiting)
		if err != nil {
			return
		}
		var next *model.ScriptJob
		for i := range list {
			j := &list[i]
			if j.State != model.JobQueued {
				continue
			}
			ready := true
			for _, a := range j.After {
				if model.JobActive(states[a]) {
					ready = false
				}
			}
			if ready {
				next = j
				break
			}
		}
		if next == nil {
			return
		}
		ok, err := s.Store.ClaimJob(ctx, next.ID)
		if err != nil {
			return
		}
		if !ok {
			continue
		}
		jctx, cancel := context.WithCancelCause(ctx)
		m.mu.Lock()
		m.running++
		m.cancels[next.ID] = cancel
		owner := m.owners[next.ID]
		m.mu.Unlock()
		m.busy.Add(1)
		go func(j model.ScriptJob) {
			defer func() {
				cancel(nil)
				m.mu.Lock()
				m.running--
				delete(m.cancels, j.ID)
				delete(m.owners, j.ID)
				m.mu.Unlock()
				m.busy.Done()
				m.signal()
			}()
			s.runJob(jctx, &j, owner)
		}(*next)
	}
}

// cancelRunning stops a running job of this process; false if it is not
// running here.
func (m *jobManager) cancelRunning(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.cancels[id]
	if ok {
		c(errJobCancelled)
	}
	return ok
}

var (
	errJobCancelled  = errors.New("cancelled while running")
	errServerStopped = errors.New("the server stopped while the test was running; its earlier result was restored")
)

// stopped ends a job whose context ended (cancelled, or the server stops).
func (s *Server) stopped(ctx context.Context, tenantID string, j *model.ScriptJob) {
	if errors.Is(context.Cause(ctx), errJobCancelled) {
		s.endJob(ctx, tenantID, j, model.JobCancelled, errJobCancelled.Error())
		return
	}
	s.endJob(ctx, tenantID, j, model.JobInterrupted, errServerStopped.Error())
}

// ---------------------------------------------------------------- a job

// jobRequest is a stand-in request for helpers that take one (they only
// use its context).
func jobRequest(ctx context.Context) *http.Request {
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/script-job", nil)
	return r
}

// endJob finishes a job without a result: the execution gets its earlier
// result back.
func (s *Server) endJob(ctx context.Context, tenantID string, j *model.ScriptJob, state, message string) {
	// An error after the job's context ended (a store lookup or the runner
	// failing with "context canceled") is the cancellation itself: record
	// it as cancelled or interrupted, not as an error.
	if state == model.JobError && ctx.Err() != nil {
		state, message = model.JobInterrupted, errServerStopped.Error()
		if errors.Is(context.Cause(ctx), errJobCancelled) {
			state, message = model.JobCancelled, errJobCancelled.Error()
		}
	}
	// a stopped job is recorded even though its context has ended
	ctx = context.WithoutCancel(ctx)
	if ok, err := s.Store.FinishJob(ctx, j.ID, state, message); err != nil || !ok {
		return
	}
	if _, err := s.Store.RestoreExecution(ctx, tenantID, j.ExecutionID, j.Previous, message); err != nil {
		s.Log.Warn("job: restore execution", "job", j.ID, "err", err)
	}
}

// runJob runs one claimed job.
func (s *Server) runJob(ctx context.Context, j *model.ScriptJob, p *Principal) {
	if p == nil {
		s.endJob(ctx, j.TenantID, j, model.JobError, "the job has no owner (it was enqueued by another server process)")
		return
	}
	tenant := p.TenantID()
	e, err := s.Store.GetExecution(ctx, tenant, j.ExecutionID)
	if err != nil {
		s.endJob(ctx, tenant, j, model.JobError, err.Error())
		return
	}
	if e.Status != model.ResultInProgress {
		// someone recorded a result while the job was queued
		_, _ = s.Store.FinishJob(ctx, j.ID, model.JobCancelled, "the result was set by hand while the test was queued")
		return
	}
	rd, err := s.executionReadiness(ctx, tenant, e)
	if err != nil {
		s.endJob(ctx, tenant, j, model.JobError, err.Error())
		return
	}
	if !rd.Allowed {
		var why []string
		for _, i := range rd.Issues {
			if i.Blocking {
				why = append(why, issueText(i))
			}
		}
		s.endJob(ctx, tenant, j, model.JobSkipped, "not executed: "+strings.Join(why, "; "))
		return
	}
	t, err := s.Store.GetTest(ctx, tenant, e.TestCaseID)
	if err != nil {
		s.endJob(ctx, tenant, j, model.JobError, err.Error())
		return
	}
	if t.ExecutionType != model.ExecAutomated || strings.TrimSpace(t.Script) == "" {
		s.endJob(ctx, tenant, j, model.JobSkipped, "the test has no automated script")
		return
	}
	run, err := s.Store.GetRun(ctx, tenant, e.RunID)
	if err != nil {
		s.endJob(ctx, tenant, j, model.JobError, err.Error())
		return
	}
	set, err := s.Store.WebTestSettings(ctx, tenant, run.ProjectID)
	if err != nil {
		s.endJob(ctx, tenant, j, model.JobError, err.Error())
		return
	}
	lang := t.ScriptLanguage
	if lang == "" {
		lang = "sh"
	}
	var (
		status   string
		output   string
		message  string
		duration time.Duration
		evidence []runner.Evidence
	)
	target := s.executionTarget(ctx, tenant, run, e, set)
	env, secrets := scriptEnv(t, run, set, target.BaseURL)
	if lang == runner.LangPlaywright {
		if !s.Runner.WebEnabled() {
			s.endJob(ctx, tenant, j, model.JobError, "web tests are not enabled on this server (runner.web.enabled)")
			return
		}
		res, err := s.Runner.RunWeb(ctx, runner.WebJob{Title: t.Title, Script: t.Script, BaseURL: target.BaseURL,
			Browser: target.Browser, Channel: target.Channel, Env: env, Secrets: secrets})
		if err != nil {
			s.endJob(ctx, tenant, j, model.JobError, runner.Mask(err.Error(), secrets))
			return
		}
		if ctx.Err() != nil {
			s.stopped(ctx, tenant, j)
			return
		}
		output, message, duration, evidence = res.Output, res.Message, res.Duration, res.Evidence
		if len(target.Notes) > 0 {
			output += "[qtestframe] " + strings.Join(target.Notes, "; ") + "\n"
		}
		switch res.Status {
		case "passed":
			status = model.ResultPassed
		case "skipped":
			status = model.ResultSkipped
		default:
			status = model.ResultFailed
		}
	} else {
		if !s.Runner.Enabled() {
			s.endJob(ctx, tenant, j, model.JobError, runner.ErrDisabled.Error())
			return
		}
		res, err := s.Runner.Run(ctx, lang, t.Script, env)
		if err != nil {
			s.endJob(ctx, tenant, j, model.JobError, runner.Mask(err.Error(), secrets))
			return
		}
		if ctx.Err() != nil {
			s.stopped(ctx, tenant, j)
			return
		}
		output, duration = runner.Mask(res.Output, secrets), res.Duration
		status = model.ResultFailed
		if res.Passed {
			status = model.ResultPassed
		} else {
			message = fmt.Sprintf("exit status %d", res.ExitCode)
			if res.TimedOut {
				message = "timed out"
			}
		}
	}
	ctx = context.WithoutCancel(ctx)
	r := jobRequest(ctx)
	if len(evidence) > 0 {
		s.attachEvidence(ctx, p, run.ProjectID, e.ID, evidence)
	}
	dur := duration.Milliseconds()
	runBefore := run.Status
	after, err := s.Store.UpdateExecution(ctx, tenant, e.ID, p.User.Email, store.ExecutionUpdate{Status: &status, Output: &output, DurationMS: &dur})
	if err != nil {
		s.endJob(ctx, tenant, j, model.JobError, err.Error())
		return
	}
	state := model.JobFailed
	if status == model.ResultPassed {
		state = model.JobPassed
	} else if status == model.ResultSkipped {
		state = model.JobSkipped
	}
	_, _ = s.Store.FinishJob(ctx, j.ID, state, message)
	s.audit(ctx, p, "execution.script", "execution", e.ID, e.TestKey+" "+status+" (background job)")
	s.afterExecutionChange(r, p, e, after, runBefore)
	if reqs := s.requirementsOfTests(ctx, p, run.ProjectID, []string{e.TestCaseID}); len(reqs) > 0 {
		s.syncEvidence(ctx, p, run.ProjectID, reqs)
	}
}

// webEvidenceTag marks attachments produced by web tests; a new run of
// the test replaces them.
const webEvidenceTag = "web-test"

// attachEvidence stores screenshots, traces and videos as attachments of
// the execution, replacing those of its previous web test run.
func (s *Server) attachEvidence(ctx context.Context, p *Principal, projectID, executionID string, ev []runner.Evidence) {
	tenant := p.TenantID()
	if old, err := s.Store.ListAttachments(ctx, tenant, "execution", executionID); err == nil {
		for _, f := range old {
			for _, tag := range f.Tags {
				if tag == webEvidenceTag {
					_ = s.Store.Detach(ctx, tenant, f.ID, "execution", executionID)
					break
				}
			}
		}
	}
	for _, e := range ev {
		f := &model.FileInfo{ProjectID: &projectID, Kind: model.FileAttachment, Name: e.Name, ContentType: e.ContentType,
			Description: evidenceDescription(e.Kind), Tags: []string{webEvidenceTag, e.Kind}, UploadedBy: p.User.Email}
		if err := s.Store.CreateFile(ctx, tenant, f, e.Data); err != nil {
			s.Log.Warn("web test evidence", "err", err)
			continue
		}
		if err := s.Store.Attach(ctx, f.ID, "execution", executionID); err != nil {
			_ = s.Store.DeleteFile(ctx, tenant, f.ID)
		}
	}
}

func evidenceDescription(kind string) string {
	switch kind {
	case "trace":
		return "Playwright trace: open with npx playwright show-trace or at trace.playwright.dev"
	case "video":
		return "Video of the web test"
	}
	return "Screenshot of the web test"
}

// ---------------------------------------------------------------- enqueue

// skippedExecution is an execution that "Run automated tests" left out.
type skippedExecution struct {
	ExecutionID       string `json:"execution_id"`
	TestKey           string `json:"test_key"`
	ConfigurationName string `json:"configuration_name,omitempty"`
	Reason            string `json:"reason"`
	// Code: not_automated, no_script, disabled, active, not_ready, cycle.
	Code string `json:"code"`
}

type enqueueResult struct {
	Queued  []model.ScriptJob  `json:"queued"`
	Skipped []skippedExecution `json:"skipped"`
}

// canRun reports why an automated test cannot run on this server ("" =
// it can).
func (s *Server) cannotRun(t *model.TestCase) (code, reason string) {
	switch {
	case t.ExecutionType != model.ExecAutomated:
		return "not_automated", "manual test"
	case strings.TrimSpace(t.Script) == "":
		return "no_script", "the test has no script"
	case t.ScriptLanguage == runner.LangPlaywright && !s.Runner.WebEnabled():
		return "disabled", "web tests are not enabled on this server"
	case t.ScriptLanguage != runner.LangPlaywright && !s.Runner.Enabled():
		return "disabled", "script execution is disabled on this server"
	}
	return "", ""
}

// enqueue queues jobs for executions of a run (in run order), honouring
// readiness: a prerequisite in the same run (and configuration) that is
// queued too runs first; other blocking issues leave the execution out.
// Must run inside the request transaction (s.atomic).
func (s *Server) enqueue(ctx context.Context, p *Principal, run *model.TestRun, execs []model.Execution) (*enqueueResult, error) {
	tenant := p.TenantID()
	rc, err := s.loadReadiness(ctx, tenant, run.ProjectID, run.ID)
	if err != nil {
		return nil, err
	}
	all, err := s.Store.ListExecutions(ctx, tenant, run.ID)
	if err != nil {
		return nil, err
	}
	byKey := map[string]string{} // runKey → execution id
	for _, e := range all {
		byKey[runKey(e.TestCaseID, e.ConfigurationID)] = e.ID
	}
	active := map[string]string{} // execution → active job
	jobs, err := s.Store.ListRunJobs(ctx, tenant, run.ID, 1000)
	if err != nil {
		return nil, err
	}
	for _, j := range jobs {
		if model.JobActive(j.State) {
			active[j.ExecutionID] = j.ID
		}
	}
	res := &enqueueResult{Queued: []model.ScriptJob{}, Skipped: []skippedExecution{}}
	skip := func(e *model.Execution, code, reason string) {
		res.Skipped = append(res.Skipped, skippedExecution{ExecutionID: e.ID, TestKey: e.TestKey, ConfigurationName: e.ConfigurationName, Reason: reason, Code: code})
	}
	var cands []*model.Execution
	for i := range execs {
		e := &execs[i]
		t := rc.tests[e.TestCaseID]
		if t == nil {
			skip(e, "not_automated", "unknown test")
			continue
		}
		if code, reason := s.cannotRun(t); code != "" {
			skip(e, code, reason)
			continue
		}
		if _, ok := active[e.ID]; ok {
			skip(e, "active", "already queued or running")
			continue
		}
		cands = append(cands, e)
	}
	// prerequisites first (Kahn's algorithm over the candidates, stable in run order)
	inBatch := map[string]*model.Execution{}
	for _, e := range cands {
		inBatch[e.ID] = e
	}
	prereqs := func(e *model.Execution) []string {
		var out []string
		for _, d := range rc.deps[e.TestCaseID] {
			if id, ok := byKey[runKey(d.DependsOnID, e.ConfigurationID)]; ok && inBatch[id] != nil && id != e.ID {
				out = append(out, id)
			}
		}
		return out
	}
	var order []*model.Execution
	placed := map[string]bool{}
	for len(order) < len(cands) {
		progress := false
		for _, e := range cands {
			if placed[e.ID] {
				continue
			}
			ok := true
			for _, pid := range prereqs(e) {
				if !placed[pid] {
					ok = false
				}
			}
			if ok {
				placed[e.ID] = true
				order = append(order, e)
				progress = true
			}
		}
		if !progress {
			break
		}
	}
	for _, e := range cands {
		if !placed[e.ID] {
			skip(e, "cycle", "its prerequisites depend on it (circular dependency)")
		}
	}
	batch := uuid.NewString()
	jobOf := map[string]string{} // execution → job of this batch
	var created []*model.ScriptJob
	for _, e := range order {
		rd := rc.check(e.TestCaseID, e.ConfigurationID)
		var after, why []string
		for _, i := range rd.Issues {
			if !i.Blocking {
				continue
			}
			if i.Kind == IssueDependency && i.InRun {
				pre := byKey[runKey(i.ID, e.ConfigurationID)]
				if jid, ok := jobOf[pre]; ok {
					after = append(after, jid)
					continue
				}
				if jid, ok := active[pre]; ok {
					after = append(after, jid)
					continue
				}
			}
			why = append(why, issueText(i))
		}
		if len(why) > 0 {
			skip(e, "not_ready", strings.Join(why, "; "))
			continue
		}
		j := &model.ScriptJob{ID: uuid.NewString(), ProjectID: run.ProjectID, RunID: run.ID, ExecutionID: e.ID, BatchID: batch, Seq: len(created),
			After: after, RequestedBy: p.User.Email, UserID: p.User.ID, Instance: s.jobs.instance, Previous: store.StateOf(e),
			TestKey: e.TestKey, TestTitle: e.TestTitle, ConfigurationName: e.ConfigurationName}
		jobOf[e.ID] = j.ID
		created = append(created, j)
	}
	if len(created) == 0 {
		return res, nil
	}
	inProgress := model.ResultInProgress
	for _, j := range created {
		if _, err := s.Store.UpdateExecution(ctx, tenant, j.ExecutionID, p.User.Email, store.ExecutionUpdate{Status: &inProgress}); err != nil {
			return nil, err
		}
	}
	if err := s.Store.CreateScriptJobs(ctx, tenant, created); err != nil {
		return nil, err
	}
	// the owners are known before the commit, so the pool can never claim
	// a job without one (a rolled back request leaves unused entries)
	m := s.jobs
	m.mu.Lock()
	for _, j := range created {
		m.owners[j.ID] = p
		res.Queued = append(res.Queued, *j)
	}
	m.mu.Unlock()
	store.AfterCommit(ctx, m.signal)
	return res, nil
}

// handleRunAutomated: POST /runs/{id}/run-automated {"execution_ids": [],
// "include_passed": false} queues the automated executions of a run (all,
// or the given ones) as background jobs.
func (s *Server) handleRunAutomated(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		ExecutionIDs  []string `json:"execution_ids"`
		IncludePassed bool     `json:"include_passed"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	run, err := s.Store.GetRun(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !s.Runner.Enabled() {
		s.fail(w, r, runner.ErrDisabled)
		return
	}
	all, err := s.Store.ListExecutions(r.Context(), p.TenantID(), run.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sort.SliceStable(all, func(i, k int) bool { return all[i].Position < all[k].Position })
	var execs []model.Execution
	if len(req.ExecutionIDs) > 0 {
		ids, err := bulkIDs(req.ExecutionIDs)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		want := map[string]bool{}
		for _, id := range ids {
			want[id] = true
		}
		for _, e := range all {
			if want[e.ID] {
				execs = append(execs, e)
				delete(want, e.ID)
			}
		}
		if len(want) > 0 {
			s.fail(w, r, badRequest("%d execution(s) are not part of this run", len(want)))
			return
		}
	} else {
		for _, e := range all {
			t, err := s.Store.GetTest(r.Context(), p.TenantID(), e.TestCaseID)
			if err != nil || t.ExecutionType != model.ExecAutomated {
				continue
			}
			if e.Status == model.ResultPassed && !req.IncludePassed {
				continue
			}
			execs = append(execs, e)
		}
	}
	res, err := s.enqueue(r.Context(), p, run, execs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "run.automated", "run", run.ID, fmt.Sprintf("%d queued, %d skipped", len(res.Queued), len(res.Skipped)))
	writeJSON(w, http.StatusAccepted, res)
}

// jobCounts summarises the current jobs of a run.
type jobCounts struct {
	Queued    int `json:"queued"`
	Running   int `json:"running"`
	Done      int `json:"done"`
	Passed    int `json:"passed"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
	Cancelled int `json:"cancelled"`
	Error     int `json:"error"`
	Total     int `json:"total"`
}

// currentJobs are the jobs of the batches that still have work, or of the
// latest batch when all are done.
func currentJobs(list []model.ScriptJob) ([]model.ScriptJob, jobCounts) {
	batches := map[string]bool{}
	for _, j := range list {
		if model.JobActive(j.State) {
			batches[j.BatchID] = true
		}
	}
	if len(batches) == 0 && len(list) > 0 {
		batches[list[0].BatchID] = true
	}
	var out []model.ScriptJob
	var c jobCounts
	for _, j := range list {
		if !batches[j.BatchID] {
			continue
		}
		out = append(out, j)
		c.Total++
		switch j.State {
		case model.JobQueued:
			c.Queued++
		case model.JobRunning:
			c.Running++
		default:
			c.Done++
			switch j.State {
			case model.JobPassed:
				c.Passed++
			case model.JobFailed:
				c.Failed++
			case model.JobSkipped:
				c.Skipped++
			case model.JobCancelled:
				c.Cancelled++
			default:
				c.Error++
			}
		}
	}
	sort.SliceStable(out, func(i, k int) bool {
		if out[i].CreatedAt.Equal(out[k].CreatedAt) {
			return out[i].Seq < out[k].Seq
		}
		return out[i].CreatedAt.Before(out[k].CreatedAt)
	})
	return out, c
}

// handleListRunJobs: GET /runs/{id}/jobs — the current jobs of a run and
// their counts (the run detail polls it while jobs are active).
func (s *Server) handleListRunJobs(w http.ResponseWriter, r *http.Request, p *Principal) {
	run, err := s.Store.GetRun(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListRunJobs(r.Context(), p.TenantID(), run.ID, 1000)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cur, counts := currentJobs(list)
	if cur == nil {
		cur = []model.ScriptJob{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": cur, "counts": counts, "active": counts.Queued+counts.Running > 0})
}

// cancelJobs cancels queued jobs (their executions get their earlier
// result back) and, with running, stops running ones of this process.
func (s *Server) cancelJobs(ctx context.Context, p *Principal, list []model.ScriptJob, running bool) (cancelled, stopping int, err error) {
	note := "cancelled by " + p.User.Email
	var stop, dropped []string
	for _, j := range list {
		switch {
		case j.State == model.JobQueued:
			ok, err := s.Store.FinishJob(ctx, j.ID, model.JobCancelled, note)
			if err != nil {
				return 0, 0, err
			}
			if !ok {
				continue
			}
			if _, err := s.Store.RestoreExecution(ctx, p.TenantID(), j.ExecutionID, j.Previous, note); err != nil {
				return 0, 0, err
			}
			cancelled++
			dropped = append(dropped, j.ID)
		case j.State == model.JobRunning && running:
			stop = append(stop, j.ID)
			stopping++
		}
	}
	store.AfterCommit(ctx, func() {
		for _, id := range stop {
			s.jobs.cancelRunning(id)
		}
		s.jobs.mu.Lock()
		for _, id := range dropped {
			delete(s.jobs.owners, id)
		}
		s.jobs.mu.Unlock()
		s.jobs.signal()
	})
	return cancelled, stopping, nil
}

// handleCancelRunJobs: POST /runs/{id}/jobs/cancel {"job_ids": [],
// "running": false} cancels queued jobs of a run (all, or the given ones);
// with running, running jobs are stopped too.
func (s *Server) handleCancelRunJobs(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		JobIDs  []string `json:"job_ids"`
		Running bool     `json:"running"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	run, err := s.Store.GetRun(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListRunJobs(r.Context(), p.TenantID(), run.ID, 1000)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.JobIDs) > 0 {
		want := map[string]bool{}
		for _, id := range req.JobIDs {
			want[id] = true
		}
		var sel []model.ScriptJob
		for _, j := range list {
			if want[j.ID] {
				sel = append(sel, j)
			}
		}
		list = sel
	}
	n, stopping, err := s.cancelJobs(r.Context(), p, list, req.Running)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "run.jobs_cancel", "run", run.ID, fmt.Sprintf("%d cancelled, %d stopping", n, stopping))
	writeJSON(w, http.StatusOK, map[string]int{"cancelled": n, "stopping": stopping})
}

// handleCancelExecutionJob: POST /executions/{id}/run-script/cancel
// cancels the queued job of an execution or stops its running one.
func (s *Server) handleCancelExecutionJob(w http.ResponseWriter, r *http.Request, p *Principal) {
	e, err := s.Store.GetExecution(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	j, err := s.Store.LatestExecutionJob(r.Context(), p.TenantID(), e.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if j == nil || !model.JobActive(j.State) {
		s.fail(w, r, fmt.Errorf("%w: the execution has no queued or running job", store.ErrConflict))
		return
	}
	n, stopping, err := s.cancelJobs(r.Context(), p, []model.ScriptJob{*j}, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "execution.script_cancel", "execution", e.ID, e.TestKey)
	writeJSON(w, http.StatusOK, map[string]int{"cancelled": n, "stopping": stopping})
}
