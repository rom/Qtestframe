package chat

import (
	"strings"
)

// Event groups and labels.
//
// The events a chat channel can subscribe to are not listed here: they are
// the events of the central registry (model.WebhookEvents), which the API
// passes through Group and EventLabel. An event added to the registry
// appears in the channel settings at once: in the group named by the part
// before its first dot (see groupOf), else under "other", with the label
// of the table below or, without one, a label made from its name
// ("discussion.created" → "Discussion created").

// Groups are the event groups in the order of the channel settings.
var Groups = []string{"runs", "defects", "requirements", "reviews", "releases", "waivers", "risks", "other"}

// groupOf maps the first part of an event name to its group.
var groupOf = map[string]string{
	"run": "runs", "execution": "runs", "cycle": "runs", "session": "runs", "exploratory": "runs",
	"environment": "runs", "testdata": "runs",
	"defect":      "defects",
	"requirement": "requirements", "test": "requirements", "comment": "requirements", "baseline": "requirements",
	"discussion": "requirements", "mention": "requirements",
	"review":  "reviews",
	"release": "releases",
	"waiver":  "waivers",
	"risk":    "risks",
}

// Group returns the group of an event ("other" when its prefix is unknown).
func Group(event string) string {
	prefix, _, _ := strings.Cut(event, ".")
	if g, ok := groupOf[prefix]; ok {
		return g
	}
	return "other"
}

var groupLabels = map[string][2]string{
	"runs":         {"Test runs", "Testkörningar"},
	"defects":      {"Defects", "Defekter"},
	"requirements": {"Requirements and tests", "Krav och tester"},
	"reviews":      {"Reviews", "Granskningar"},
	"releases":     {"Releases", "Releaser"},
	"waivers":      {"Waivers", "Undantag"},
	"risks":        {"Risks", "Risker"},
	"other":        {"Other", "Övrigt"},
}

// GroupLabel names a group in a language ("en" or "sv").
func GroupLabel(lang, group string) string {
	if l, ok := groupLabels[group]; ok {
		return pick(lang, l)
	}
	return humanize(group)
}

var eventLabels = map[string][2]string{
	"requirement.created":           {"Requirement created", "Krav skapat"},
	"requirement.updated":           {"Requirement updated", "Krav ändrat"},
	"test.created":                  {"Test created", "Test skapat"},
	"test.updated":                  {"Test updated", "Test ändrat"},
	"run.created":                   {"Test run created", "Testkörning skapad"},
	"run.updated":                   {"Test run updated", "Testkörning ändrad"},
	"run.completed":                 {"Test run completed", "Testkörning avslutad"},
	"execution.updated":             {"Test result recorded", "Testresultat registrerat"},
	"defect.created":                {"Defect reported", "Defekt rapporterad"},
	"defect.updated":                {"Defect updated", "Defekt ändrad"},
	"defect.status":                 {"Defect status changed", "Defektens status ändrad"},
	"defect.assigned":               {"Defect assigned", "Defekt tilldelad"},
	"defect.regression":             {"Regression detected", "Regression upptäckt"},
	"risk.created":                  {"Risk identified", "Risk identifierad"},
	"risk.updated":                  {"Risk updated", "Risk ändrad"},
	"release.created":               {"Release created", "Release skapad"},
	"release.updated":               {"Release updated", "Release ändrad"},
	"release.released":              {"Release released", "Release släppt"},
	"comment.created":               {"New comment", "Ny kommentar"},
	"review.requested":              {"Review requested", "Granskning begärd"},
	"review.completed":              {"Review completed", "Granskning klar"},
	"baseline.signed":               {"Baseline signed", "Baslinje signerad"},
	"waiver.requested":              {"Waiver requested", "Undantag begärt"},
	"waiver.approved":               {"Waiver approved", "Undantag godkänt"},
	"waiver.rejected":               {"Waiver rejected", "Undantag avslaget"},
	"waiver.revoked":                {"Waiver revoked", "Undantag återkallat"},
	"environment.status_changed":    {"Environment status changed", "Miljöns status ändrad"},
	"environment.booking_requested": {"Environment booking requested", "Miljöbokning begärd"},
	"environment.booking_approved":  {"Environment booking approved", "Miljöbokning godkänd"},
	"environment.booking_rejected":  {"Environment booking rejected", "Miljöbokning avslagen"},
	"testdata.refresh_requested":    {"Test data refresh requested", "Uppdatering av testdata begärd"},
	"testdata.refresh_resolved":     {"Test data refresh request closed", "Begäran om uppdatering av testdata stängd"},
	EventTest:                       {"Test message", "Testmeddelande"},
}

// EventTest is the event of a "send test message".
const EventTest = "chat.test"

// EventLabel names an event in a language.
func EventLabel(lang, event string) string {
	if l, ok := eventLabels[event]; ok {
		return pick(lang, l)
	}
	return humanize(event)
}

var statusLabels = map[string]string{
	"passed": "godkänt", "failed": "underkänt", "blocked": "blockerat", "skipped": "överhoppat", "not_run": "ej kört",
	"in_progress": "pågår", "open": "öppen", "resolved": "löst", "closed": "stängd", "rejected": "avvisad",
	"approved": "godkänd", "changes_requested": "ändringar begärda", "completed": "avslutad", "planned": "planerad",
	"released": "släppt", "requested": "begärd", "revoked": "återkallad", "expired": "utgången", "draft": "utkast",
	"pending": "väntar", "cancelled": "avbruten", "identified": "identifierad", "mitigating": "åtgärdas",
	"mitigated": "åtgärdad", "accepted": "accepterad", "critical": "kritisk", "high": "hög", "medium": "medel",
	"low": "låg", "major": "allvarlig", "minor": "mindre", "trivial": "trivial", "review": "granskning",
	"obsolete": "utgått", "ready": "klar", "active": "aktiv", "deprecated": "utfasad",
}

// ValueLabel shows a status, priority or level value ("in_progress" →
// "in progress", in Swedish "pågår").
func ValueLabel(lang, v string) string {
	if lang == "sv" {
		if l, ok := statusLabels[strings.ToLower(v)]; ok {
			return l
		}
	}
	return strings.ReplaceAll(v, "_", " ")
}

// humanize makes a label of a name: "discussion.created" → "Discussion created".
func humanize(name string) string {
	s := strings.Join(strings.FieldsFunc(name, func(r rune) bool { return r == '.' || r == '_' || r == '-' }), " ")
	if s == "" {
		return name
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func pick(lang string, l [2]string) string {
	if lang == "sv" {
		return l[1]
	}
	return l[0]
}

// texts of the messages, en and sv
var texts = map[string][2]string{
	"status":   {"Status", "Status"},
	"priority": {"Priority", "Prioritet"},
	"by":       {"By", "Av"},
	"project":  {"Project", "Projekt"},
	"open":     {"Open in Qtestframe", "Öppna i Qtestframe"},
	"openKey":  {"Open %s in Qtestframe", "Öppna %s i Qtestframe"},
	"digest":   {"%d events in %s", "%d händelser i %s"},
	"digest1":  {"%d events", "%d händelser"},
	"more":     {"… and %d more", "… och %d till"},
}

func text(lang, key string) string { return pick(lang, texts[key]) }
