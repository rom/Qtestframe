package chat

import (
	"html"
	"strconv"
	"strings"
)

// The payload of each provider. Every format starts with a plain text
// (notifications, screen readers, clients without cards), then the rich
// part: the item key and title as a link to the item, the status in text
// (the colour only repeats it), who did it and the summary.

// ---------------------------------------------------------------- Slack

// slackEscape escapes the control characters of Slack mrkdwn.
func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func slackLink(m Message) string {
	t := slackEscape(m.itemText())
	if m.URL == "" {
		return "*" + t + "*"
	}
	return "*<" + m.URL + "|" + strings.ReplaceAll(t, "|", "¦") + ">*"
}

func slackSection(lang string, m Message, digest bool) map[string]any {
	lines := []string{}
	if digest {
		lines = append(lines, "*"+slackEscape(EventLabel(lang, m.Event))+"*: "+slackLink(m))
	} else if m.itemText() != "" {
		lines = append(lines, slackLink(m))
	}
	if l := plainLine(lang, m, digest); l != "" {
		lines = append(lines, slackEscape(l))
	}
	if m.Summary != "" {
		lines = append(lines, slackEscape(m.Summary))
	}
	return map[string]any{"type": "section", "text": map[string]any{"type": "mrkdwn", "text": shorten(strings.Join(lines, "\n"), 3000)}}
}

func slackPayload(lang string, all []Message) map[string]any {
	msgs, more := prepared(all, maxItems[Slack])
	head := header(lang, all)
	fallback := head
	if len(msgs) == 1 && msgs[0].itemText() != "" {
		fallback += ": " + msgs[0].itemText()
	}
	out := map[string]any{"text": shorten(slackEscape(fallback), 3000), "unfurl_links": false, "unfurl_media": false}
	headerBlock := map[string]any{"type": "header", "text": map[string]any{"type": "plain_text", "text": shorten(head, 150)}}
	if len(msgs) == 1 {
		m := msgs[0]
		blocks := []any{slackSection(lang, m, false)}
		if m.URL != "" {
			blocks = append(blocks, map[string]any{"type": "actions", "elements": []any{map[string]any{
				"type": "button", "text": map[string]any{"type": "plain_text", "text": shorten(openText(lang, m), 75)}, "url": m.URL}}})
		}
		out["blocks"] = []any{headerBlock}
		out["attachments"] = []any{map[string]any{"color": m.Color(), "blocks": blocks}}
		return out
	}
	blocks := []any{headerBlock}
	for _, m := range msgs {
		blocks = append(blocks, slackSection(lang, m, true))
	}
	if more > 0 {
		blocks = append(blocks, map[string]any{"type": "context", "elements": []any{map[string]any{"type": "mrkdwn", "text": slackEscape(moreText(lang, more))}}})
	}
	out["blocks"] = blocks
	return out
}

// ---------------------------------------------------------------- Microsoft Teams

var adaptiveColor = map[string]string{ToneGood: "Good", ToneWarning: "Warning", ToneDanger: "Attention", ToneInfo: "Accent"}

// mdEscape escapes the Markdown characters that would format or link.
func mdEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`, "`", "\\`", "<", "&lt;", ">", "&gt;", "#", `\#`, "~", `\~`, "|", `\|`).Replace(s)
}

func mdLink(m Message) string {
	t := mdEscape(m.itemText())
	if m.URL == "" {
		return "**" + t + "**"
	}
	return "[" + t + "](" + strings.NewReplacer("(", "%28", ")", "%29", " ", "%20").Replace(m.URL) + ")"
}

func teamsPayload(lang string, all []Message) map[string]any {
	msgs, more := prepared(all, maxItems[Teams])
	digest := len(msgs) > 1
	body := []any{map[string]any{"type": "TextBlock", "text": mdEscape(header(lang, all)), "weight": "Bolder", "size": "Medium", "wrap": true, "style": "heading"}}
	for _, m := range msgs {
		items := []any{}
		line := mdLink(m)
		if digest {
			line = mdEscape(EventLabel(lang, m.Event)) + ": " + line
		}
		if m.itemText() != "" || digest {
			items = append(items, map[string]any{"type": "TextBlock", "text": line, "wrap": true, "weight": "Bolder"})
		}
		if m.Status != "" {
			items = append(items, map[string]any{"type": "TextBlock", "text": mdEscape(text(lang, "status") + ": " + ValueLabel(lang, m.Status)), "wrap": true, "color": adaptiveColor[m.ToneOf()], "spacing": "None"})
		}
		var fs []any
		for _, f := range facts(lang, m, digest) {
			if f[0] == text(lang, "status") {
				continue
			}
			fs = append(fs, map[string]any{"title": f[0], "value": mdEscape(f[1])})
		}
		if len(fs) > 0 {
			items = append(items, map[string]any{"type": "FactSet", "facts": fs, "spacing": "Small"})
		}
		if m.Summary != "" {
			items = append(items, map[string]any{"type": "TextBlock", "text": mdEscape(m.Summary), "wrap": true, "isSubtle": true, "spacing": "Small"})
		}
		body = append(body, map[string]any{"type": "Container", "items": items, "separator": digest, "spacing": "Medium"})
	}
	if more > 0 {
		body = append(body, map[string]any{"type": "TextBlock", "text": mdEscape(moreText(lang, more)), "wrap": true, "isSubtle": true})
	}
	card := map[string]any{"$schema": "http://adaptivecards.io/schemas/adaptive-card.json", "type": "AdaptiveCard", "version": "1.4",
		"body": body, "msteams": map[string]any{"width": "Full"}}
	if !digest && msgs[0].URL != "" {
		card["actions"] = []any{map[string]any{"type": "Action.OpenUrl", "title": openText(lang, msgs[0]), "url": msgs[0].URL}}
	}
	summary := header(lang, all)
	if !digest && msgs[0].itemText() != "" {
		summary += ": " + msgs[0].itemText()
	}
	return map[string]any{"type": "message", "summary": shorten(summary, 200), "attachments": []any{map[string]any{
		"contentType": "application/vnd.microsoft.card.adaptive", "contentUrl": nil, "content": card}}}
}

