package funcs

import (
	"encoding/csv"
	"server/models"
	"strings"
	"testing"
)

// Header row from the 2025 Devpost project export
const devpostHeader = `Project Title,Submission Url,Project Status,Judging Status,Highest Step Completed,Project Created At,Project Submitted At,Submission Last Submitted At,About The Project,"""Try it out"" Links",Video Demo Link,Image Gallery URLs,Attached File S3 Key,Opt-In Prizes,Built With,Notes,Team Colleges/Universities,Additional Team Member Count`

func readCSVRows(t *testing.T, content string) [][]string {
	rows, err := csv.NewReader(strings.NewReader(content)).ReadAll()
	if err != nil {
		t.Fatalf("could not parse test csv: %s", err.Error())
	}
	return rows
}

func TestCheckDevpostHeaderAccepts2025Format(t *testing.T) {
	header := readCSVRows(t, devpostHeader)[0]
	if err := checkDevpostHeader(header); err != nil {
		t.Fatalf("expected 2025 header to be valid, got: %s", err.Error())
	}

	// A leading BOM should not break the check
	header[0] = "\xef\xbb\xbf" + header[0]
	if err := checkDevpostHeader(header); err != nil {
		t.Fatalf("expected header with BOM to be valid, got: %s", err.Error())
	}
}

func TestCheckDevpostHeaderRejectsShiftedFormat(t *testing.T) {
	// Old layout: prizes at column 9 instead of 13
	old := "Project Title,Submission Url,Project Status,Judging Status,Highest Step Completed,Project Created At,About The Project,Try it out Links,Video Demo Link,Opt-In Prizes,Built With,Notes,Team Colleges/Universities,Additional Team Member Count"
	if err := checkDevpostHeader(readCSVRows(t, old)[0]); err == nil {
		t.Fatal("expected shifted header to be rejected")
	}

	// Too few columns
	if err := checkDevpostHeader([]string{"Project Title", "Submission Url"}); err == nil {
		t.Fatal("expected short header to be rejected")
	}
}

func TestParseDevpostChallenges(t *testing.T) {
	got := parseDevpostChallenges("Best Design, Best Use of Gemini API by MLH, ,Toyota Financial Services,")
	want := []string{"Best Design", "Best Use of Gemini API by MLH", "Toyota Financial Services"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v, want %v", got, want)
	}

	if got := parseDevpostChallenges(""); len(got) != 0 {
		t.Fatalf("expected empty list, got %v", got)
	}
}

func TestCreateChallengeProjectCSV(t *testing.T) {
	p1 := models.NewProject("Second", 2, 1, "desc", "https://devpost.com/b", "", "", []string{"Nvidia"})
	p2 := models.NewProject("First", 1, 1, "desc", "https://devpost.com/a", "https://a.app, https://github.com/a", "https://youtu.be/a", []string{"Nvidia"})

	rows := readCSVRows(t, string(CreateChallengeProjectCSV([]*models.Project{p1, p2})))

	if strings.Join(rows[0], ",") != "Table,Name,Devpost,Try It Out" {
		t.Fatalf("unexpected header: %v", rows[0])
	}
	if len(rows) != 3 {
		t.Fatalf("expected 2 projects, got %d rows", len(rows)-1)
	}
	// Sorted by table number, links preserved (including comma-separated try links)
	want := []string{"1", "First", "https://devpost.com/a", "https://a.app, https://github.com/a"}
	if strings.Join(rows[1], "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected first row: %v", rows[1])
	}
	if rows[2][0] != "2" || rows[2][2] != "https://devpost.com/b" || rows[2][3] != "" {
		t.Fatalf("unexpected second row: %v", rows[2])
	}
}
