package tracker

import (
	"regexp"
	"strconv"
	"strings"
)

// JiraMarkdown converts Jira wiki markup (the descriptions of the REST API
// v2) to Markdown: headings, quotes, lists, tables, code and noformat
// blocks, bold, italic, strikethrough, monospace, links, mentions and
// images. Colours and other macros keep only their text.
func JiraMarkdown(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	var out []string
	inCode, inQuote, inPanel := false, false, false
	tableHead := false
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if inCode {
			if i := strings.Index(line, "{code}"); i >= 0 || strings.Contains(line, "{noformat}") {
				if i < 0 {
					i = strings.Index(line, "{noformat}")
				}
				if pre := strings.TrimRight(line[:i], " "); pre != "" {
					out = append(out, pre)
				}
				out = append(out, "```")
				inCode = false
				continue
			}
			out = append(out, line)
			continue
		}
		if m := codeStartRe.FindStringSubmatch(trimmed); m != nil {
			lang := m[2]
			rest := strings.TrimSpace(trimmed[len(m[0]):])
			if end := strings.Index(rest, "{"+m[1]+"}"); end >= 0 {
				// a one-line block
				out = append(out, "```"+lang, rest[:end], "```")
				continue
			}
			out = append(out, "```"+lang)
			if rest != "" {
				out = append(out, rest)
			}
			inCode = true
			continue
		}
		if quoteRe.MatchString(trimmed) {
			inQuote = !inQuote
			if t := strings.TrimSpace(quoteRe.ReplaceAllString(trimmed, "")); t != "" {
				out = append(out, "> "+jiraInline(t))
			}
			continue
		}
		if panelRe.MatchString(trimmed) {
			inPanel = !inPanel
			continue
		}
		var md string
		switch {
		case trimmed == "":
			tableHead = false
			md = ""
		case headingRe.MatchString(trimmed):
			m := headingRe.FindStringSubmatch(trimmed)
			md = strings.Repeat("#", int(m[1][0]-'0')) + " " + jiraInline(strings.TrimSpace(m[2]))
		case strings.HasPrefix(trimmed, "bq. "):
			md = "> " + jiraInline(strings.TrimSpace(trimmed[4:]))
		case trimmed == "----":
			md = "---"
		case strings.HasPrefix(trimmed, "||"):
			cells := splitRow(strings.Trim(trimmed, "|"), "||")
			md = "| " + strings.Join(cells, " | ") + " |\n|" + strings.Repeat(" --- |", len(cells))
			tableHead = true
		case strings.HasPrefix(trimmed, "|"):
			cells := splitRow(strings.Trim(trimmed, "|"), "|")
			md = "| " + strings.Join(cells, " | ") + " |"
			if !tableHead {
				// a table without a heading row: Markdown needs one
				md = "|" + strings.Repeat("   |", len(cells)) + "\n|" + strings.Repeat(" --- |", len(cells)) + "\n" + md
				tableHead = true
			}
		case listRe.MatchString(trimmed):
			m := listRe.FindStringSubmatch(trimmed)
			marks := m[1]
			bullet := "- "
			if strings.HasSuffix(marks, "#") {
				bullet = "1. "
			}
			md = strings.Repeat("  ", len(marks)-1) + bullet + jiraInline(m[2])
		default:
			md = jiraInline(trimmed)
		}
		if inQuote && md != "" {
			md = "> " + md
		}
		out = append(out, md)
	}
	if inCode {
		out = append(out, "```")
	}
	return strings.TrimSpace(collapseBlank(strings.Join(out, "\n")))
}

var (
	codeStartRe = regexp.MustCompile(`^\{(code|noformat)(?::(?:language=)?([A-Za-z0-9+#_-]*)[^}]*)?\}`)
	quoteRe     = regexp.MustCompile(`\{quote\}`)
	panelRe     = regexp.MustCompile(`^\{panel(?::[^}]*)?\}$`)
	headingRe   = regexp.MustCompile(`^h([1-6])\.\s+(.*)$`)
	listRe      = regexp.MustCompile(`^([*#-]+)\s+(.*)$`)
	monoRe      = regexp.MustCompile(`\{\{(.+?)\}\}`)
	linkRe      = regexp.MustCompile(`\[([^\[\]|]*)\|([^\[\]]+)\]`)
	bareLinkRe  = regexp.MustCompile(`\[((?:https?|mailto):[^\[\]|]+)\]`)
	mentionRe   = regexp.MustCompile(`\[~(?:accountid:)?([^\[\]]+)\]`)
	imageRe     = regexp.MustCompile(`!([^!\s|]+\.(?i:png|jpe?g|gif|svg|webp|bmp))(?:\|[^!]*)?!`)
	boldRe      = regexp.MustCompile(`(^|[\s(\[>])\*([^*\s](?:[^*]*[^*\s])?)\*($|[\s).,:;!?\]])`)
	italicRe    = regexp.MustCompile(`(^|[\s(\[>])_([^_\s](?:[^_]*[^_\s])?)_($|[\s).,:;!?\]])`)
	strikeRe    = regexp.MustCompile(`(^|[\s(])-([^-\s](?:[^-]*[^-\s])?)-($|[\s).,:;!?])`)
	underRe     = regexp.MustCompile(`(^|[\s(])\+([^+\s](?:[^+]*[^+\s])?)\+($|[\s).,:;!?])`)
	colorRe     = regexp.MustCompile(`\{color(?::[^}]*)?\}`)
	blankRe     = regexp.MustCompile(`\n{3,}`)
)

// jiraInline converts the inline markup of a line (outside code).
func jiraInline(s string) string {
	// monospace first; its content is kept apart from the other rules
	var codes []string
	s = monoRe.ReplaceAllStringFunc(s, func(m string) string {
		codes = append(codes, "`"+monoRe.FindStringSubmatch(m)[1]+"`")
		return "\x00" + strconv.Itoa(len(codes)-1) + "\x00"
	})
	s = colorRe.ReplaceAllString(s, "")
	s = imageRe.ReplaceAllString(s, "![]($1)")
	s = mentionRe.ReplaceAllString(s, "@$1")
	s = linkRe.ReplaceAllStringFunc(s, func(m string) string {
		p := linkRe.FindStringSubmatch(m)
		text := strings.TrimSpace(p[1])
		if text == "" {
			return "<" + p[2] + ">"
		}
		return "[" + text + "](" + p[2] + ")"
	})
	s = bareLinkRe.ReplaceAllString(s, "<$1>")
	for range 2 { // adjacent matches share a boundary
		s = boldRe.ReplaceAllString(s, "$1\x02$2\x02$3")
		s = italicRe.ReplaceAllString(s, "$1\x03$2\x03$3")
		s = strikeRe.ReplaceAllString(s, "$1~~$2~~$3")
		s = underRe.ReplaceAllString(s, "$1$2$3")
	}
	s = strings.NewReplacer("\x02", "**", "\x03", "*", `\\`, "  \n").Replace(s)
	for i, c := range codes {
		s = strings.Replace(s, "\x00"+strconv.Itoa(i)+"\x00", c, 1)
	}
	return s
}

func splitRow(row, sep string) []string {
	parts := strings.Split(row, sep)
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(jiraInline(strings.TrimSpace(strings.Trim(p, "|"))), "|", `\|`)
	}
	return parts
}

func collapseBlank(s string) string { return blankRe.ReplaceAllString(s, "\n\n") }
