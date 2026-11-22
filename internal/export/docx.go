package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	nsW = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsR = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

	xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
)

const docxContentTypes = xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Default Extension="png" ContentType="image/png"/>` +
	`<Default Extension="jpeg" ContentType="image/jpeg"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
	`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
	`</Types>`

const docxRootRels = xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
	`</Relationships>`

// docxDocRels are the relationships of a report's document part: the
// styles and the logo.
func docxDocRels(l *Logo) string {
	return xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		docxLogoRel(l) + `</Relationships>`
}

const docxStyles = xmlHeader + `<w:styles xmlns:w="` + nsW + `">` +
	`<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="Calibri" w:cs="Calibri"/><w:sz w:val="20"/><w:szCs w:val="20"/><w:lang w:val="{{LANG}}"/></w:rPr></w:rPrDefault>` +
	`<w:pPrDefault><w:pPr><w:spacing w:after="120" w:line="259" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>` +
	`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>` +
	`<w:pPr><w:spacing w:after="80"/></w:pPr><w:rPr><w:b/><w:color w:val="1F3864"/><w:sz w:val="48"/><w:szCs w:val="48"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Subtitle"><w:name w:val="Subtitle"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>` +
	`<w:rPr><w:color w:val="595959"/><w:sz w:val="26"/><w:szCs w:val="26"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>` +
	`<w:pPr><w:keepNext/><w:spacing w:before="360" w:after="120"/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:b/><w:color w:val="2F5496"/><w:sz w:val="32"/><w:szCs w:val="32"/></w:rPr></w:style>` +
	`<w:style w:type="table" w:default="1" w:styleId="TableNormal"><w:name w:val="Normal Table"/><w:tblPr><w:tblInd w:w="0" w:type="dxa"/>` +
	`<w:tblCellMar><w:top w:w="0" w:type="dxa"/><w:left w:w="108" w:type="dxa"/><w:bottom w:w="0" w:type="dxa"/><w:right w:w="108" w:type="dxa"/></w:tblCellMar></w:tblPr></w:style>` +
	`<w:style w:type="table" w:styleId="QTable"><w:name w:val="Qtestframe Table"/><w:basedOn w:val="TableNormal"/>` +
	`<w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:rPr><w:sz w:val="18"/><w:szCs w:val="18"/></w:rPr>` +
	`<w:tblPr><w:tblBorders>` +
	`<w:top w:val="single" w:sz="4" w:space="0" w:color="A6A6A6"/><w:left w:val="single" w:sz="4" w:space="0" w:color="A6A6A6"/>` +
	`<w:bottom w:val="single" w:sz="4" w:space="0" w:color="A6A6A6"/><w:right w:val="single" w:sz="4" w:space="0" w:color="A6A6A6"/>` +
	`<w:insideH w:val="single" w:sz="4" w:space="0" w:color="A6A6A6"/><w:insideV w:val="single" w:sz="4" w:space="0" w:color="A6A6A6"/>` +
	`</w:tblBorders><w:tblCellMar><w:top w:w="40" w:type="dxa"/><w:left w:w="80" w:type="dxa"/><w:bottom w:w="40" w:type="dxa"/><w:right w:w="80" w:type="dxa"/></w:tblCellMar></w:tblPr></w:style>` +
	`</w:styles>`

// esc returns s escaped for XML character data / attribute values, with
// invalid XML characters removed.
func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(cleanXML(s)))
	return b.String()
}

// docxRuns renders text as one or more runs, turning newlines into breaks
// and tabs into tab elements.
func docxRuns(b *strings.Builder, text string, bold bool) {
	b.WriteString("<w:r>")
	if bold {
		b.WriteString("<w:rPr><w:b/><w:bCs/></w:rPr>")
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			b.WriteString("<w:br/>")
		}
		for j, part := range strings.Split(line, "\t") {
			if j > 0 {
				b.WriteString("<w:tab/>")
			}
			if part != "" {
				b.WriteString(`<w:t xml:space="preserve">`)
				b.WriteString(esc(strings.ReplaceAll(part, "\r", "")))
				b.WriteString("</w:t>")
			}
		}
	}
	b.WriteString("</w:r>")
}

func docxPara(b *strings.Builder, style, text string) {
	b.WriteString("<w:p>")
	if style != "" {
		b.WriteString(`<w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`)
	}
	docxRuns(b, text, false)
	b.WriteString("</w:p>")
}