func teamsLegacyPayload(lang string, all []Message) map[string]any {
	msgs, more := prepared(all, maxItems[TeamsLegacy])
	digest := len(msgs) > 1
	var sections []any
	for _, m := range msgs {
		sec := map[string]any{"activityTitle": mdEscape(m.itemText()), "markdown": true}
		if digest {
			sec["activityTitle"] = mdEscape(EventLabel(lang, m.Event)) + ": " + mdLink(m)
		} else if m.URL != "" {
			sec["activityTitle"] = mdLink(m)
		}
		var fs []any
		for _, f := range facts(lang, m, digest) {
			fs = append(fs, map[string]any{"name": f[0], "value": mdEscape(f[1])})
		}
		if len(fs) > 0 {
			sec["facts"] = fs
		}
		if m.Summary != "" {
			sec["text"] = mdEscape(m.Summary)
		}
		sections = append(sections, sec)
	}
	if more > 0 {
		sections = append(sections, map[string]any{"text": mdEscape(moreText(lang, more))})
	}
	out := map[string]any{"@type": "MessageCard", "@context": "https://schema.org/extensions", "summary": shorten(header(lang, all), 200),
		"themeColor": strings.TrimPrefix(msgs[0].Color(), "#"), "title": mdEscape(header(lang, all)), "sections": sections}
	if digest {
		out["themeColor"] = strings.TrimPrefix(toneHex[ToneInfo], "#")
	} else if msgs[0].URL != "" {
		out["potentialAction"] = []any{map[string]any{"@type": "OpenUri", "name": openText(lang, msgs[0]), "targets": []any{map[string]any{"os": "default", "uri": msgs[0].URL}}}}
	}
	return out
}

// ---------------------------------------------------------------- Mattermost, Rocket.Chat

// attachmentPayload: Mattermost and Rocket.Chat incoming webhooks take a
// Markdown text and Slack-style attachments.
func attachmentPayload(provider, lang string, all []Message) map[string]any {
	msgs, more := prepared(all, maxItems[provider])
	digest := len(msgs) > 1
	var atts []any
	for _, m := range msgs {
		title := m.itemText()
		if digest {
			title = EventLabel(lang, m.Event) + ": " + title
		}
		a := map[string]any{"fallback": shorten(EventLabel(lang, m.Event)+": "+m.itemText(), 300), "color": m.Color(), "title": title}
		if m.URL != "" {
			a["title_link"] = m.URL
		}
		if m.Actor != "" {
			a["author_name"] = m.Actor
		}
		if m.Summary != "" {
			a["text"] = mdEscape(m.Summary)
		}
		var fs []any
		for _, f := range facts(lang, m, digest) {
			if f[0] == text(lang, "by") {
				continue
			}
			fs = append(fs, map[string]any{"short": true, "title": f[0], "value": mdEscape(f[1])})
		}
		if len(fs) > 0 {
			a["fields"] = fs
		}
		atts = append(atts, a)
	}
	head := "**" + mdEscape(header(lang, all)) + "**"
	if more > 0 {
		head += "\n" + mdEscape(moreText(lang, more))
	}
	out := map[string]any{"text": head, "attachments": atts}
	if provider == Mattermost {
		out["username"] = "Qtestframe"
	} else {
		out["alias"] = "Qtestframe"
	}
	return out
}

// ---------------------------------------------------------------- Google Chat

