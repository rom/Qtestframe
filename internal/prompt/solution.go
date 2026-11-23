package prompt

import (
	"fmt"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// Solution prompts: architecture, design, implementation plan and the full
// solution, built from the solution preamble, a summary of the project and
// the requirements of a scope. They can be compact (titles and criteria
// only) and split into sequential parts (per top-level folder, or by size)
// that share the preamble and carry a continuation instruction.

// Solution prompt kinds.
const (
	Architecture       = "architecture"
	Design             = "design"
	ImplementationPlan = "implementation_plan"
	FullSolution       = "full_solution"
)

// SolutionKinds lists the solution prompt kinds (all take requirements).
var SolutionKinds = []string{Architecture, Design, ImplementationPlan, FullSolution}

// IsSolution reports whether a kind is a solution prompt.
func IsSolution(kind string) bool { return slices.Contains(SolutionKinds, kind) }

// Output formats a solution prompt can ask for (besides Markdown).
const (
	FormatMermaid = "mermaid"
	FormatC4      = "c4"
	FormatOpenAPI = "openapi"
	FormatSQL     = "sql"
)

// Formats lists the output formats.
var Formats = []string{FormatMermaid, FormatC4, FormatOpenAPI, FormatSQL}

// DefaultFormats are the formats of a kind when none are given.
func DefaultFormats(kind string) []string {
	switch kind {
	case Architecture:
		return []string{FormatMermaid, FormatC4}
	case Design:
		return []string{FormatMermaid, FormatOpenAPI, FormatSQL}
	case FullSolution:
		return []string{FormatMermaid, FormatC4, FormatOpenAPI, FormatSQL}
	}
	return []string{}
}

// Split modes.
const (
	SplitNone   = "none"
	SplitFolder = "folder"
	SplitSize   = "size"
)

// Splits lists the split modes.
var Splits = []string{SplitNone, SplitFolder, SplitSize}

// Token budgets of a part (split by size; folder parts larger than the
// budget are split further).
const (
	MinPartTokens     = 1000
	MaxPartTokens     = 1000000
	DefaultPartTokens = 8000
)

// EstimateTokens estimates the tokens of a text: characters / 4.
func EstimateTokens(s string) int { return (len([]rune(s)) + 3) / 4 }

// Risk is a project risk linked to a requirement.
type Risk struct {
	Key, Title, Level, Mitigation string
	// Attack: MITRE ATT&CK / ATLAS technique ids; Emb3d: EMB3D threats.
	Attack, Emb3d []string
}

// Count is a name with a number.
type Count struct {
	Name string
	N    int
}

// Summary is the project summary of a solution prompt.
type Summary struct {
	// Scope describes the chosen requirements ("whole project", "folder
	// Payments", "query tag:checkout" …).
	Scope string
	// Total is the number of requirements in the project; InScope in the
	// scope (all parts together).
	Total, InScope int
	// Tests in the project.
	Tests    int
	Folders  []Count
	Releases []string
	Variants []string
}

// Part is one prompt of a split solution prompt.
type Part struct {
	Index  int    `json:"index"`
	Title  string `json:"title"`
	Label  string `json:"label"`
	Text   string `json:"prompt"`
	Items  int    `json:"items"`
	Tokens int    `json:"tokens"`
}

// validateSolution checks the options of a solution prompt.
func validateSolution(in *Input) error {
	for _, f := range in.Formats {
		if !slices.Contains(Formats, f) {
			return fmt.Errorf("formats must be among %s", strings.Join(Formats, ", "))
		}
	}
	if in.Split == "" {
		in.Split = SplitNone
	}
	if !slices.Contains(Splits, in.Split) {
		return fmt.Errorf("split must be one of %s", strings.Join(Splits, ", "))
	}
	if in.MaxTokens != 0 && (in.MaxTokens < MinPartTokens || in.MaxTokens > MaxPartTokens) {
		return fmt.Errorf("max_tokens must be between %d and %d", MinPartTokens, MaxPartTokens)
	}
	if in.Split == SplitSize && in.MaxTokens == 0 {
		in.MaxTokens = DefaultPartTokens
	}
	return nil
}

// group is the requirements of one part.
type group struct {
	label string
	reqs  []Requirement
}

// topFolder is the first segment of a folder path.
func topFolder(path string) string {
	if i := strings.Index(path, "/"); i >= 0 {
		return strings.TrimSpace(path[:i])
	}
	return strings.TrimSpace(path)
}

func buildSolution(in Input) Result {
	tx := solTexts[in.Lang]
	if tx == nil {
		in.Lang = "en"
		tx = solTexts["en"]
	}
	project := strings.TrimSpace(in.ProjectName)
	if in.ProjectKey != "" {
		project += " (" + in.ProjectKey + ")"
	}
	title := fmt.Sprintf(tx.s["title."+in.Kind], project)

	// the groups of requirements
	var groups []group
	switch in.Split {
	case SplitFolder:
		idx := map[string]int{}
		for _, r := range in.Requirements {
			l := r.Group
			if l == "" {
				l = topFolder(r.Folder)
			}
			if l == "" {
				l = tx.s["noFolder"]
			}
			i, ok := idx[l]
			if !ok {
				i = len(groups)
				idx[l] = i
				groups = append(groups, group{label: l})
			}
			groups[i].reqs = append(groups[i].reqs, r)
		}
	default:
		groups = []group{{reqs: in.Requirements}}
	}
	if len(groups) == 0 {
		groups = []group{{}}
	}
	// by size: split what does not fit the budget
	if in.MaxTokens > 0 && in.Split != SplitNone {
		overhead := EstimateTokens(renderSolution(in, tx, title, nil, "", 1, 2))
		budget := max(in.MaxTokens-overhead, 1)
		var out []group
		for _, g := range groups {
			cur, used := group{label: g.label}, 0
			for _, r := range g.reqs {
				n := EstimateTokens(requirementText(tx, r, in.Compact))
				if len(cur.reqs) > 0 && used+n > budget {
					out = append(out, cur)
					cur, used = group{label: g.label}, 0
				}
				cur.reqs = append(cur.reqs, r)
				used += n
			}
			out = append(out, cur)
		}
		// number the pieces of a folder: "Payments (2)"
		count := map[string]int{}
		for _, g := range out {
			count[g.label]++
		}
		seen := map[string]int{}
		for i := range out {
			l := out[i].label
			switch {
			case l == "" && len(out[i].reqs) > 0:
				// by size: the range of keys
				first, last := out[i].reqs[0].Key, out[i].reqs[len(out[i].reqs)-1].Key
				out[i].label = first
				if last != first {
					out[i].label += " – " + last
				}
			case count[l] > 1:
				seen[l]++
				out[i].label = fmt.Sprintf("%s (%d)", l, seen[l])
			}
		}
		groups = out
	}

	res := Result{Kind: in.Kind, Lang: in.Lang, Title: title, Items: len(in.Requirements)}
	var texts []string
	for i, g := range groups {
		var earlier []string
		for _, e := range groups[:i] {
			earlier = append(earlier, e.label)
		}
		txt := renderSolution(in, tx, title, g.reqs, strings.Join(earlier, ", "), i+1, len(groups))
		pt := title
		if len(groups) > 1 {
			pt = fmt.Sprintf(tx.s["partTitle"], title, i+1, len(groups))
			if g.label != "" {
				pt += ": " + g.label
			}
		}
		res.Parts = append(res.Parts, Part{Index: i + 1, Title: pt, Label: g.label, Text: txt, Items: len(g.reqs), Tokens: EstimateTokens(txt)})
		texts = append(texts, txt)
	}
	res.Text = strings.Join(texts, "\n---\n\n")
	res.Tokens = EstimateTokens(res.Text)
	return res
}

// renderSolution writes the prompt of one part (n parts in all).
func renderSolution(in Input, tx *solLang, title string, reqs []Requirement, earlier string, i, n int) string {
	b := &builder{tx: texts[in.Lang], lang: in.Lang}
	if b.tx == nil {
		b.tx = texts["en"]
	}
	if n > 1 {
		b.line("# " + fmt.Sprintf(tx.s["partTitle"], title, i, n))
	} else {
		b.line("# " + title)
	}
	b.blank()
	b.para(tx.s["role."+in.Kind])
	b.para(fmt.Sprintf(tx.s["task."+in.Kind], len(reqs)))

	// the preamble
	b.line("## " + tx.s["h.preamble"])
	b.blank()
	pre := in.Preamble
	if pre == nil || pre.Empty() {
		b.para("_" + tx.s["noPreamble"] + "_")
	} else {
		if pre.ArchitectureStyle != "" {
			b.line("- **" + tx.labels["architecture_style"] + ":** " + or(tx.styles[pre.ArchitectureStyle], pre.ArchitectureStyle))
		}
		for _, f := range model.PreambleFields {
			v := strings.TrimSpace(Redact(*pre.Field(f)))
			if v == "" {
				continue
			}
			lines := strings.Split(v, "\n")
			if len(lines) == 1 {
				b.line("- **" + tx.labels[f] + ":** " + v)
				continue
			}
			b.line("- **" + tx.labels[f] + ":**")
			for _, l := range lines {
				if l = strings.TrimRight(l, " \t"); l != "" {
					b.line("  " + l)
				}
			}
		}
		b.blank()
	}
	if tc := strings.TrimSpace(in.TechContext); tc != "" {
		b.line("### " + b.tx.s["h.tech"])
		b.blank()
		b.para(Redact(tc))
	}

	// the project summary
	sum := in.Summary
	b.line("## " + tx.s["h.summary"])
	b.blank()
	b.line("- " + b.tx.s["project"] + ": " + strings.TrimSpace(in.ProjectName+" ("+in.ProjectKey+")"))
	if d := strings.TrimSpace(in.ProjectDescription); d != "" {
		b.line("- " + b.tx.s["description"] + ": " + oneLine(d))
	}
	inScope := max(sum.InScope, len(in.Requirements))
	b.line("- " + fmt.Sprintf(tx.s["sum.scope"], or(sum.Scope, tx.s["wholeProject"]), inScope, max(sum.Total, inScope)))
	if n > 1 {
		b.line("- " + fmt.Sprintf(tx.s["sum.part"], i, n, len(reqs)))
	}
	if ob := obligations(reqs); ob != "" {
		b.line("- " + tx.s["sum.obligations"] + ": " + ob)
	}
	if len(sum.Folders) > 0 {
		var fs []string
		for _, f := range sum.Folders {
			fs = append(fs, fmt.Sprintf("%s (%d)", f.Name, f.N))
		}
		b.line("- " + tx.s["sum.folders"] + ": " + strings.Join(fs, ", "))
	}
	if len(sum.Releases) > 0 {
		b.line("- " + tx.s["sum.releases"] + ": " + strings.Join(sum.Releases, ", "))
	}
	if len(sum.Variants) > 0 {
		b.line("- " + tx.s["sum.variants"] + ": " + strings.Join(sum.Variants, ", "))
	}
	risks, tests := map[string]bool{}, map[string]bool{}
	for _, r := range reqs {
		for _, k := range r.Risks {
			risks[k.Key] = true
		}
		for _, t := range r.Tests {
			tests[t] = true
		}
	}
	b.line("- " + fmt.Sprintf(tx.s["sum.links"], len(risks), len(tests), sum.Tests))
	b.blank()

	// the requirements
	b.line("## " + b.tx.s["h.requirements"])
	b.blank()
	if len(reqs) == 0 {
		b.para("_" + b.tx.s["noItems"] + "_")
	} else {
		b.para(b.tx.s["rfc2119"])
		if in.Compact {
			b.para("_" + tx.s["compactNote"] + "_")
		}
		for _, r := range reqs {
			b.sb.WriteString(requirementText(tx, r, in.Compact))
		}
	}
	if in.Kind == ImplementationPlan || in.Kind == FullSolution {
		if order, cycles := dependencyOrder(reqs); len(order) > 0 {
			b.line("## " + tx.s["h.order"])
			b.blank()
			b.para(tx.s["orderIntro"])
			b.line(strings.Join(order, " → "))
			b.blank()
			for _, c := range cycles {
				b.line("- " + tx.s["cycle"] + ": " + strings.Join(c, ", "))
			}
			if len(cycles) > 0 {
				b.blank()
			}
		}
	}

	// instructions
	b.line("## " + b.tx.s["h.instructions"])
	b.blank()
	if in.Kind == FullSolution {
		for _, k := range []string{Architecture, Design, ImplementationPlan} {
			b.line("### " + tx.s["part."+k])
			b.blank()
			for _, s := range tx.instructions[k] {
				b.line("- " + s)
			}
			b.blank()
		}
		b.line("### " + tx.s["h.general"])
		b.blank()
	} else {
		for _, s := range tx.instructions[in.Kind] {
			b.line("- " + s)
		}
	}
	for _, s := range tx.common {
		b.line("- " + s)
	}
	b.line("- " + tx.formats["markdown"])
	for _, f := range Formats {
		if slices.Contains(in.Formats, f) {
			b.line("- " + tx.formats[f])
		}
	}
	b.blank()
	if notes := strings.TrimSpace(in.Notes); notes != "" {
		b.line("### " + b.tx.s["h.notes"])
		b.blank()
		b.para(notes)
	}
	if n > 1 {
		b.line("## " + tx.s["h.continuation"])
		b.blank()
		mode := tx.s["by."+in.Split]
		switch i {
		case 1:
			b.para(fmt.Sprintf(tx.s["cont.first"], i, n, mode))
		case n:
			b.para(fmt.Sprintf(tx.s["cont.last"], i, n, earlier))
		default:
			b.para(fmt.Sprintf(tx.s["cont.middle"], i, n, earlier))
		}
	}

	// the answer
	b.line("## " + b.tx.s["h.answer"])
	b.blank()
	k := 0
	for _, s := range tx.answers[in.Kind] {
		k++
		b.line(fmt.Sprintf("%d. %s", k, s))
	}
	for _, s := range tx.tail {
		k++
		b.line(fmt.Sprintf("%d. %s", k, s))
	}
	b.blank()
	b.line(tx.s["traceHeader"])
	b.line("|---|---|---|---|")
	b.line(tx.s["traceExample"])
	return strings.TrimRight(b.sb.String(), "\n") + "\n"
}

// requirementText renders one requirement of a solution prompt.
func requirementText(tx *solLang, r Requirement, compact bool) string {
	lang := tx.lang
	b := &builder{tx: texts[lang], lang: lang}
	s := b.tx.s
	b.line(fmt.Sprintf("### %s %s", r.Key, r.Title))
	b.blank()
	var meta []string
	if r.Obligation != "" {
		meta = append(meta, strings.ToUpper(strings.ReplaceAll(r.Obligation, "_", " ")))
	}
	for _, v := range []struct{ group, value string }{{"type", r.Type}, {"priority", r.Priority}, {"status", r.Status}} {
		if v.value != "" && (!compact || v.group == "priority") {
			meta = append(meta, s["f."+v.group]+": "+b.label(v.group, v.value))
		}
	}
	if r.Folder != "" && !compact {
		meta = append(meta, tx.s["f.folder"]+": "+r.Folder)
	}
	if len(meta) > 0 {
		b.line("_" + strings.Join(meta, " · ") + "_")
		b.blank()
	}
	if !compact {
		if d := strings.TrimSpace(r.Description); d != "" {
			b.para(quote(d))
		}
		b.field(s["f.rationale"], oneLine(r.Rationale))
	}
	if len(r.Criteria) > 0 {
		b.line("- **" + s["f.criteria"] + ":**")
		for _, c := range r.Criteria {
			l := "  - " + oneLine(c.Description)
			if c.Method != "" && !compact {
				l += " (" + b.label("method", c.Method) + ")"
			}
			if t := strings.TrimSpace(c.Threshold); t != "" {
				l += " — " + s["f.threshold"] + ": " + t
			}
			b.line(l)
		}
	}
	if !compact {
		if len(r.Relations) > 0 {
			b.field(s["f.relations"], strings.Join(r.Relations, "; "))
		}
		if len(r.Risks) > 0 {
			var rs []string
			for _, k := range r.Risks {
				x := strings.TrimSpace(k.Key + " " + k.Title)
				var extra []string
				if k.Level != "" {
					extra = append(extra, tx.s["f.level"]+" "+or(tx.levels[k.Level], k.Level))
				}
				if m := oneLine(k.Mitigation); m != "" {
					extra = append(extra, tx.s["f.mitigation"]+": "+m)
				}
				if len(k.Attack) > 0 {
					extra = append(extra, "ATT&CK "+strings.Join(k.Attack, ", "))
				}
				if len(k.Emb3d) > 0 {
					extra = append(extra, "EMB3D "+strings.Join(k.Emb3d, ", "))
				}
				if len(extra) > 0 {
					x += " (" + strings.Join(extra, "; ") + ")"
				}
				rs = append(rs, x)
			}
			b.field(tx.s["f.risks"], strings.Join(rs, "; "))
		}
		if len(r.Tests) > 0 {
			b.field(s["f.verifiedBy"], strings.Join(r.Tests, "; "))
		}
		var std []string
		std = append(std, r.Standards...)
		if len(r.D3fend) > 0 {
			std = append(std, "D3FEND "+strings.Join(r.D3fend, ", "))
		}
		if len(r.Emb3d) > 0 {
			std = append(std, "EMB3D "+strings.Join(r.Emb3d, ", "))
		}
		if len(std) > 0 {
			b.field(tx.s["f.standards"], strings.Join(std, "; "))
		}
	}
	b.blank()
	return b.sb.String()
}

// obligations counts the requirements per obligation: "MUST 3, SHOULD 1".
func obligations(reqs []Requirement) string {
	order := []string{"must", "must_not", "should", "should_not", "may", ""}
	n := map[string]int{}
	for _, r := range reqs {
		n[r.Obligation]++
	}
	var out []string
	for _, o := range order {
		if n[o] == 0 {
			continue
		}
		name := strings.ToUpper(strings.ReplaceAll(o, "_", " "))
		if o == "" {
			name = "—"
		}
		out = append(out, fmt.Sprintf("%s %d", name, n[o]))
	}
	return strings.Join(out, ", ")
}

// dependencyOrder orders the keys so that prerequisites (DependsOn) come
// first, otherwise in the given order; requirements in a dependency cycle
// are appended and reported. Only requirements with a dependency among the
// given ones are ordered; nil when there are none.
func dependencyOrder(reqs []Requirement) ([]string, [][]string) {
	in := map[string]bool{}
	for _, r := range reqs {
		in[r.Key] = true
	}
	deps := map[string][]string{}
	involved := map[string]bool{}
	for _, r := range reqs {
		for _, d := range r.DependsOn {
			if in[d] && d != r.Key {
				deps[r.Key] = append(deps[r.Key], d)
				involved[r.Key], involved[d] = true, true
			}
		}
	}
	if len(involved) == 0 {
		return nil, nil
	}
	done := map[string]bool{}
	var order []string
	for progress := true; progress; {
		progress = false
		for _, r := range reqs {
			if !involved[r.Key] || done[r.Key] {
				continue
			}
			ready := true
			for _, d := range deps[r.Key] {
				if !done[d] {
					ready = false
				}
			}
			if ready {
				done[r.Key] = true
				order = append(order, r.Key)
				progress = true
			}
		}
	}
	var cycle []string
	for _, r := range reqs {
		if involved[r.Key] && !done[r.Key] {
			cycle = append(cycle, r.Key)
			order = append(order, r.Key)
		}
	}
	var cycles [][]string
	if len(cycle) > 0 {
		cycles = append(cycles, cycle)
	}
	return order, cycles
}

// Scope describes how the requirements of a prompt were chosen.
type Scope struct {
	// Selected: chosen one by one (their number); Query, Folder (path),
	// Release and Variant (names) as given.
	Selected                        int
	Query, Folder, Release, Variant string
}

// ScopeLabel phrases a scope in a language ("" for the whole project).
func ScopeLabel(lang string, sc Scope) string {
	tx := solTexts[lang]
	if tx == nil {
		tx = solTexts["en"]
	}
	var out []string
	if sc.Selected > 0 {
		out = append(out, fmt.Sprintf(tx.s["scope.selected"], sc.Selected))
	}
	for _, x := range []struct{ key, v string }{{"scope.query", sc.Query}, {"scope.folder", sc.Folder}, {"scope.release", sc.Release}, {"scope.variant", sc.Variant}} {
		if v := strings.TrimSpace(x.v); v != "" {
			out = append(out, tx.s[x.key]+" "+v)
		}
	}
	return strings.Join(out, " · ")
}
