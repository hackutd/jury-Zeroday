package funcs

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"server/models"
	"sort"
	"strings"
	"unicode"
)

// sponsorPrefixes maps each sponsor to the prefixes its opt-in prize names start with, after
// normalizeSponsorName. Prefixes let both "Nvidia" (last year's export) and "Nvidia Track" match,
// and make both Spacetime prizes ("Spacetime Track", "Spacetime ScrabbleBot Prize") one sponsor.
// Opt-in prizes that match none of these (Best Design, MLH prizes, ...) are not sponsors.
var sponsorPrefixes = []struct {
	sponsor  string
	prefixes []string
}{
	{"Nvidia", []string{"nvidia"}},
	{"Capital One", []string{"capitalone"}},
	{"State Farm", []string{"statefarm"}},
	{"CBRE", []string{"cbre"}},
	{"Spacetime", []string{"spacetime"}},
	{"Photon", []string{"photon"}},
	{"T-Mobile", []string{"tmobile"}},
	{"8090", []string{"8090"}},
	{"PNC", []string{"pnc"}},
	{"Goldman Sachs", []string{"goldmansachs"}},
	{"EOG Resources", []string{"eog"}},
	{"Winstead PC", []string{"winstead"}},
	{"Toyota Financial Services", []string{"toyotafinancial"}},
	{"NMC^2", []string{"nmc", "northmark"}},
}

// nvidiaGroupLabels are the PDF/CSV group names, by number of other sponsors
var nvidiaGroupLabels = []string{"Nvidia only", "Nvidia + 1 sponsor", "Nvidia + 2 sponsors", "Nvidia + 3 or more sponsors"}

// normalizeSponsorName lowercases a prize name and drops everything but letters and digits,
// so "T-Mobile Track", "t mobile" and "TMobile" all become "tmobile..."
func normalizeSponsorName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SponsorOf returns the sponsor an opt-in prize belongs to (eg. "Nvidia Track" -> "Nvidia"), or ""
// if the prize isn't a sponsor prize
func SponsorOf(prize string) string {
	normalized := normalizeSponsorName(prize)
	for _, s := range sponsorPrefixes {
		for _, prefix := range s.prefixes {
			if strings.HasPrefix(normalized, prefix) {
				return s.sponsor
			}
		}
	}
	return ""
}

// nvidiaProject is a project that opted into Nvidia, with its other sponsor prizes
type nvidiaProject struct {
	project     *models.Project
	otherPrizes []string // Other sponsors' opt-in prizes, as named in Devpost
	group       int      // Index into nvidiaGroupLabels (number of other sponsors, capped at 3)
}