func googleChatPayload(lang string, all []Message) map[string]any {
	msgs, more := prepared(all, maxItems[GoogleChat])
	digest := len(msgs) > 1
	var sections []any
	for _, m := range msgs {
		var widgets []any
		label := EventLabel(lang, m.Event)
		if m.itemText() != "" {
			widgets = append(widgets, map[string]any{"decoratedText": map[string]any{"topLabel": label, "text": "<b>" + html.EscapeString(m.itemText()) + "</b>", "wrapText": true}})
		}
		if l := plainLine(lang, m, digest); l != "" {
			widgets = append(widgets, map[string]any{"textParagraph": map[string]any{"text": html.EscapeString(l)}})
		}
		if m.Summary != "" {
			widgets = append(widgets, map[string]any{"textParagraph": map[string]any{"text": html.EscapeString(m.Summary)}})
		}
		if m.URL != "" {
			widgets = append(widgets, map[string]any{"buttonList": map[string]any{"buttons": []any{map[string]any{
				"text": openText(lang, m), "onClick": map[string]any{"openLink": map[string]any{"url": m.URL}}}}}})
		}
		if len(widgets) == 0 {
			widgets = append(widgets, map[string]any{"textParagraph": map[string]any{"text": html.EscapeString(label)}})
		}
		sections = append(sections, map[string]any{"widgets": widgets})
	}
	if more > 0 {
		sections = append(sections, map[string]any{"widgets": []any{map[string]any{"textParagraph": map[string]any{"text": html.EscapeString(moreText(lang, more))}}}})
	}
	head := map[string]any{"title": shorten(header(lang, all), 200)}
	if p := msgs[0].Project; p != "" {
		head["subtitle"] = p
	}
	fallback := header(lang, all)
	if !digest && msgs[0].itemText() != "" {
		fallback += ": " + msgs[0].itemText()
	}
	return map[string]any{"text": shorten(fallback, 4000), "cardsV2": []any{map[string]any{"cardId": "qtestframe", "card": map[string]any{"header": head, "sections": sections}}}}
}

// ---------------------------------------------------------------- Discord

func discordPayload(lang string, all []Message) map[string]any {
	msgs, more := prepared(all, maxItems[Discord])
	digest := len(msgs) > 1
	// 6000 characters over all embeds of a message
	budget := 5600 / len(msgs)
	var embeds []any
	for _, m := range msgs {
		title := m.itemText()
		if title == "" {
			title = EventLabel(lang, m.Event)
		}
		e := map[string]any{"title": shorten(title, 256), "color": hexInt(m.Color()), "author": map[string]any{"name": shorten(EventLabel(lang, m.Event), 256)}}
		if m.URL != "" {
			e["url"] = m.URL
		}
		if m.Summary != "" {
			e["description"] = shorten(mdEscape(m.Summary), min(4096, max(100, budget-len(title)-200)))
		}
		var fs []any
		for _, f := range facts(lang, m, true) {
			fs = append(fs, map[string]any{"name": f[0], "value": shorten(f[1], 200), "inline": true})
		}
		if len(fs) > 0 {
			e["fields"] = fs
		}
		if m.Project != "" {
			e["footer"] = map[string]any{"text": shorten(m.Project, 200)}
		}
		if !m.Time.IsZero() {
			e["timestamp"] = m.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		embeds = append(embeds, e)
	}
	out := map[string]any{"username": "Qtestframe", "embeds": embeds, "allowed_mentions": map[string]any{"parse": []any{}}}
	if digest {
		c := mdEscape(header(lang, all))
		if more > 0 {
			c += "\n" + mdEscape(moreText(lang, more))
		}
		out["content"] = shorten(c, 2000)
	}
	return out
}

func hexInt(c string) int {
	n, _ := strconv.ParseInt(strings.TrimPrefix(c, "#"), 16, 32)
	return int(n)
}

// ---------------------------------------------------------------- Matrix

func matrixPayload(lang string, all []Message) map[string]any {
	msgs, more := prepared(all, maxItems[Matrix])
	digest := len(msgs) > 1
	var plain, rich strings.Builder
	head := header(lang, all)
	plain.WriteString(head)
	rich.WriteString("<p><strong>" + html.EscapeString(head) + "</strong></p>")
	item := func(m Message) (string, string) {
		p := m.itemText()
		h := html.EscapeString(p)
		if m.URL != "" {
			h = `<a href="` + html.EscapeString(m.URL) + `">` + h + `</a>`
			if p != "" {
				p += " <" + m.URL + ">"
			} else {
				p = m.URL
			}
		}
		if digest {
			label := EventLabel(lang, m.Event)
			p, h = label+": "+p, html.EscapeString(label)+": "+h
		}
		if l := plainLine(lang, m, digest); l != "" {
			p += " — " + l
			h += "<br>" + html.EscapeString(l)
		}
		if m.Summary != "" {
			p += "\n" + m.Summary
			h += "<br><em>" + strings.ReplaceAll(html.EscapeString(m.Summary), "\n", "<br>") + "</em>"
		}
		return p, h
	}
	if digest {
		rich.WriteString("<ul>")
		for _, m := range msgs {
			p, h := item(m)
			plain.WriteString("\n• " + p)
			rich.WriteString("<li>" + h + "</li>")
		}
		rich.WriteString("</ul>")
		if more > 0 {
			plain.WriteString("\n" + moreText(lang, more))
			rich.WriteString("<p>" + html.EscapeString(moreText(lang, more)) + "</p>")
		}
	} else {
		p, h := item(msgs[0])
		plain.WriteString("\n" + p)
		rich.WriteString("<p>" + h + "</p>")
	}
	return map[string]any{"msgtype": "m.notice", "body": shorten(plain.String(), 30000), "format": "org.matrix.custom.html", "formatted_body": shorten(rich.String(), 30000)}
}
