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

	rows := readCSVRows(t, string(CreateChallengeProjectCSV([]*models.Project{p1, p2}, false)))

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

// Header row from the 2025 Devpost PII export (custom questions shortened). Like the real export, only
// Team Member 1's columns are named, then a "..." column where the unlabeled members start
const devpostPIIHeader = `Project Title,Submission Url,Project Status,Judging Status,Highest Step Completed,Project Created At,Project Submitted At,Submission Last Submitted At,About The Project,"""Try it out"" Links",Video Demo Link,Image Gallery URLs,Attached File S3 Key,Opt-In Prizes,Built With,Submitter First Name,Submitter Last Name,Submitter Email,Notes,Some Custom Question,Additional Team Member Count,Team Member 1 First Name,Team Member 1 Last Name,Team Member 1 Email,...`

func TestFindDevpostTeamColumns(t *testing.T) {
	cols, extraFrom := findDevpostTeamColumns(readCSVRows(t, devpostPIIHeader)[0])
	want := []devpostTeamCols{{15, 16, 17}, {21, 22, 23}}
	if len(cols) != len(want) || cols[0] != want[0] || cols[1] != want[1] {
		t.Fatalf("got %v, want %v", cols, want)
	}
	if extraFrom != 24 {
		t.Fatalf("expected unlabeled members to start at column 24, got %d", extraFrom)
	}

	// Same header without the "..." placeholder
	noDots := readCSVRows(t, devpostPIIHeader)[0]
	if _, extraFrom := findDevpostTeamColumns(noDots[:len(noDots)-1]); extraFrom != 24 {
		t.Fatalf("expected unlabeled members to start at column 24 without the placeholder, got %d", extraFrom)
	}

	// The non-PII export has no team columns
	if cols, extraFrom := findDevpostTeamColumns(readCSVRows(t, devpostHeader)[0]); len(cols) != 0 || extraFrom != -1 {
		t.Fatalf("expected no team columns in non-PII header, got %v, %d", cols, extraFrom)
	}

	// Named member columns that don't end the header are not followed by unlabeled members
	header := append(readCSVRows(t, devpostPIIHeader)[0], "Some Later Question")
	if _, extraFrom := findDevpostTeamColumns(header); extraFrom != -1 {
		t.Fatalf("expected no unlabeled members when member columns don't end the header, got %d", extraFrom)
	}
}

