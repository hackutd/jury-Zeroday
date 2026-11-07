package funcs

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"server/models"
	"strings"
	"testing"
)

func TestSponsorOf(t *testing.T) {
	cases := map[string]string{
		// This year's names (from Devpost) and last year's
		"Nvidia Track":                    "Nvidia",
		"Nvidia":                          "Nvidia",
		"Capital One Track":               "Capital One",
		"State Farm Track":                "State Farm",
		"StateFarm":                       "State Farm",
		"CBRE Track":                      "CBRE",
		"Spacetime Track":                 "Spacetime",
		"Spacetime ScrabbleBot Prize":     "Spacetime",
		"Photon Track":                    "Photon",
		"T-Mobile Track":                  "T-Mobile",
		"8090 Track":                      "8090",
		"PNC Track":                       "PNC",
		"PNC Bank":                        "PNC",
		"Goldman Sachs Track":             "Goldman Sachs",
		"EOG Resources Track":             "EOG Resources",
		"Winstead PC Track":               "Winstead PC",
		"Toyota Financial Services Track": "Toyota Financial Services",
		"NMC^2 Track":                     "NMC^2",
		"Northmark Compute & Cloud":       "NMC^2",
		// Not sponsors
		"Best Design":                   "",
		"Best Beginner":                 "",
		"Best Use of Gemini API by MLH": "",
		"":                              "",
	}
	for prize, want := range cases {
		if got := SponsorOf(prize); got != want {
			t.Errorf("SponsorOf(%q) = %q, want %q", prize, got, want)
		}
	}
}

// nvidiaTestProjects returns projects at the given tables with the given opt-in prizes
func nvidiaTestProjects() []*models.Project {
	p := func(name string, table int64, prizes ...string) *models.Project {
		return models.NewProject(name, table, 0, "", "https://devpost.com/"+name, "", "", prizes)
	}
	return []*models.Project{
		p("TwoOthers", 1, "Nvidia Track", "PNC Track", "CBRE Track", "Best Design"),
		p("OneOtherLate", 9, "Nvidia Track", "T-Mobile Track"),
		p("SpacetimeTwice", 5, "Nvidia Track", "Spacetime Track", "Spacetime ScrabbleBot Prize"),
		p("OnlyLate", 7, "Nvidia Track", "Best Beginner"),
		p("OnlyEarly", 3, "Nvidia Track"),
		p("NotNvidia", 2, "PNC Track", "Best Design"),
		p("ThreeOthers", 4, "Nvidia Track", "PNC Track", "CBRE Track", "Photon Track"),
	}
}

func TestGroupNvidiaProjects(t *testing.T) {
	grouped, title := groupNvidiaProjects(nvidiaTestProjects())
	if title != "Nvidia Track" {
		t.Fatalf("title = %q", title)
	}
	got := []string{}
	for _, p := range grouped {
		got = append(got, nvidiaGroupLabels[p.group]+": "+p.project.Name)
	}
	want := []string{
		"Nvidia only: OnlyEarly",
		"Nvidia only: OnlyLate",
		"Nvidia + 1 sponsor: SpacetimeTwice", // both Spacetime prizes are one sponsor
		"Nvidia + 1 sponsor: OneOtherLate",
		"Nvidia + 2 sponsors: TwoOthers",
		"Nvidia + 3 or more sponsors: ThreeOthers",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestCreateNvidiaPDF(t *testing.T) {
	pdf := CreateNvidiaPDF(nvidiaTestProjects())
	checkPDFStructure(t, pdf)
	for _, want := range []string{
		"(Nvidia Track) Tj",
		"(Nvidia only \\(2\\)) Tj",
		"(Nvidia + 1 sponsor \\(2\\)) Tj",
		"(Nvidia + 2 sponsors \\(1\\)) Tj",
		"(Nvidia + 3 or more sponsors \\(1\\)) Tj",
		"(Also: PNC Track, CBRE Track) Tj",
		"(Also: Spacetime Track, Spacetime ScrabbleBot Prize) Tj",
		"(6 projects) Tj",
	} {
		if !bytes.Contains(pdf, []byte(want)) {
			t.Fatalf("PDF should contain %s", want)
		}
	}
	if bytes.Contains(pdf, []byte("(NotNvidia)")) {
		t.Fatalf("PDF should not list projects without Nvidia")
	}
	// Groups are in order: only, +1, +2
	only := bytes.Index(pdf, []byte("(Nvidia only"))
	one := bytes.Index(pdf, []byte("(Nvidia + 1"))
	two := bytes.Index(pdf, []byte("(Nvidia + 2"))
	if !(only < one && one < two) {
		t.Fatalf("groups are out of order")
	}

	// Empty groups still show, and there's no 3+ group when nobody is in it
	empty := CreateNvidiaPDF([]*models.Project{models.NewProject("Solo", 1, 0, "", "", "", "", []string{"Nvidia"})})
	checkPDFStructure(t, empty)
	if !bytes.Contains(empty, []byte("(No projects.) Tj")) || bytes.Contains(empty, []byte("3 or more")) {
		t.Fatalf("empty groups should say No projects. and the 3+ group should be hidden")
	}

	// Many projects take more than one page, with the group continued on the next one
	many := []*models.Project{}
	for i := 1; i <= 60; i++ {
		many = append(many, models.NewProject("Project", int64(i), 0, "", "", "", "", []string{"Nvidia Track", "PNC Track"}))
	}
	big := CreateNvidiaPDF(many)
	checkPDFStructure(t, big)
	if !bytes.Contains(big, []byte("(continued\\)")) || bytes.Count(big, []byte("/Type /Page /Parent")) < 2 {
		t.Fatalf("long groups should continue on a new page")
	}

	// Set JURY_PDF_OUT to a path to look at the PDF
	if out := os.Getenv("JURY_PDF_OUT"); out != "" {
		os.WriteFile(out, CreateNvidiaPDF(append(nvidiaTestProjects(), many...)), 0o644)
	}
}

func TestCreateNvidiaExportZip(t *testing.T) {
	projects := nvidiaTestProjects()
	projects[1].TeamMembers = []models.TeamMember{{Name: "Ann A", Email: "ann@example.com"}}

	data, err := CreateNvidiaExportZip(projects)
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
	if len(files) != 2 || files["nvidia.pdf"] == nil || files["nvidia.csv"] == nil {
		t.Fatalf("zip should have nvidia.pdf and nvidia.csv, got %d files", len(files))
	}
	checkPDFStructure(t, files["nvidia.pdf"])
	if bytes.Contains(files["nvidia.pdf"], []byte("@example.com")) {
		t.Fatalf("PDF should not include emails")
	}

	rows := readCSVRows(t, string(files["nvidia.csv"]))
	if strings.Join(rows[0], "|") != "Group|Table|Name|Devpost|Try It Out|Other Sponsors|Member 1 Name|Member 1 Email" {
		t.Fatalf("unexpected header %v", rows[0])
	}
	if len(rows) != 7 {
		t.Fatalf("expected 6 projects, got %v", rows)
	}
	if strings.Join(rows[1][:3], "|") != "Nvidia only|3|OnlyEarly" {
		t.Fatalf("first row should be the earliest Nvidia-only project, got %v", rows[1])
	}
	if strings.Join(rows[4], "|") != "Nvidia + 1 sponsor|9|OneOtherLate|https://devpost.com/OneOtherLate||T-Mobile Track|Ann A|ann@example.com" {
		t.Fatalf("unexpected row %v", rows[4])
	}
}
