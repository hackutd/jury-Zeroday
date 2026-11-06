package funcs

import (
	"archive/zip"
	"bytes"
	"fmt"
	"server/models"
	"sort"
	"strings"
	"unicode"
)

// This file builds small text-only PDFs by hand, so the prod image doesn't need a PDF library.
// It only uses the built-in Helvetica fonts, which every PDF reader has. Those fonts only cover
// the WinAnsi (Western European) characters: other letters are printed as "?" and emoji and other
// symbols are left out.

// US Letter page, in points (1/72 inch)
const (
	pdfPageWidth  = 612.0
	pdfPageHeight = 792.0
	pdfMargin     = 54.0
)

// Prize PDF layout, in points
const (
	prizeTitleSize   = 24.0 // Prize name at the top of every page
	prizeHeaderSize  = 13.0 // "TABLE" / "PROJECT" column headers
	prizeTableSize   = 18.0 // Table number in each row
	prizeNameSize    = 15.0 // Project name in each row
	prizeFooterSize  = 10.0 // Page numbers
	prizeTableColumn = 90.0 // Width of the table number column
	prizeRowPadding  = 9.0  // Space above and below the text in each row
	prizeLineGap     = 4.0  // Extra space between wrapped lines of a project name
)

// Glyph widths (per 1000 units of font size) for ASCII 32-126 in Helvetica and Helvetica-Bold,
// from the standard Adobe font metrics. Used to wrap long names; other characters use 556.
var helveticaWidths = []int{
	278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278, // space - /
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 278, 278, 584, 584, 584, 556, // 0 - ?
	1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778, // @ - O
	667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 278, 278, 278, 469, 556, // P - _
	333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556, // ` - o
	556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584, // p - ~
}
var helveticaBoldWidths = []int{
	278, 333, 474, 556, 556, 889, 722, 238, 333, 333, 389, 584, 278, 333, 278, 278, // space - /
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 333, 333, 584, 584, 584, 611, // 0 - ?
	975, 722, 722, 722, 722, 667, 611, 778, 722, 278, 556, 722, 611, 833, 722, 778, // @ - O
	667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 333, 278, 333, 584, 556, // P - _
	333, 556, 611, 556, 611, 556, 333, 611, 611, 278, 278, 556, 278, 889, 611, 611, // ` - o
	611, 611, 389, 556, 333, 611, 556, 778, 556, 556, 500, 389, 280, 389, 584, // p - ~
}

// cp1252Extras maps the non-Latin-1 characters WinAnsi supports (curly quotes, dashes, ...)
// to their byte in the 0x80-0x9F range
var cp1252Extras = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87, 'ˆ': 0x88,
	'‰': 0x89, 'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C, 'Ž': 0x8E, '‘': 0x91, '’': 0x92, '“': 0x93,
	'”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97, '˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B,
	'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}

// toWinAnsi converts a string to WinAnsi bytes for the built-in fonts. Tabs and newlines become
// spaces. Letters and digits the fonts can't show become "?" (so it's clear something is missing),
// and anything else they can't show (emoji, symbols, joiners) is dropped.
func toWinAnsi(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			out = append(out, ' ')
		case r >= 32 && r <= 126, r >= 160 && r <= 255:
			out = append(out, byte(r))
		default:
			if b, ok := cp1252Extras[r]; ok {
				out = append(out, b)
			} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
				out = append(out, '?')
			}
		}
	}
	return out
}

// textWidth returns the width of WinAnsi text in points at the given font size
func textWidth(text []byte, size float64, bold bool) float64 {
	widths := helveticaWidths
	if bold {
		widths = helveticaBoldWidths
	}
	total := 0
	for _, b := range text {
		if b >= 32 && b <= 126 {
			total += widths[b-32]
		} else {
			total += 556
		}
	}
	return float64(total) * size / 1000
}

// wrapText splits text into lines that fit within maxWidth points, breaking at spaces. A single
// word that is too long on its own is broken by character. Always returns at least one line.
func wrapText(text string, size float64, bold bool, maxWidth float64) [][]byte {
	words := strings.Fields(string(toWinAnsi(text)))
	lines := [][]byte{}
	current := []byte{}
	for _, word := range words {
		w := []byte(word)

		// Break up words that don't fit on a line by themselves
		for textWidth(w, size, bold) > maxWidth {
			if len(current) > 0 {
				lines = append(lines, current)
				current = []byte{}
			}
			cut := len(w) - 1
			for cut > 1 && textWidth(w[:cut], size, bold) > maxWidth {
				cut--
			}
			lines = append(lines, w[:cut])
			w = w[cut:]
		}

		// Add the word to the current line, or start a new line
		candidate := w
		if len(current) > 0 {
			candidate = append(append(append([]byte{}, current...), ' '), w...)
		}
		if len(current) > 0 && textWidth(candidate, size, bold) > maxWidth {
			lines = append(lines, current)
			current = w
		} else {
			current = candidate
		}
	}
	if len(current) > 0 || len(lines) == 0 {
		lines = append(lines, current)
	}
	return lines
}

