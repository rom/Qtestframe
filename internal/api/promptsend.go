package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rom/qtestframe/internal/ai"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/prompt"
)

// promptSystem is the system prompt when a built prompt is sent to the AI
// assistance: the prompt itself says what to do.
const promptSystem = "You are a careful assistant for software teams. Follow the instructions of the user's prompt exactly and answer in Markdown, in the language of the prompt. Never invent requirements; never include secrets."

// handlePromptSend: POST /projects/{pid}/prompts/send — builds a prompt
// (same body as POST …/prompts, "part" chooses the part of a split prompt)
// and sends it to the optional AI assistance (internal/ai) when it is
// available in the project. Streamed as server-sent events (start, delta
// {text}, result, error) when the client asks for text/event-stream, else
// JSON {"answer", "truncated", "meta", "title", "kind"}. The answer is not
// stored: the client may save it as a project document. Audited as
// ai.request (never the content) and rate limited like the other AI
// features.
func (s *Server) handlePromptSend(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req promptRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	// the availability first: no prompt is built for nothing
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	st, c, err := s.aiState(ctx, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !st.Available {
		writeError(w, http.StatusConflict, "ai_unavailable", "AI assistance is not available here: "+st.Reason)
		return
	}
	pr, res, ok := s.buildPrompt(w, r, p, &req)
	if !ok {
		return
	}
	text := res.Text
	if len(res.Parts) > 1 {
		i := max(req.Part, 1)
		if i > len(res.Parts) {
			s.fail(w, r, badRequest("part must be between 1 and %d", len(res.Parts)))
			return
		}
		text = res.Parts[i-1].Text
	}
	if !s.aiLimiter(c.RateLimit).allow("ai:" + p.User.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", fmt.Sprintf("at most %d AI requests per hour", c.RateLimit))
		return
	}
	call := ai.Request{System: promptSystem, Messages: []ai.Message{{Role: "user", Text: text}}, MaxTokens: c.MaxTokens}
	call.Temperature, _ = config.ParseTemperature(c.Temperature)
	var ev *sseWriter
	var onDelta func(string)
	if wantsStream(r) {
		rc := http.NewResponseController(w)
		// a long answer may outlast the server's write timeout
		_ = rc.SetWriteDeadline(time.Now().Add(2*max(c.Timeout, 30*time.Second) + time.Minute))
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		ev = &sseWriter{w: w, rc: rc}
		ev.send("start", map[string]any{"feature": "prompt", "kind": res.Kind, "provider": st.Provider, "model": st.Model, "tokens": prompt.EstimateTokens(text)})
		onDelta = func(t string) { ev.send("delta", map[string]string{"text": t}) }
	}
	start := time.Now()
	resp, err := s.aiProvider(c).Complete(ctx, call, onDelta)
	meta := aiMeta{DurationMS: time.Since(start).Milliseconds(), Result: ai.Result{Feature: "prompt", Provider: st.Provider, Model: st.Model}}
	if resp != nil {
		meta.Usage = resp.Usage
		if resp.Model != "" {
			meta.Model = resp.Model
		}
		meta.Cost = (float64(resp.Usage.InputTokens)*st.PriceInput + float64(resp.Usage.OutputTokens)*st.PriceOutput) / 1e6
	}
	status := "ok"
	switch {
	case err != nil && ctx.Err() != nil:
		status = "cancelled"
	case err != nil:
		_, code, _ := aiError(err)
		status = code
	}
	// the audit entry: what was asked about, never the content
	s.audit(context.WithoutCancel(ctx), p, "ai.request", "project", pr.ID,
		fmt.Sprintf("feature=prompt kind=%s status=%s provider=%s model=%s items=%d input_tokens=%d output_tokens=%d duration_ms=%d",
			res.Kind, status, st.Provider, meta.Model, res.Items, meta.Usage.InputTokens, meta.Usage.OutputTokens, meta.DurationMS))
	if err != nil {
		if ctx.Err() != nil {
			return // the client went away (cancelled)
		}
		code, name, msg := aiError(err)
		s.Log.Warn("ai: prompt failed", "kind", res.Kind, "err", err)
		if ev != nil {
			ev.send("error", map[string]any{"status": code, "code": name, "error": msg, "meta": meta})
			return
		}
		writeError(w, code, name, msg)
		return
	}
	out := map[string]any{"answer": resp.Text, "truncated": resp.Truncated, "meta": meta, "title": res.Title, "kind": res.Kind}
	if ev != nil {
		ev.send("result", out)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
