package reqimport

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqlang"
)

// Word documents: headings become folders, paragraphs with an obligation
// keyword become requirements, tables with a header row are read like CSV.

type docPara struct {
	text     strings.Builder
	style    string
	numbered bool
	outline  int // -1 = none
	index    int // 1-based paragraph number (outside tables)
}

type docCell struct {
	paras  []string
	span   int
	vmerge string // "", "restart" or "continue"
}

type docTable struct {
	rows  [][]*docCell
	index int
}

type docBlock struct {
	para  *docPara
	table *docTable
}

func attrVal(t xml.StartElement, name string) (string, bool) {
	for _, a := range t.Attr {
		if a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

// docxBlocks reads the paragraphs and top-level tables of document.xml.
func docxBlocks(data []byte) ([]docBlock, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	var (
		blocks  []docBlock
		stack   []string
		paras   []*docPara
		tables  []*docTable
		skip    int // depth inside ignored subtrees
		inText  bool
		nPara   int
		nTable  int
		parent  = func() string { return stackAt(stack, 2) }
		topCell = func() *docCell {
			if len(tables) == 0 {
				return nil
			}
			t := tables[len(tables)-1]
			if len(t.rows) == 0 || len(t.rows[len(t.rows)-1]) == 0 {
				return nil
			}
			row := t.rows[len(t.rows)-1]
			return row[len(row)-1]
		}
	)
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, invalidf("word/document.xml: %v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			stack = append(stack, name)
			if skip > 0 || name == "Fallback" || name == "delText" || name == "instrText" {
				skip++
				continue
			}
			var p *docPara
			if len(paras) > 0 {
				p = paras[len(paras)-1]
			}
			switch name {
			case "p":
				paras = append(paras, &docPara{outline: -1})
			case "pStyle":
				if p != nil && parent() == "pPr" {
					p.style, _ = attrVal(t, "val")
				}
			case "numPr":
				if p != nil {
					p.numbered = true
				}
			case "outlineLvl":
				if v, ok := attrVal(t, "val"); ok && p != nil {
					if n, err := strconv.Atoi(v); err == nil {
						p.outline = n
					}
				}
			case "t":
				inText = p != nil
			case "tab":
				if p != nil && parent() == "r" {
					p.text.WriteString(" ")
				}
			case "br", "cr":
				if p != nil && parent() == "r" {
					p.text.WriteString("\n")
				}
			case "tbl":
				tables = append(tables, &docTable{})
			case "tr":
				if len(tables) > 0 {
					tb := tables[len(tables)-1]
					tb.rows = append(tb.rows, nil)
				}
			case "tc":
				if len(tables) > 0 {
					tb := tables[len(tables)-1]
					if len(tb.rows) == 0 {
						tb.rows = append(tb.rows, nil)
					}
					tb.rows[len(tb.rows)-1] = append(tb.rows[len(tb.rows)-1], &docCell{span: 1})
				}
			case "gridSpan":
				if c := topCell(); c != nil {
					if v, ok := attrVal(t, "val"); ok {
						if n, err := strconv.Atoi(v); err == nil && n > 1 && n < 64 {
							c.span = n
						}
					}
				}
			case "vMerge":
				if c := topCell(); c != nil {
					v, _ := attrVal(t, "val")
					if v == "" {
						v = "continue"
					}
					c.vmerge = v
				}
			}
		case xml.EndElement:
			name := t.Name.Local
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if skip > 0 {
				skip--
				continue
			}
			switch name {
			case "t":
				inText = false
			case "p":
				if len(paras) == 0 {
					continue
				}
				p := paras[len(paras)-1]
				paras = paras[:len(paras)-1]
				if c := topCell(); c != nil {
					if s := tidyLines(p.text.String()); s != "" {
						c.paras = append(c.paras, s)
					}
					continue
				}
				nPara++
				p.index = nPara
				blocks = append(blocks, docBlock{para: p})
			case "tbl":
				if len(tables) == 0 {
					continue
				}
				tb := tables[len(tables)-1]
				tables = tables[:len(tables)-1]
				if c := topCell(); c != nil {
					// nested table: its text belongs to the outer cell
					for _, row := range tb.rows {
						for _, cell := range row {
							c.paras = append(c.paras, cell.paras...)
						}
					}
					continue
				}
				nTable++
				tb.index = nTable
				blocks = append(blocks, docBlock{table: tb})
			}
		case xml.CharData:
			if inText && skip == 0 && len(paras) > 0 {
				paras[len(paras)-1].text.Write(t)
			}
		}
	}
	return blocks, nil
}

func stackAt(stack []string, fromTop int) string {
	if len(stack) < fromTop {
		return ""
	}
	return stack[len(stack)-fromTop]
}

// ---------------------------------------------------------------- styles

type wStyles struct {
	Styles []wStyle `xml:"style"`
}

type wStyle struct {
	ID      string `xml:"styleId,attr"`
	Name    wVal   `xml:"name"`
	BasedOn wVal   `xml:"basedOn"`
	Outline *wVal  `xml:"pPr>outlineLvl"`
}

type wVal struct {
	Val string `xml:"val,attr"`
}

var (
	headingNameRe = regexp.MustCompile(`(?i)^(?:heading|rubrik|überschrift|titre|kop)\s*([1-9])$`)
	headingIDRe   = regexp.MustCompile(`(?i)^(?:heading|rubrik|berschrift|titre|kop)([1-9])$`)
)

// headingLevels maps style IDs to heading levels (1-9).
func headingLevels(stylesXML []byte) map[string]int {
	out := map[string]int{}
	var st wStyles
	if len(stylesXML) == 0 || xml.Unmarshal(stylesXML, &st) != nil {
		return out
	}
	byID := map[string]wStyle{}
	for _, s := range st.Styles {
		byID[s.ID] = s
	}
	var level func(id string, depth int) int
	level = func(id string, depth int) int {
		s, ok := byID[id]
		if !ok || depth > 10 {
			return 0
		}
		if m := headingNameRe.FindStringSubmatch(strings.TrimSpace(s.Name.Val)); m != nil {
			return int(m[1][0] - '0')
		}
		if s.Outline != nil {
			if n, err := strconv.Atoi(s.Outline.Val); err == nil && n >= 0 && n < 9 {
				return n + 1
			}
		}
		if s.BasedOn.Val != "" {
			return level(s.BasedOn.Val, depth+1)
		}
		return 0
	}
	for id := range byID {
		if n := level(id, 0); n > 0 {
			out[id] = n
		}
	}
	return out
}

func (p *docPara) headingLevel(styles map[string]int) int {
	if p.outline >= 0 && p.outline < 9 {
		return p.outline + 1
	}
	if n, ok := styles[p.style]; ok {
		return n
	}
	if m := headingIDRe.FindStringSubmatch(p.style); m != nil {
		return int(m[1][0] - '0')
	}
	return 0
}

// ---------------------------------------------------------------- obligations

// normative obligation levels make a paragraph a requirement.
var normative = map[string]bool{model.ObligationMust: true, model.ObligationMustNot: true, model.ObligationShall: true,
	model.ObligationShallNot: true, model.ObligationShould: true, model.ObligationShouldNot: true,
	model.ObligationMay: true, model.ObligationOptional: true}

// normativeObligation returns the strongest normative keyword of text. In
// English text only uppercase keywords count when upperOnly is set (the
// document writes its keywords in uppercase, as RFC 8174 asks).
func normativeObligation(text string, upperOnly bool) (level string, upper bool) {
	a := reqlang.AnalyzeLang(text, "")
	best := -1
	for _, k := range a.Keywords {
		if !normative[k.Level] {
			continue
		}
		word := text[k.Offset : k.Offset+k.Length]
		isUp := word == strings.ToUpper(word)
		if isUp {
			upper = true
		}
		if upperOnly && a.Language == "en" && !isUp {
			continue
		}
		r := rankOf(k.Level)
		if best < 0 || r < best {
			best, level = r, k.Level
		}
	}
	return level, upper
}

func rankOf(level string) int {
	for i, l := range reqlang.Levels {
		if l == level {
			return i
		}
	}
	return len(reqlang.Levels)
}

// ---------------------------------------------------------------- parse

func parseDOCX(data []byte, opt Options, res *Result) error {
	if !isZip(data) {
		return invalidf("not a valid .docx file (old .doc files are not supported; save as .docx)")
	}
	zr, err := openZip(data, ".docx")
	if err != nil {
		return err
	}
	df := zipFile(zr, "word/document.xml")
	if df == nil {
		return invalidf("not a Word document: word/document.xml is missing")
	}
	doc, err := readZipEntry(df)
	if err != nil {
		return err
	}
	var stylesXML []byte
	if sf := zipFile(zr, "word/styles.xml"); sf != nil {
		stylesXML, _ = readZipEntry(sf)
	}
	styles := headingLevels(stylesXML)
	blocks, err := docxBlocks(doc)
	if err != nil {
		return err
	}
	// RFC 8174: if the document writes keywords in uppercase anywhere, only
	// uppercase English keywords are normative.
	upperOnly := false
	for _, b := range blocks {
		if b.para != nil && b.para.headingLevel(styles) == 0 {
			if _, up := normativeObligation(tidyLines(b.para.text.String()), false); up {
				upperOnly = true
				break
			}
		}
	}
	var path []string
	appendTo := -1
	addPara := func(text string, row int, loc string) bool {
		level, _ := normativeObligation(text, upperOnly)
		if level == "" {
			return false
		}
		id, rest := splitID(text)
		c := Candidate{Description: rest, ExternalID: id, Folder: strings.Join(path, "/"), Row: row, Location: loc,
			Obligation: level, ObligationFrom: "text"}
		if !finish(&c, raw{}) {
			return false
		}
		c.Warnings = dropWarning(c.Warnings, "title derived from the description")
		res.Candidates = append(res.Candidates, c)
		return true
	}
	for _, b := range blocks {
		if p := b.para; p != nil {
			text := tidyLines(p.text.String())
			if text == "" {
				continue
			}
			if lvl := p.headingLevel(styles); lvl > 0 {
				if lvl-1 < len(path) {
					path = path[:lvl-1]
				}
				path = append(path, strings.ReplaceAll(strings.ReplaceAll(text, "\n", " "), "/", "-"))
				appendTo = -1
				continue
			}
			loc := fmt.Sprintf("paragraph %d", p.index)
			if addPara(text, p.index, loc) {
				appendTo = -1
				if strings.HasSuffix(text, ":") {
					appendTo = len(res.Candidates) - 1
				}
				continue
			}
			if p.numbered && appendTo >= 0 {
				res.Candidates[appendTo].Description += "\n- " + text
				continue
			}
			appendTo = -1
			continue
		}
		appendTo = -1
		tb := b.table
		grid := tableGrid(tb)
		prefix := fmt.Sprintf("table %d, ", tb.index)
		out, err := parseRows(grid, opt.Mapping, nil, prefix)
		if err != nil {
			// No header row: scan the cells like paragraphs.
			for ri, row := range tb.rows {
				for _, cell := range row {
					for _, para := range cell.paras {
						addPara(para, ri+1, fmt.Sprintf("%srow %d", prefix, ri+1))
					}
				}
			}
			continue
		}
		for _, col := range out.columns {
			if !containsColumn(res.Columns, col.Header) {
				col.Index = len(res.Columns)
				res.Columns = append(res.Columns, col)
			}
		}
		for _, c := range out.candidates {
			c.Folder = joinFolder(strings.Join(path, "/"), c.Folder)
			res.Candidates = append(res.Candidates, c)
		}
		res.Errors = append(res.Errors, out.errors...)
	}
	return nil
}

func dropWarning(ws []string, w string) []string {
	out := ws[:0]
	for _, x := range ws {
		if x != w {
			out = append(out, x)
		}
	}
	return out
}

// tableGrid flattens a Word table into rows of cell texts: horizontally
// merged cells leave the spanned columns empty, vertically merged cells
// repeat the value of the cell above.
func tableGrid(tb *docTable) [][]string {
	var grid [][]string
	for _, row := range tb.rows {
		var cells []string
		for _, c := range row {
			col := len(cells)
			text := strings.Join(c.paras, "\n")
			if c.vmerge == "continue" && len(grid) > 0 && col < len(grid[len(grid)-1]) {
				text = grid[len(grid)-1][col]
			}
			cells = append(cells, text)
			for i := 1; i < c.span; i++ {
				cells = append(cells, "")
			}
		}
		grid = append(grid, cells)
	}
	return grid
}