// pdfString escapes WinAnsi text as a PDF literal string
func pdfString(text []byte) string {
	var b strings.Builder
	b.WriteByte('(')
	for _, c := range text {
		if c == '(' || c == ')' || c == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	b.WriteByte(')')
	return b.String()
}

// pdfPage builds the content stream (drawing commands) of one page
type pdfPage struct {
	buf bytes.Buffer
}

// text draws one line of WinAnsi text with its baseline at (x, y). Font F1 is Helvetica, F2 is bold.
func (p *pdfPage) text(x, y, size float64, bold bool, text []byte) {
	font := "F1"
	if bold {
		font = "F2"
	}
	fmt.Fprintf(&p.buf, "BT /%s %.2f Tf %.2f %.2f Td %s Tj ET\n", font, size, x, y, pdfString(text))
}

// rect fills a rectangle with a gray level (0 = black, 1 = white), then goes back to black
func (p *pdfPage) rect(x, y, w, h, gray float64) {
	fmt.Fprintf(&p.buf, "%.3f g %.2f %.2f %.2f %.2f re f 0 g\n", gray, x, y, w, h)
}

// line draws a horizontal line from x1 to x2 at y
func (p *pdfPage) line(x1, x2, y, width float64) {
	fmt.Fprintf(&p.buf, "%.2f w %.2f %.2f m %.2f %.2f l S\n", width, x1, y, x2, y)
}

// buildPDF puts the pages' content streams together into a PDF file with the two Helvetica fonts
func buildPDF(title string, pages []*pdfPage) []byte {
	var out bytes.Buffer
	offsets := []int{}
	startObj := func() int {
		offsets = append(offsets, out.Len())
		return len(offsets)
	}

	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	// 1: catalog, 2: page tree, 3-4: fonts, 5: document info, then a page + content stream per page
	pageIds := []string{}
	for i := range pages {
		pageIds = append(pageIds, fmt.Sprintf("%d 0 R", 6+2*i))
	}
	fmt.Fprintf(&out, "%d 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n", startObj())
	fmt.Fprintf(&out, "%d 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", startObj(), strings.Join(pageIds, " "), len(pages))
	fmt.Fprintf(&out, "%d 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>\nendobj\n", startObj())
	fmt.Fprintf(&out, "%d 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>\nendobj\n", startObj())
	fmt.Fprintf(&out, "%d 0 obj\n<< /Title %s /Producer (Jury) >>\nendobj\n", startObj(), pdfString(toWinAnsi(title)))
	for _, page := range pages {
		pageObj := startObj()
		fmt.Fprintf(&out, "%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>\nendobj\n",
			pageObj, pdfPageWidth, pdfPageHeight, pageObj+1)
		fmt.Fprintf(&out, "%d 0 obj\n<< /Length %d >>\nstream\n", startObj(), page.buf.Len())
		out.Write(page.buf.Bytes())
		out.WriteString("\nendstream\nendobj\n")
	}

	// Cross-reference table: the byte offset of every object (each entry is exactly 20 bytes)
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R /Info 5 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return out.Bytes()
}

// CreatePrizePDF creates a printable PDF for one prize (opt-in challenge), meant to be handed to
// that prize's sponsor: the prize name at the top of every page, then each project's table number
// and name in large text, sorted by table number. Contains no PII.
//  1. Wrap each project name to fit the name column
//  2. Fill pages with rows until the next row would run into the footer, repeating the
//     prize name and column headers at the top of each new page
//  3. Draw the "Page X of N" footers once the number of pages is known
func CreatePrizePDF(prize string, projects []*models.Project) []byte {
	// Sort a copy of the projects by table number
	sorted := make([]*models.Project, len(projects))
	copy(sorted, projects)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Location < sorted[j].Location })

	contentWidth := pdfPageWidth - 2*pdfMargin
	nameX := pdfMargin + prizeTableColumn
	nameWidth := contentWidth - prizeTableColumn - 8
	titleLines := wrapText(prize, prizeTitleSize, true, contentWidth)
	footerY := pdfMargin - prizeFooterSize

	pages := []*pdfPage{}
	var page *pdfPage
	var y float64

	// Start a new page with the prize name and the column headers, setting y to the first row's top
	newPage := func() {
		page = &pdfPage{}
		pages = append(pages, page)
		y = pdfPageHeight - pdfMargin
		for _, line := range titleLines {
			y -= prizeTitleSize
			page.text(pdfMargin, y, prizeTitleSize, true, line)
			y -= prizeLineGap
		}
		y -= 22
		page.text(pdfMargin, y, prizeHeaderSize, true, []byte("TABLE"))
		page.text(nameX, y, prizeHeaderSize, true, []byte("PROJECT"))
		y -= 8
		page.line(pdfMargin, pdfPageWidth-pdfMargin, y, 1.5)
	}
	newPage()

	if len(sorted) == 0 {
		page.text(pdfMargin, y-prizeNameSize-prizeRowPadding, prizeNameSize, false, []byte("No projects opted into this prize."))
	}

	for i, project := range sorted {
		lines := wrapText(project.Name, prizeNameSize, false, nameWidth)
		textHeight := float64(len(lines))*(prizeNameSize+prizeLineGap) - prizeLineGap
		rowHeight := max(textHeight, prizeTableSize) + 2*prizeRowPadding

		// Move to a new page if this row would run into the footer (unless the page is still empty)
		if y-rowHeight < pdfMargin+12 && i > 0 {
			newPage()
		}

		// Shade every other row so rows are easy to follow across the page
		if i%2 == 1 {
			page.rect(pdfMargin, y-rowHeight, contentWidth, rowHeight, 0.93)
		}
		page.text(pdfMargin+6, y-prizeRowPadding-prizeTableSize+3, prizeTableSize, true, []byte(fmt.Sprintf("%d", project.Location)))
		lineY := y - prizeRowPadding - prizeNameSize + 2
		for _, line := range lines {
			page.text(nameX, lineY, prizeNameSize, false, line)
			lineY -= prizeNameSize + prizeLineGap
		}
		y -= rowHeight
	}

	// Footers, now that the page count is known
	for i, p := range pages {
		footer := []byte(fmt.Sprintf("Page %d of %d", i+1, len(pages)))
		p.text(pdfPageWidth-pdfMargin-textWidth(footer, prizeFooterSize, false), footerY, prizeFooterSize, false, footer)
		p.text(pdfMargin, footerY, prizeFooterSize, false, toWinAnsi(fmt.Sprintf("%d projects", len(sorted))))
	}

	return buildPDF(prize, pages)
}