func TestParseDevpostTeam(t *testing.T) {
	cols, extraFrom := findDevpostTeamColumns(readCSVRows(t, devpostPIIHeader)[0])

	// Submitter, Team Member 1, then two unlabeled members past the header (one with no last name)
	record := make([]string, 30)
	record[15], record[16], record[17] = " Keshav ", "Taneja ", "keshav@example.com"
	record[21], record[22], record[23] = "Prakrit", "Chauhan", "prakrit@example.com"
	record[24], record[25], record[26] = "Niket", "Sheth", "niket@example.com"
	record[27], record[28], record[29] = "username01", "", "user@example.com"

	got := parseDevpostTeam(record, cols, extraFrom)
	want := []string{
		"Keshav Taneja <keshav@example.com>",
		"Prakrit Chauhan <prakrit@example.com>",
		"Niket Sheth <niket@example.com>",
		"username01 <user@example.com>",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, m := range got {
		if m.Name+" <"+m.Email+">" != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}

	// A solo submitter's row ends before Team Member 1's columns
	if got := parseDevpostTeam(record[:21], cols, extraFrom); len(got) != 1 || got[0].Name != "Keshav Taneja" {
		t.Fatalf("expected only the submitter, got %v", got)
	}

	// Parsing a row must not change the shared columns for later rows
	if len(cols) != 2 {
		t.Fatalf("parseDevpostTeam modified the shared columns: %v", cols)
	}

	// A person listed twice (same email) is only added once
	record[21], record[22], record[23] = "Keshav", "Taneja", "KESHAV@example.com"
	if got := parseDevpostTeam(record[:24], cols, extraFrom); len(got) != 1 {
		t.Fatalf("expected duplicate member to be dropped, got %v", got)
	}
}

func TestCreateChallengeProjectCSVWithTeamMembers(t *testing.T) {
	p1 := models.NewProject("Second", 20, 1, "desc", "https://devpost.com/b", "https://b.app", "", []string{"Nvidia"})
	p1.TeamMembers = []models.TeamMember{{Name: "Ann A", Email: "ann@example.com"}}
	p2 := models.NewProject("First", 10, 1, "desc", "https://devpost.com/a", "", "", []string{"Nvidia"})
	p2.TeamMembers = []models.TeamMember{{Name: "Bob B", Email: "bob@example.com"}, {Name: "Cat C", Email: ""}}

	// Same columns as without team members, then the participants export's member columns
	rows := readCSVRows(t, string(CreateChallengeProjectCSV([]*models.Project{p1, p2}, true)))
	want := [][]string{
		{"Table", "Name", "Devpost", "Try It Out", "Member 1 Name", "Member 2 Name", "Member 1 Email", "Member 2 Email"},
		{"10", "First", "https://devpost.com/a", "", "Bob B", "Cat C", "bob@example.com", ""},
		{"20", "Second", "https://devpost.com/b", "https://b.app", "Ann A", "", "ann@example.com", ""},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %v, want %v", rows, want)
	}
	for i := range want {
		if strings.Join(rows[i], "|") != strings.Join(want[i], "|") {
			t.Fatalf("row %d: got %v, want %v", i, rows[i], want[i])
		}
	}

	// Leaving team members out must not leak them
	rows = readCSVRows(t, string(CreateChallengeProjectCSV([]*models.Project{p1, p2}, false)))
	if strings.Join(rows[0], ",") != "Table,Name,Devpost,Try It Out" || len(rows[1]) != 4 {
		t.Fatalf("expected no member columns, got %v", rows)
	}
}

func TestCreateParticipantsCSV(t *testing.T) {
	p1 := models.NewProject("Second", 20, 1, "desc", "https://devpost.com/b", "", "", []string{})
	p1.TeamMembers = []models.TeamMember{{Name: "Ann A", Email: "ann@example.com"}}
	p2 := models.NewProject("First", 10, 1, "desc", "https://devpost.com/a", "", "", []string{})
	p2.TeamMembers = []models.TeamMember{{Name: "Bob B", Email: "bob@example.com"}, {Name: "Cat C", Email: ""}}
	p3 := models.NewProject("Third", 30, 1, "desc", "https://devpost.com/c", "", "", []string{})
	p3.TeamMembers = nil // imported before team members existed

	rows := readCSVRows(t, string(CreateParticipantsCSV([]*models.Project{p1, p2, p3})))
	want := [][]string{
		{"Table", "Project", "Devpost", "Member 1 Name", "Member 2 Name", "Member 1 Email", "Member 2 Email"},
		{"10", "First", "https://devpost.com/a", "Bob B", "Cat C", "bob@example.com", ""},
		{"20", "Second", "https://devpost.com/b", "Ann A", "", "ann@example.com", ""},
		{"30", "Third", "https://devpost.com/c", "", "", "", ""},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %v, want %v", rows, want)
	}
	for i := range want {
		if strings.Join(rows[i], "|") != strings.Join(want[i], "|") {
			t.Fatalf("row %d: got %v, want %v", i, rows[i], want[i])
		}
	}
}

func TestCreateDiscordCSV(t *testing.T) {
	zebra := models.NewProject("Zebra", 1, 1, "desc", "https://devpost.com/z", "", "", []string{})
	zebra.TeamMembers = []models.TeamMember{{Name: "Ann A", Email: "ann@example.com"}}
	apple := models.NewProject("apple", 30, 1, "desc", "https://devpost.com/a", "", "", []string{})
	bananaLate := models.NewProject("Banana", 20, 1, "desc", "https://devpost.com/b2", "", "", []string{})
	bananaEarly := models.NewProject("Banana", 5, 1, "desc", "https://devpost.com/b1", "", "", []string{})

	rows := readCSVRows(t, string(CreateDiscordCSV([]*models.Project{zebra, apple, bananaLate, bananaEarly})))
	// Alphabetical ignoring case, same names ordered by table, and no team member columns
	want := [][]string{
		{"Table", "Project", "Devpost"},
		{"30", "apple", "https://devpost.com/a"},
		{"5", "Banana", "https://devpost.com/b1"},
		{"20", "Banana", "https://devpost.com/b2"},
		{"1", "Zebra", "https://devpost.com/z"},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %v, want %v", rows, want)
	}
	for i := range want {
		if strings.Join(rows[i], "|") != strings.Join(want[i], "|") {
			t.Fatalf("row %d: got %v, want %v", i, rows[i], want[i])
		}
	}
}