// groupNvidiaProjects finds the projects that opted into Nvidia and sorts them by group (number of
// other sponsors, where all of one sponsor's prizes count once), then by table number. It also
// returns the Nvidia prize name as written in Devpost, for the PDF title.
func groupNvidiaProjects(projects []*models.Project) ([]nvidiaProject, string) {
	title := "Nvidia"
	out := []nvidiaProject{}
	for _, project := range projects {
		isNvidia := false
		otherSponsors := map[string]bool{}
		otherPrizes := []string{}
		for _, prize := range project.ChallengeList {
			switch sponsor := SponsorOf(prize); sponsor {
			case "":
			case "Nvidia":
				if !isNvidia && len(out) == 0 {
					title = strings.TrimSpace(prize)
				}
				isNvidia = true
			default:
				otherSponsors[sponsor] = true
				otherPrizes = append(otherPrizes, strings.TrimSpace(prize))
			}
		}
		if isNvidia {
			out = append(out, nvidiaProject{project, otherPrizes, min(len(otherSponsors), len(nvidiaGroupLabels)-1)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].group != out[j].group {
			return out[i].group < out[j].group
		}
		return out[i].project.Location < out[j].project.Location
	})
	return out, title
}

// CreateNvidiaPDF creates a printable PDF of the projects that opted into Nvidia, in groups: Nvidia
// only, Nvidia + 1 other sponsor, Nvidia + 2 other sponsors (and Nvidia + 3 or more, only if any),
// each sorted by table number. Rows show the table number, project name and the other sponsor
// prizes. The Nvidia prize name and column headers are on every page. Contains no PII.
func CreateNvidiaPDF(projects []*models.Project) []byte {
	grouped, title := groupNvidiaProjects(projects)

	const groupSize = 16.0 // Group headings
	const alsoSize = 11.0  // "Also: ..." line under the project name
	contentWidth := pdfPageWidth - 2*pdfMargin
	nameX := pdfMargin + prizeTableColumn
	nameWidth := contentWidth - prizeTableColumn - 8
	titleLines := wrapText(title, prizeTitleSize, true, contentWidth)
	footerY := pdfMargin - prizeFooterSize
	bottom := pdfMargin + 12

	pages := []*pdfPage{}
	var page *pdfPage
	var y float64

	// Start a new page with the title and column headers, setting y to the first row's top
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

	// Draw a group heading on a shaded band
	heading := func(text string) {
		height := groupSize + 2*prizeRowPadding
		y -= 6
		page.rect(pdfMargin, y-height, contentWidth, height, 0.82)
		page.text(pdfMargin+6, y-prizeRowPadding-groupSize+3, groupSize, true, toWinAnsi(text))
		y -= height
	}

	// The three main groups always show (even when empty); the 3+ group only if it has projects
	groups := [][]nvidiaProject{{}, {}, {}, {}}
	for _, p := range grouped {
		groups[p.group] = append(groups[p.group], p)
	}

	newPage()
	for g, members := range groups {
		if g == len(groups)-1 && len(members) == 0 {
			continue
		}
		label := fmt.Sprintf("%s (%d)", nvidiaGroupLabels[g], len(members))

		// Keep a heading together with its first row
		if y-(groupSize+2*prizeRowPadding+6)-(prizeTableSize+2*prizeRowPadding) < bottom {
			newPage()
		}
		heading(label)

		if len(members) == 0 {
			page.text(nameX, y-prizeRowPadding-prizeNameSize+2, prizeNameSize, false, []byte("No projects."))
			y -= prizeNameSize + 2*prizeRowPadding
			continue
		}

		for i, p := range members {
			lines := wrapText(p.project.Name, prizeNameSize, false, nameWidth)
			alsoLines := [][]byte{}
			if len(p.otherPrizes) > 0 {
				alsoLines = wrapText("Also: "+strings.Join(p.otherPrizes, ", "), alsoSize, false, nameWidth)
			}
			textHeight := float64(len(lines))*(prizeNameSize+prizeLineGap) - prizeLineGap +
				float64(len(alsoLines))*(alsoSize+prizeLineGap)
			rowHeight := max(textHeight, prizeTableSize) + 2*prizeRowPadding

			// Continue the group on a new page if this row would run into the footer
			if y-rowHeight < bottom {
				newPage()
				heading(label + " (continued)")
			}

			// Shade every other row so rows are easy to follow across the page
			if i%2 == 1 {
				page.rect(pdfMargin, y-rowHeight, contentWidth, rowHeight, 0.93)
			}
			page.text(pdfMargin+6, y-prizeRowPadding-prizeTableSize+3, prizeTableSize, true, []byte(fmt.Sprintf("%d", p.project.Location)))
			lineY := y - prizeRowPadding - prizeNameSize + 2
			for i, line := range lines {
				if i > 0 {
					lineY -= prizeNameSize + prizeLineGap
				}
				page.text(nameX, lineY, prizeNameSize, false, line)
			}
			for _, line := range alsoLines {
				lineY -= alsoSize + prizeLineGap
				page.text(nameX, lineY, alsoSize, false, line)
			}
			y -= rowHeight
		}
	}

	// Footers, now that the page count is known
	for i, p := range pages {
		footer := []byte(fmt.Sprintf("Page %d of %d", i+1, len(pages)))
		p.text(pdfPageWidth-pdfMargin-textWidth(footer, prizeFooterSize, false), footerY, prizeFooterSize, false, footer)
		p.text(pdfMargin, footerY, prizeFooterSize, false, toWinAnsi(fmt.Sprintf("%d projects", len(grouped))))
	}

	return buildPDF(title, pages)
}

// CreateNvidiaCSV creates a CSV of the projects that opted into Nvidia, in the same groups and order
// as CreateNvidiaPDF, with each team member's name and email. Contains PII: admin only.
func CreateNvidiaCSV(projects []*models.Project) []byte {
	grouped, _ := groupNvidiaProjects(projects)
	nvidiaProjects := make([]*models.Project, len(grouped))
	for i, p := range grouped {
		nvidiaProjects[i] = p.project
	}
	maxMembers := largestTeamSize(nvidiaProjects)

	csvBuffer := &bytes.Buffer{}
	w := csv.NewWriter(csvBuffer)
	header := []string{"Group", "Table", "Name", "Devpost", "Try It Out", "Other Sponsors"}
	w.Write(append(header, teamMemberHeader(maxMembers)...))
	for _, p := range grouped {
		row := []string{
			nvidiaGroupLabels[p.group],
			fmt.Sprintf("%d", p.project.Location),
			p.project.Name,
			p.project.Url,
			p.project.TryLink,
			strings.Join(p.otherPrizes, ", "),
		}
		w.Write(append(row, teamMemberCells(p.project, maxMembers)...))
	}
	w.Flush()
	return csvBuffer.Bytes()
}

// CreateNvidiaExportZip creates the Nvidia export: nvidia.pdf (see CreateNvidiaPDF) and nvidia.csv
// (see CreateNvidiaCSV, contains PII)
func CreateNvidiaExportZip(projects []*models.Project) ([]byte, error) {
	zipBuffer := &bytes.Buffer{}
	w := zip.NewWriter(zipBuffer)
	files := []struct {
		name string
		data []byte
	}{
		{"nvidia.pdf", CreateNvidiaPDF(projects)},
		{"nvidia.csv", CreateNvidiaCSV(projects)},
	}
	for _, file := range files {
		f, err := w.Create(file.name)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(file.data); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return zipBuffer.Bytes(), nil
}
