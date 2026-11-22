package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rom/qtestframe/internal/chat"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// seedChat gives the demo project chat channels with a delivery log: a
// Slack channel for failed runs and new defects, a Teams digest for the
// 1.4 release and a Matrix room that turned itself off after failures.
// They are all off (their addresses do not exist), so the demo never posts
// anywhere; "Send test message" shows the error.
func seedChat(ctx context.Context, st *store.Store, tenantID string, p *model.Project) error {
	rels, err := st.ListReleases(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	relID := ""
	for _, r := range rels {
		if r.Version == "1.4" || relID == "" {
			relID = r.ID
		}
	}
	slack := &model.ChatChannel{ProjectID: p.ID, Name: "QA team (Slack)", Provider: chat.Slack, URL: "https://chat.example.invalid/services/demo-qa",
		Events: []string{"run.completed", "execution.updated", "defect.created", "defect.regression"}, Filters: model.ChatFilters{FailedOnly: true},
		Language: "en", CreatedBy: "anna@example.com"}
	teams := &model.ChatChannel{ProjectID: p.ID, Name: "Release 1.4 (Teams)", Provider: chat.Teams, URL: "https://chat.example.invalid/workflows/demo-release",
		Events: []string{"release.updated", "release.released", "run.completed", "waiver.requested", "waiver.approved"}, Filters: model.ChatFilters{ReleaseID: relID, MinPriority: "high"},
		Language: "sv", DigestMinutes: 60, CreatedBy: "anna@example.com"}
	matrix := &model.ChatChannel{ProjectID: p.ID, Name: "Matrix #qa", Provider: chat.Matrix, Homeserver: "https://matrix.example.invalid", Room: "#qa:example.invalid",
		Token: "demo-token-not-real", Events: []string{"*"}, Language: "en", CreatedBy: "anna@example.com"}
	for _, c := range []*model.ChatChannel{slack, teams, matrix} {
		if err := st.CreateChatChannel(ctx, tenantID, c); err != nil {
			return err
		}
	}
	// a delivery log: posts of the Slack channel and the failures that
	// turned the Matrix room off
	now := st.Now()
	log := func(c *model.ChatChannel, event string, msg chat.Message, o store.ChatOutcome) error {
		b, _ := json.Marshal([]chat.Message{msg})
		id, err := st.AddChatDelivery(ctx, tenantID, c, event, b, 1, false)
		if err != nil {
			return err
		}
		if _, err := st.ClaimChatDelivery(ctx, id, time.Minute); err != nil {
			return err
		}
		_, err = st.FinishChatDelivery(ctx, id, c.ID, o)
		return err
	}
	msg := chat.Message{Event: "run.completed", Kind: "run", Title: "Regression 1.4", Status: "completed", Summary: "9 passed, 2 failed", Project: p.Key + " " + p.Name, Time: now}
	if err := log(slack, chat.EventTest, chat.Message{Event: chat.EventTest, Title: "Qtestframe can post to this channel", Time: now}, store.ChatOutcome{OK: true, StatusCode: 200}); err != nil {
		return err
	}
	if err := log(slack, "run.completed", msg, store.ChatOutcome{OK: true, StatusCode: 200, CountFailure: true}); err != nil {
		return err
	}
	for range model.ChatMaxFailures {
		if err := log(matrix, "defect.created", chat.Message{Event: "defect.created", Time: now}, store.ChatOutcome{StatusCode: 401, Error: "HTTP 401: M_UNKNOWN_TOKEN", CountFailure: true}); err != nil {
			return err
		}
	}
	return st.DisableChatChannel(ctx, matrix.ID, "turned off after 5 failed deliveries in a row; last error: HTTP 401: M_UNKNOWN_TOKEN")
}
