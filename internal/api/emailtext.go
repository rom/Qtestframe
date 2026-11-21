package api

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

// Texts of the e-mails the server sends, in the recipient's language
// (Preferences.Language; English when unset).
var mailTexts = map[string]map[string]string{
	"en": {
		"notification.subject": "%s",
		"notification.intro":   "%s:",
		"notification.open":    "Open in Qtestframe",
		"digest.subject":       "Qtestframe: %d new notifications",
		"digest.subject1":      "Qtestframe: 1 new notification",
		"digest.intro":         "These notifications arrived since your last digest and are still unread.",
		"digest.more":          "There are more unread notifications; they follow in the next digest.",
		"digest.all":           "All notifications",
		"footer.prefs":         "You get these e-mails because of your notification settings. Change or turn them off in your profile:",
		"footer.prefsLink":     "Notification settings",
		"invite.subject":       "Invitation to %s in Qtestframe",
		"invite.intro":         "%s invites you to join %s in Qtestframe as %s.",
		"invite.local":         "Choose your name and password with the link below. The link can be used once and expires on %s.",
		"invite.sso":           "Sign in with your organisation's account using the link below. The link expires on %s.",
		"invite.action":        "Accept the invitation",
		"invite.ignore":        "If you did not expect this invitation you can ignore this e-mail.",
		"added.subject":        "You were added to %s in Qtestframe",
		"added.intro":          "%s added you to %s in Qtestframe as %s.",
		"added.action":         "Open Qtestframe",
		"report.subject":       "Report: %s",
		"report.intro":         "%s sends you the report %s (%s) of project %s. It is attached.",
		"report.scheduled":     "This is the scheduled report %s (%s) of project %s. It is attached.",
		"report.schedFooter":   "You get this report because %s subscribed you to it. Ask them, or someone who manages the project, to change the recipients.",
		"report.open":          "Reports of the project",
		"report.failed":        "The report e-mail could not be sent",
		"test.subject":         "Qtestframe test e-mail",
		"test.intro":           "This is a test e-mail from Qtestframe, sent by %s. E-mail delivery works.",
		"by":                   "by %s",
		"mfa.reset.subject":    "Your multi-factor authentication was reset",
		"mfa.reset.intro":      "%s reset the multi-factor authentication of your Qtestframe account. Your authenticator app and recovery codes no longer work, and you were signed out. Set up a new authenticator app in your profile after signing in.",
		"mfa.disabled.subject": "Multi-factor authentication was turned off",
		"mfa.disabled.intro":   "Multi-factor authentication was turned off for your Qtestframe account. Signing in now needs only your password.",
		"mfa.notYou":           "If you did not expect this, contact your administrator and change your password.",
		"mfa.action":           "Open your profile",
	},
	"sv": {
		"notification.subject": "%s",
		"notification.intro":   "%s:",
		"notification.open":    "Öppna i Qtestframe",
		"digest.subject":       "Qtestframe: %d nya notiser",
		"digest.subject1":      "Qtestframe: 1 ny notis",
		"digest.intro":         "Notiserna nedan har kommit sedan din förra sammanfattning och är fortfarande olästa.",
		"digest.more":          "Det finns fler olästa notiser; de kommer i nästa sammanfattning.",
		"digest.all":           "Alla notiser",
		"footer.prefs":         "Du får de här e-postmeddelandena på grund av dina notisinställningar. Ändra eller stäng av dem i din profil:",
		"footer.prefsLink":     "Notisinställningar",
		"invite.subject":       "Inbjudan till %s i Qtestframe",
		"invite.intro":         "%s bjuder in dig till %s i Qtestframe som %s.",
		"invite.local":         "Välj ditt namn och lösenord via länken nedan. Länken kan användas en gång och gäller till %s.",
		"invite.sso":           "Logga in med din organisations konto via länken nedan. Länken gäller till %s.",
		"invite.action":        "Anta inbjudan",
		"invite.ignore":        "Om du inte väntade dig den här inbjudan kan du bortse från meddelandet.",
		"added.subject":        "Du har lagts till i %s i Qtestframe",
		"added.intro":          "%s har lagt till dig i %s i Qtestframe som %s.",
		"added.action":         "Öppna Qtestframe",
		"report.subject":       "Rapport: %s",
		"report.intro":         "%s skickar rapporten %s (%s) för projektet %s till dig. Den är bifogad.",
		"report.scheduled":     "Det här är den schemalagda rapporten %s (%s) för projektet %s. Den är bifogad.",
		"report.schedFooter":   "Du får rapporten eftersom %s har lagt till dig som mottagare. Be hen, eller någon som förvaltar projektet, att ändra mottagarna.",
		"report.open":          "Projektets rapporter",
		"report.failed":        "Rapporten kunde inte skickas med e-post",
		"test.subject":         "Testmeddelande från Qtestframe",
		"test.intro":           "Det här är ett testmeddelande från Qtestframe, skickat av %s. E-postutskicket fungerar.",
		"by":                   "av %s",
		"mfa.reset.subject":    "Din tvåstegsverifiering har återställts",
		"mfa.reset.intro":      "%s har återställt tvåstegsverifieringen för ditt Qtestframe-konto. Din autentiseringsapp och dina återställningskoder fungerar inte längre, och du har loggats ut. Konfigurera en ny autentiseringsapp i din profil när du har loggat in.",
		"mfa.disabled.subject": "Tvåstegsverifieringen har stängts av",
		"mfa.disabled.intro":   "Tvåstegsverifieringen har stängts av för ditt Qtestframe-konto. Inloggning kräver nu bara ditt lösenord.",
		"mfa.notYou":           "Om du inte väntade dig detta, kontakta din administratör och byt lösenord.",
		"mfa.action":           "Öppna din profil",
	},
}

