package model

import "time"

// ChatChannel posts the events of a project to a chat tool (Slack,
// Microsoft Teams, Mattermost, Rocket.Chat, Google Chat, Matrix, Discord).
type ChatChannel struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	// Provider: one of chat.Providers.
	Provider string `json:"provider"`
	// URL is the incoming webhook URL and Token the Matrix access token;
	// both are write-only (the API reports HasSecret and Target only).
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	HasSecret bool   `json:"has_secret"`
	// Target shows where the channel posts without the secret: the host
	// of the webhook URL, or the Matrix homeserver and room.
	Target string `json:"target"`
	// Matrix: homeserver base URL and room ID (!id:server) or alias.
	Homeserver string `json:"homeserver"`
	Room       string `json:"room"`
	// Events of the central registry (model.WebhookEvents; "*" = all).
	Events  []string    `json:"events"`
	Filters ChatFilters `json:"filters"`
	// Language of the messages: en | sv.
	Language string `json:"language"`
	// DigestMinutes > 0 collects the events and posts one message every
	// so many minutes; 0 posts each event at once.
	DigestMinutes int  `json:"digest_minutes"`
	Enabled       bool `json:"enabled"`
	// RestrictedContent lets the channel show titles and summaries of a
	// restricted project (only organisation administrators may set it);
	// without it such a project's messages name only the event, the key
	// and the link.
	RestrictedContent bool `json:"restricted_content"`
	// Failures counts the deliveries that failed in a row; the channel is
	// turned off (DisabledReason) after ChatMaxFailures.
	Failures       int        `json:"failures"`
	DisabledReason string     `json:"disabled_reason"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastDeliveryAt *time.Time `json:"last_delivery_at"`
	LastStatus     int        `json:"last_status"`
	LastError      string     `json:"last_error"`
	// Derived: events waiting for the next digest and when it is due.
	Pending      int        `json:"pending"`
	NextDigestAt *time.Time `json:"next_digest_at"`
}

// ChatMaxFailures: failed deliveries in a row that turn a channel off.
const ChatMaxFailures = 5

// ChatMaxDigestMinutes is the longest digest interval (one day).
const ChatMaxDigestMinutes = 1440

// ChatFilters narrow the events a channel posts.
type ChatFilters struct {
	// FailedOnly: of the run events, only runs with failed results and
	// failed test results.
	FailedOnly bool `json:"failed_only"`
	// MinPriority (critical | high | medium | low; "" any): events of
	// items with a lower priority (defects: severity, risks: level) are
	// left out; events without a priority pass.
	MinPriority string `json:"min_priority"`
	// ReleaseID: only events of this release (the release, its runs and
	// waivers); events without a release are left out.
	ReleaseID string `json:"release_id"`
}

// ChatDelivery is one post of a channel with its attempts.
type ChatDelivery struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	// Event: the event, "digest" or "chat.test".
	Event string `json:"event"`
	// Count: the events in the post.
	Count int `json:"count"`
	// Status: queued | sending | retry | sent | failed (batched rows wait
	// for the digest and are not listed).
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	StatusCode    int        `json:"status_code"`
	Error         string     `json:"error"`
	CreatedAt     time.Time  `json:"created_at"`
	NextAttemptAt *time.Time `json:"next_attempt_at"`
	SentAt        *time.Time `json:"sent_at"`
}

// Chat delivery statuses.
const (
	ChatBatched = "batched"
	ChatQueued  = "queued"
	ChatSending = "sending"
	ChatRetry   = "retry"
	ChatSent    = "sent"
	ChatFailed  = "failed"
)