// zipSafeName turns a prize name into a file name that works on every OS: path separators and
// other reserved characters become "-". Names already used get " (2)", " (3)", ... added.
func zipSafeName(name string, used map[string]bool) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	clean = strings.Trim(clean, ". ")
	if clean == "" {
		clean = "prize"
	}
	unique := clean
	for i := 2; used[strings.ToLower(unique)]; i++ {
		unique = fmt.Sprintf("%s (%d)", clean, i)
	}
	used[strings.ToLower(unique)] = true
	return unique
}

// CreatePrizeExportZip creates the zip for sponsors from every opt-in prize (challenge) in the
// projects, with two folders:
//   - csv-with-emails/<prize>.csv: the Export by Challenges CSV with team member names and emails (PII)
//   - pdf/<prize>.pdf: a printable list of table numbers and project names (see CreatePrizePDF)
func CreatePrizeExportZip(projects []*models.Project) ([]byte, error) {
	// Get list of challenges, in the order they first appear
	challengeList := []string{}
	for _, project := range projects {
		for _, challenge := range project.ChallengeList {
			if !contains(challengeList, challenge) {
				challengeList = append(challengeList, challenge)
			}
		}
	}

	zipBuffer := &bytes.Buffer{}
	w := zip.NewWriter(zipBuffer)
	used := map[string]bool{}
	for _, challenge := range challengeList {
		challengeProjects := []*models.Project{}
		for _, project := range projects {
			if contains(project.ChallengeList, challenge) {
				challengeProjects = append(challengeProjects, project)
			}
		}

		name := zipSafeName(challenge, used)
		files := map[string][]byte{
			"csv-with-emails/" + name + ".csv": CreateChallengeProjectCSV(challengeProjects, true),
			"pdf/" + name + ".pdf":             CreatePrizePDF(challenge, challengeProjects),
		}
		for _, path := range []string{"csv-with-emails/" + name + ".csv", "pdf/" + name + ".pdf"} {
			f, err := w.Create(path)
			if err != nil {
				return nil, err
			}
			if _, err := f.Write(files[path]); err != nil {
				return nil, err
			}
		}
	}

	if err := w.Close(); err != nil {
		return nil, err
	}
	return zipBuffer.Bytes(), nil
}
