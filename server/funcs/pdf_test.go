package funcs

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"server/models"
	"strconv"
	"strings"
	"testing"
)

// checkPDFStructure checks that the PDF starts and ends correctly and that every cross-reference
// entry points at the start of its object, so readers can open it without repairing it
func checkPDFStructure(t *testing.T, pdf []byte) {
	t.Helper()
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) || !bytes.HasSuffix(pdf, []byte("%%EOF\n")) {
		t.Fatalf("PDF should start with %%PDF-1.4 and end with %%%%EOF")
	}
	m := regexp.MustCompile(`startxref\n(\d+)\n`).FindSubmatch(pdf)
	if m == nil {
		t.Fatalf("PDF has no startxref")
	}
	xref, _ := strconv.Atoi(string(m[1]))
	if !bytes.HasPrefix(pdf[xref:], []byte("xref\n")) {
		t.Fatalf("startxref does not point at the xref table")
	}
	entries := regexp.MustCompile(`(\d{10}) 00000 n `).FindAllSubmatch(pdf[xref:], -1)
	for i, e := range entries {
		offset, _ := strconv.Atoi(string(e[1]))
		want := fmt.Sprintf("%d 0 obj", i+1)
		if !bytes.HasPrefix(pdf[offset:], []byte(want)) {
			t.Fatalf("xref entry %d points at %q, want %q", i+1, pdf[offset:offset+10], want)
		}
	}
}

func TestCreatePrizePDF(t *testing.T) {
	projects := []*models.Project{}
	for i := 60; i >= 1; i-- {
		projects = append(projects, models.NewProject(fmt.Sprintf("Project %02d", i), int64(i), 0, "", "", "", "", []string{"Best Design"}))
	}
	// Long name (wraps), special PDF characters, accents and an emoji
	projects = append(projects, models.NewProject("A really long project name that definitely needs to wrap onto more than one line (v2) \\ café 🚀", 61, 0, "", "", "", "", []string{"Best Design"}))

	pdf := CreatePrizePDF("Best Design", projects)
	checkPDFStructure(t, pdf)

	// 61 rows don't fit on one page, and the prize name is on every page
	pages := bytes.Count(pdf, []byte("/Type /Page /Parent"))
	if pages < 2 {
		t.Fatalf("61 projects should take more than one page, got %d", pages)
	}
	if got := bytes.Count(pdf, []byte("(Best Design) Tj")); got != pages {
		t.Fatalf("prize name should be on all %d pages, found %d times", pages, got)
	}
	if !bytes.Contains(pdf, []byte(fmt.Sprintf("(Page %d of %d) Tj", pages, pages))) {
		t.Fatalf("last page should say Page %d of %d", pages, pages)
	}

	// Sorted by table number: table 1 is drawn before table 60
	if bytes.Index(pdf, []byte("(Project 01)")) > bytes.Index(pdf, []byte("(Project 60)")) {
		t.Fatalf("projects should be sorted by table number")
	}

	// Parentheses and backslashes are escaped, é is WinAnsi 0xE9 and the emoji is dropped
	if !bytes.Contains(pdf, []byte(`\(v2\) \\ caf`+"\xe9)")) {
		t.Fatalf("special characters were not encoded as expected")
	}
	if got := string(toWinAnsi("Café 🚀 日本")); got != "Caf\xe9  ??" {
		t.Fatalf("toWinAnsi: got %q", got)
	}

	// Set JURY_PDF_OUT to a path to look at the PDF
	if out := os.Getenv("JURY_PDF_OUT"); out != "" {
		os.WriteFile(out, pdf, 0o644)
	}
}

func TestCreatePrizePDFEmpty(t *testing.T) {
	pdf := CreatePrizePDF("Nobody", []*models.Project{})
	checkPDFStructure(t, pdf)
	if !bytes.Contains(pdf, []byte("No projects opted into this prize.")) {
		t.Fatalf("empty prize should say so")
	}
}

func TestWrapText(t *testing.T) {
	lines := wrapText("one two three four", 12, false, textWidth([]byte("one two"), 12, false))
	got := []string{}
	for _, l := range lines {
		got = append(got, string(l))
	}
	if strings.Join(got, "|") != "one two|three|four" {
		t.Fatalf("unexpected wrap: %v", got)
	}

	// A single word wider than the line is broken up instead of overflowing
	for _, l := range wrapText(strings.Repeat("W", 50), 12, false, 100) {
		if textWidth(l, 12, false) > 100 {
			t.Fatalf("line %q is wider than 100pt", l)
		}
	}
}

func TestZipSafeName(t *testing.T) {
	used := map[string]bool{}
	cases := []struct{ in, want string }{
		{"Best Design", "Best Design"},
		{"AI/ML: Best Hack?", "AI-ML- Best Hack-"},
		{"best design", "best design (2)"},
		{"  ", "prize"},
		{"..", "prize (2)"},
	}
	for _, c := range cases {
		if got := zipSafeName(c.in, used); got != c.want {
			t.Fatalf("zipSafeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCreatePrizeExportZip(t *testing.T) {
	p1 := models.NewProject("Design App", 2, 0, "", "https://devpost.com/a", "", "", []string{"Best Design", "AI/ML"})
	p1.TeamMembers = []models.TeamMember{{Name: "Ann A", Email: "ann@example.com"}}
	p2 := models.NewProject("Other App", 1, 0, "", "https://devpost.com/b", "", "", []string{"Best Design"})

	data, err := CreatePrizeExportZip([]*models.Project{p1, p2})
	if err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range r.File {
		rc, _ := f.Open()
		files[f.Name], _ = io.ReadAll(rc)
		rc.Close()
	}

	want := []string{"csv-with-emails/Best Design.csv", "pdf/Best Design.pdf", "csv-with-emails/AI-ML.csv", "pdf/AI-ML.pdf"}
	if len(files) != len(want) {
		t.Fatalf("expected %v, got %d files", want, len(files))
	}
	for _, name := range want {
		if _, ok := files[name]; !ok {
			t.Fatalf("missing %s", name)
		}
	}

	// CSVs have the team member columns, PDFs don't have any emails
	if !bytes.Contains(files["csv-with-emails/Best Design.csv"], []byte("ann@example.com")) {
		t.Fatalf("prize CSV should include team member emails")
	}
	if bytes.Contains(files["pdf/Best Design.pdf"], []byte("@example.com")) {
		t.Fatalf("prize PDF should not include emails")
	}
	checkPDFStructure(t, files["pdf/AI-ML.pdf"])
	if !bytes.Contains(files["pdf/AI-ML.pdf"], []byte("(AI/ML) Tj")) {
		t.Fatalf("PDF title should keep the real prize name")
	}
}