// mailLang returns a supported language for a preference value.
func mailLang(pref string) string {
	if _, ok := mailTexts[pref]; ok {
		return pref
	}
	return "en"
}

// tr formats the text key in lang.
func tr(lang, key string, a ...any) string {
	s, ok := mailTexts[mailLang(lang)][key]
	if !ok {
		s = mailTexts["en"][key]
	}
	if len(a) == 0 {
		return s
	}
	return fmt.Sprintf(s, a...)
}

// mailDate formats a date for e-mails.
func mailDate(lang string, t interface{ Format(string) string }) string {
	if mailLang(lang) == "sv" {
		return t.Format("2006-01-02 15:04 MST")
	}
	return t.Format("2 Jan 2006 15:04 MST")
}

type mailLink struct{ Text, URL string }

type mailItem struct {
	Title, Meta, Body, URL string
}

// mailContent is the body of an e-mail, rendered as text and HTML.
type mailContent struct {
	Lang       string
	Title      string
	Intro      []string
	Items      []mailItem
	Action     *mailLink
	After      []string
	Footer     []string
	FooterLink *mailLink
}

func (c mailContent) text() string {
	var b strings.Builder
	b.WriteString(c.Title + "\n" + strings.Repeat("=", min(len([]rune(c.Title)), 72)) + "\n\n")
	for _, p := range c.Intro {
		b.WriteString(p + "\n\n")
	}
	for _, it := range c.Items {
		b.WriteString("* " + it.Title + "\n")
		if it.Meta != "" {
			b.WriteString("  " + it.Meta + "\n")
		}
		if it.Body != "" {
			for _, l := range strings.Split(it.Body, "\n") {
				b.WriteString("  " + l + "\n")
			}
		}
		if it.URL != "" {
			b.WriteString("  " + it.URL + "\n")
		}
		b.WriteString("\n")
	}
	if c.Action != nil {
		b.WriteString(c.Action.Text + ": " + c.Action.URL + "\n\n")
	}
	for _, p := range c.After {
		b.WriteString(p + "\n\n")
	}
	if len(c.Footer) > 0 || c.FooterLink != nil {
		b.WriteString("-- \n")
		for _, p := range c.Footer {
			b.WriteString(p + "\n")
		}
		if c.FooterLink != nil {
			b.WriteString(c.FooterLink.URL + "\n")
		}
	}
	return b.String()
}

var mailHTML = template.Must(template.New("mail").Parse(`<!DOCTYPE html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>{{.Title}}</title></head>
<body style="margin:0;padding:0;background:#f4f5f7;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#1f2328">
<div style="max-width:600px;margin:0 auto;padding:24px 16px">
<div style="background:#ffffff;border-radius:8px;padding:24px;border:1px solid #d8dee4">
<h1 style="font-size:20px;margin:0 0 16px">{{.Title}}</h1>
{{range .Intro}}<p style="margin:0 0 12px;line-height:1.5">{{.}}</p>{{end}}
{{if .Items}}<ul style="list-style:none;margin:0 0 12px;padding:0">{{range .Items}}<li style="border-top:1px solid #d8dee4;padding:10px 0">
<div style="font-weight:600">{{if .URL}}<a href="{{.URL}}" style="color:#0b57d0">{{.Title}}</a>{{else}}{{.Title}}{{end}}</div>
{{if .Meta}}<div style="font-size:13px;color:#57606a">{{.Meta}}</div>{{end}}
{{if .Body}}<div style="margin-top:4px;white-space:pre-wrap;line-height:1.5">{{.Body}}</div>{{end}}
</li>{{end}}</ul>{{end}}
{{with .Action}}<p style="margin:20px 0"><a href="{{.URL}}" style="display:inline-block;background:#0b57d0;color:#ffffff;text-decoration:none;padding:10px 18px;border-radius:6px;font-weight:600">{{.Text}}</a></p>{{end}}
{{range .After}}<p style="margin:0 0 12px;line-height:1.5">{{.}}</p>{{end}}
</div>
{{if or .Footer .FooterLink}}<div style="font-size:12px;color:#57606a;padding:12px 4px;line-height:1.5">{{range .Footer}}<p style="margin:0 0 6px">{{.}}</p>{{end}}{{with .FooterLink}}<a href="{{.URL}}" style="color:#57606a">{{.Text}}</a>{{end}}</div>{{end}}
</div></body></html>`))

func (c mailContent) html() string {
	var b bytes.Buffer
	if err := mailHTML.Execute(&b, c); err != nil {
		return ""
	}
	return b.String()
}