// landscape A4 in twips with 0.5" margins
const (
	docxPageW   = 16838
	docxPageH   = 11906
	docxMargin  = 720
	docxContent = docxPageW - 2*docxMargin
)

func docxTable(b *strings.Builder, s Section) {
	n := width(s)
	colW := docxContent / n
	b.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="QTable"/><w:tblW w:w="5000" w:type="pct"/><w:tblLayout w:type="autofit"/><w:tblLook w:val="04A0" w:firstRow="1" w:lastRow="0" w:firstColumn="0" w:lastColumn="0" w:noHBand="1" w:noVBand="1"/></w:tblPr><w:tblGrid>`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(b, `<w:gridCol w:w="%d"/>`, colW)
	}
	b.WriteString("</w:tblGrid>")
	row := func(cells []string, header bool) {
		b.WriteString("<w:tr>")
		if header {
			b.WriteString(`<w:trPr><w:tblHeader/><w:cantSplit/></w:trPr>`)
		}
		for i := 0; i < n; i++ {
			fmt.Fprintf(b, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>`, colW)
			if header {
				b.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="D9E2F3"/>`)
			}
			b.WriteString("</w:tcPr><w:p>")
			docxRuns(b, cell(cells, i), header)
			b.WriteString("</w:p></w:tc>")
		}
		b.WriteString("</w:tr>")
	}
	row(s.Columns, true)
	for _, r := range s.Rows {
		row(r, false)
	}
	b.WriteString("</w:tbl>")
}

func docxDocument(ds Dataset) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<w:document xmlns:w="` + nsW + `" xmlns:r="` + nsR + `" xmlns:wp="` + nsWP + `" xmlns:a="` + nsA + `" xmlns:pic="` + nsPic + `"><w:body>`)
	docxLogo(&b, ds.Logo, 1)
	docxPara(&b, "Title", ds.Title)
	if ds.Subtitle != "" {
		docxPara(&b, "Subtitle", ds.Subtitle)
	}
	docxPara(&b, "", ds.label("generated")+" "+generatedAt(ds).Format("2006-01-02 15:04:05 MST"))
	for i, s := range ds.Sections {
		docxPara(&b, "Heading1", ds.sectionTitle(s, i))
		if width(s) == 0 {
			docxPara(&b, "", ds.label("noData"))
			continue
		}
		docxTable(&b, s)
		if len(s.Rows) == 0 {
			docxPara(&b, "", ds.label("noRows"))
		} else {
			b.WriteString("<w:p/>") // a table must not be the last body element before sectPr
		}
	}
	fmt.Fprintf(&b, `<w:sectPr><w:pgSz w:w="%d" w:h="%d" w:orient="landscape"/><w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="360" w:footer="360" w:gutter="0"/></w:sectPr>`,
		docxPageW, docxPageH, docxMargin, docxMargin, docxMargin, docxMargin)
	b.WriteString("</w:body></w:document>")
	return b.String()
}

func docxCore(ds Dataset) string {
	ts := generatedAt(ds).UTC().Format(time.RFC3339)
	return xmlHeader + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" ` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" ` +
		`xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>` + esc(ds.Title) + `</dc:title>` +
		`<dc:subject>` + esc(ds.Subtitle) + `</dc:subject>` +
		`<dc:creator>Qtestframe</dc:creator>` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + ts + `</dcterms:created>` +
		`<dcterms:modified xsi:type="dcterms:W3CDTF">` + ts + `</dcterms:modified>` +
		`</cp:coreProperties>`
}

// WriteDOCX writes ds as a minimal Office Open XML word processing document.
func WriteDOCX(w io.Writer, ds Dataset) error {
	zw := zip.NewWriter(w)
	mod := generatedAt(ds)
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", docxContentTypes},
		{"_rels/.rels", docxRootRels},
		{"word/_rels/document.xml.rels", docxDocRels(ds.Logo)},
		{"word/document.xml", docxDocument(ds)},
		{"word/styles.xml", strings.Replace(docxStyles, "{{LANG}}", ds.label("docLang"), 1)},
		{"docProps/core.xml", docxCore(ds)},
	}
	for _, p := range parts {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: p.name, Method: zip.Deflate, Modified: mod})
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, p.body); err != nil {
			return err
		}
	}
	if ds.Logo.ok() {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: docxLogoPart(ds.Logo), Method: zip.Store, Modified: mod})
		if err != nil {
			return err
		}
		if _, err := f.Write(ds.Logo.Data); err != nil {
			return err
		}
	}
	return zw.Close()
}
