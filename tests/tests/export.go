package tests

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"strings"
	"tests/util"
)

// --- Export Tests ---

// readZipCSVs reads every CSV in a zip file, keyed by file name
func readZipCSVs(body string) (map[string][][]string, error) {
	r, err := zip.NewReader(bytes.NewReader([]byte(body)), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("not a valid zip: %s", err.Error())
	}

	files := map[string][][]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("could not open %s: %s", f.Name, err.Error())
		}
		content, _ := io.ReadAll(rc)
		rc.Close()
		rows, err := csv.NewReader(bytes.NewReader(content)).ReadAll()
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %s", f.Name, err.Error())
		}
		files[f.Name] = rows
	}
	return files, nil
}

// ExportByChallengesZip verifies that the export by challenges zip has one CSV per prize,
// each with the Table, Name, Devpost, Try It Out columns and sorted by table number
func ExportByChallengesZip(context *util.Context) util.Result {
	if res := resetProjects(context); !res.Success {
		return res
	}
	addTestProject(context, "Export A", "Design, Best Use of AI")
	addTestProject(context, "Export B", "Design")
	addTestProject(context, "Export C", "")

	// Move A behind B so sorting by table is actually checked
	ids, res := getProjectIDs(context, "Export A", "Export B")
	if !res.Success {
		return res
	}
	for i, location := range []int{50, 10} {
		if moveRes := util.PutRequest(context.Logger, "/project/move/"+ids[i], util.H{"location": location}, util.AdminAuth()); !util.IsOk(moveRes) {
			return util.NewResult(false, "Failed to move project: "+moveRes)
		}
	}

	status, body := util.GetRequestWithStatus(context.Logger, "/admin/export/challenges", util.AdminAuth())
	if status != 200 {
		return util.NewResult(false, fmt.Sprintf("Export by challenges failed (%d): %s", status, body))
	}
	files, err := readZipCSVs(body)
	if err != nil {
		return util.NewResult(false, "Could not read export by challenges zip: "+err.Error())
	}

	want := map[string][][]string{
		"Design.csv": {
			{"Table", "Name", "Devpost", "Try It Out"},
			{"10", "Export B", "https://example.com", ""},
			{"50", "Export A", "https://example.com", ""},
		},
		"Best Use of AI.csv": {
			{"Table", "Name", "Devpost", "Try It Out"},
			{"50", "Export A", "https://example.com", ""},
		},
	}
	if len(files) != len(want) {
		names := []string{}
		for name := range files {
			names = append(names, name)
		}
		return util.NewResult(false, fmt.Sprintf("Expected one CSV per prize (%d), got %v", len(want), names))
	}
	for name, wantRows := range want {
		gotRows, ok := files[name]
		if !ok {
			return util.NewResult(false, "Missing "+name+" in export zip")
		}
		if len(gotRows) != len(wantRows) {
			return util.NewResult(false, fmt.Sprintf("%s should have %d rows, got %v", name, len(wantRows), gotRows))
		}
		for i := range wantRows {
			if !slices.Equal(gotRows[i], wantRows[i]) {
				return util.NewResult(false, fmt.Sprintf("%s row %d should be %v, got %v", name, i, wantRows[i], gotRows[i]))
			}
		}
	}
	return util.ResultOk()
}

// devpostPIICSV is a small Devpost PII export: one team of three, one solo submitter, a project hidden
// on Devpost and a draft (custom question columns shortened). The hidden project and the draft are
// never imported, so the exact-row checks below also verify they're skipped. Like the real export, only Team Member 1's columns are named,
// followed by a "..." column where the team's second member's unlabeled cells start
const devpostPIICSV = `Project Title,Submission Url,Project Status,Judging Status,Highest Step Completed,Project Created At,Project Submitted At,Submission Last Submitted At,About The Project,"""Try it out"" Links",Video Demo Link,Image Gallery URLs,Attached File S3 Key,Opt-In Prizes,Built With,Submitter First Name,Submitter Last Name,Submitter Email,Notes,Some Custom Question,Additional Team Member Count,Team Member 1 First Name,Team Member 1 Last Name,Team Member 1 Email,...
Team Project,https://devpost.com/1-team,Submitted (Gallery/Visible),Pending,Submit,,,,About,,,,,Design,go,Ann ,Lee,ann@example.com,,,2,Bob,Kim,bob@example.com,carlusername,,carl@example.com
Solo Project,https://devpost.com/2-solo,Submitted (Gallery/Visible),Pending,Submit,,,,About,,,,,,go,Dee,Ray,dee@example.com,,,0
Hidden Project,https://devpost.com/3-hidden,Submitted (Hidden),Pending,Submit,,,,About,,,,,Design,go,Hal,Hid,hal@example.com,,,0
Untitled,,Draft,Pending,Manage team,,,,,,,,,,,Eve,Fox,eve@example.com,,,0`

// ExportParticipantsCSV verifies that importing the Devpost PII export stores team members, that
// /admin/export/participants lists their names and emails in separate columns sorted by table, and that
// the emails never show up in other project responses
func ExportParticipantsCSV(context *util.Context) util.Result {
	if res := resetProjects(context); !res.Success {
		return res
	}
	status, body := util.PostFileWithStatus(context.Logger, "/project/devpost", "csv", "devpost.csv", devpostPIICSV, nil, util.AdminAuth())
	if status != 200 || !util.IsOk(body) {
		return util.NewResult(false, fmt.Sprintf("Devpost PII import failed (%d): %s", status, body))
	}

	// Move the team behind the solo project so sorting by table is actually checked
	ids, res := getProjectIDs(context, "Team Project", "Solo Project")
	if !res.Success {
		return res
	}
	for i, location := range []int{50, 10} {
		if moveRes := util.PutRequest(context.Logger, "/project/move/"+ids[i], util.H{"location": location}, util.AdminAuth()); !util.IsOk(moveRes) {
			return util.NewResult(false, "Failed to move project: "+moveRes)
		}
	}

	status, body = util.GetRequestWithStatus(context.Logger, "/admin/export/participants", util.AdminAuth())
	if status != 200 {
		return util.NewResult(false, fmt.Sprintf("Export participants failed (%d): %s", status, body))
	}
	rows, err := csv.NewReader(bytes.NewReader([]byte(body))).ReadAll()
	if err != nil {
		return util.NewResult(false, "Could not parse participants CSV: "+err.Error())
	}
	want := [][]string{
		{"Table", "Project", "Devpost", "Member 1 Name", "Member 2 Name", "Member 3 Name", "Member 1 Email", "Member 2 Email", "Member 3 Email"},
		{"10", "Solo Project", "https://devpost.com/2-solo", "Dee Ray", "", "", "dee@example.com", "", ""},
		{"50", "Team Project", "https://devpost.com/1-team", "Ann Lee", "Bob Kim", "carlusername", "ann@example.com", "bob@example.com", "carl@example.com"},
	}
	if len(rows) != len(want) {
		return util.NewResult(false, fmt.Sprintf("Participants CSV should have %d rows, got %v", len(want), rows))
	}
	for i := range want {
		if !slices.Equal(rows[i], want[i]) {
			return util.NewResult(false, fmt.Sprintf("Participants CSV row %d should be %v, got %v", i, want[i], rows[i]))
		}
	}

	// The export is admin only
	if status, _ := util.GetRequestWithStatus(context.Logger, "/admin/export/participants", util.DefaultAuth()); status == 200 {
		return util.NewResult(false, "Participants export must require admin auth")
	}

	// Emails must not leak through other project responses or exports
	for _, url := range []string{"/project/list", "/project/list/public", "/admin/export/projects"} {
		_, listBody := util.GetRequestWithStatus(context.Logger, url, util.AdminAuth())
		if strings.Contains(listBody, "@example.com") {
			return util.NewResult(false, url+" should not include team member emails")
		}
	}
	return util.ResultOk()
}

// ExportByChallengesWithTeamMembers verifies that ?team_members=true adds the participants export's
// member name and email columns to each prize CSV, and that they are left out without it
func ExportByChallengesWithTeamMembers(context *util.Context) util.Result {
	if res := resetProjects(context); !res.Success {
		return res
	}
	status, body := util.PostFileWithStatus(context.Logger, "/project/devpost", "csv", "devpost.csv", devpostPIICSV, nil, util.AdminAuth())
	if status != 200 || !util.IsOk(body) {
		return util.NewResult(false, fmt.Sprintf("Devpost PII import failed (%d): %s", status, body))
	}

	// With team members: only Team Project opted into a prize (Design)
	status, body = util.GetRequestWithStatus(context.Logger, "/admin/export/challenges?team_members=true", util.AdminAuth())
	if status != 200 {
		return util.NewResult(false, fmt.Sprintf("Export by challenges with team members failed (%d): %s", status, body))
	}
	files, err := readZipCSVs(body)
	if err != nil {
		return util.NewResult(false, "Could not read export by challenges zip: "+err.Error())
	}
	rows, ok := files["Design.csv"]
	if !ok || len(files) != 1 {
		return util.NewResult(false, fmt.Sprintf("Expected only Design.csv in export zip, got %v", files))
	}
	if len(rows) != 2 || len(rows[1]) == 0 {
		return util.NewResult(false, fmt.Sprintf("Design.csv should have a header and 1 project, got %v", rows))
	}
	want := [][]string{
		{"Table", "Name", "Devpost", "Try It Out", "Member 1 Name", "Member 2 Name", "Member 3 Name", "Member 1 Email", "Member 2 Email", "Member 3 Email"},
		// Table number is whatever the import assigned
		{rows[1][0], "Team Project", "https://devpost.com/1-team", "", "Ann Lee", "Bob Kim", "carlusername", "ann@example.com", "bob@example.com", "carl@example.com"},
	}
	for i := range want {
		if !slices.Equal(rows[i], want[i]) {
			return util.NewResult(false, fmt.Sprintf("Design.csv row %d should be %v, got %v", i, want[i], rows[i]))
		}
	}

	// Without the param (or with anything other than "true") no emails are exported
	for _, url := range []string{"/admin/export/challenges", "/admin/export/challenges?team_members=1"} {
		status, body = util.GetRequestWithStatus(context.Logger, url, util.AdminAuth())
		if status != 200 {
			return util.NewResult(false, fmt.Sprintf("%s failed (%d): %s", url, status, body))
		}
		files, err = readZipCSVs(body)
		if err != nil {
			return util.NewResult(false, "Could not read export by challenges zip: "+err.Error())
		}
		rows := files["Design.csv"]
		if len(rows) == 0 || !slices.Equal(rows[0], []string{"Table", "Name", "Devpost", "Try It Out"}) {
			return util.NewResult(false, fmt.Sprintf("%s should not include team member columns, got %v", url, rows))
		}
	}
	return util.ResultOk()
}

// ExportDiscordCSV verifies that /admin/export/discord lists each project's name, table and Devpost
// URL sorted alphabetically by name (ignoring case), without team member emails, and is admin only
func ExportDiscordCSV(context *util.Context) util.Result {
	if res := resetProjects(context); !res.Success {
		return res
	}
	status, body := util.PostFileWithStatus(context.Logger, "/project/devpost", "csv", "devpost.csv", devpostPIICSV, nil, util.AdminAuth())
	if status != 200 || !util.IsOk(body) {
		return util.NewResult(false, fmt.Sprintf("Devpost PII import failed (%d): %s", status, body))
	}
	addTestProject(context, "alpha Project", "")

	// Put the projects in reverse alphabetical table order so sorting by name is actually checked
	ids, res := getProjectIDs(context, "alpha Project", "Solo Project", "Team Project")
	if !res.Success {
		return res
	}
	for i, location := range []int{30, 20, 10} {
		if moveRes := util.PutRequest(context.Logger, "/project/move/"+ids[i], util.H{"location": location}, util.AdminAuth()); !util.IsOk(moveRes) {
			return util.NewResult(false, "Failed to move project: "+moveRes)
		}
	}

	status, body = util.GetRequestWithStatus(context.Logger, "/admin/export/discord", util.AdminAuth())
	if status != 200 {
		return util.NewResult(false, fmt.Sprintf("Export for Discord failed (%d): %s", status, body))
	}
	if strings.Contains(body, "@example.com") {
		return util.NewResult(false, "Discord export should not include team member emails")
	}
	rows, err := csv.NewReader(bytes.NewReader([]byte(body))).ReadAll()
	if err != nil {
		return util.NewResult(false, "Could not parse Discord CSV: "+err.Error())
	}
	want := [][]string{
		{"Project", "Table", "Devpost"},
		{"alpha Project", "30", "https://example.com"},
		{"Solo Project", "20", "https://devpost.com/2-solo"},
		{"Team Project", "10", "https://devpost.com/1-team"},
	}
	if len(rows) != len(want) {
		return util.NewResult(false, fmt.Sprintf("Discord CSV should have %d rows, got %v", len(want), rows))
	}
	for i := range want {
		if !slices.Equal(rows[i], want[i]) {
			return util.NewResult(false, fmt.Sprintf("Discord CSV row %d should be %v, got %v", i, want[i], rows[i]))
		}
	}

	// The export is admin only
	if status, _ := util.GetRequestWithStatus(context.Logger, "/admin/export/discord", util.DefaultAuth()); status == 200 {
		return util.NewResult(false, "Discord export must require admin auth")
	}
	return util.ResultOk()
}

// ExportPrizesZip verifies that /admin/export/prizes returns a zip with a CSV per prize that
// includes team member emails, and a PDF per prize with the prize name and no emails
func ExportPrizesZip(context *util.Context) util.Result {
	if res := resetProjects(context); !res.Success {
		return res
	}
	status, body := util.PostFileWithStatus(context.Logger, "/project/devpost", "csv", "devpost.csv", devpostPIICSV, nil, util.AdminAuth())
	if status != 200 || !util.IsOk(body) {
		return util.NewResult(false, fmt.Sprintf("Devpost PII import failed (%d): %s", status, body))
	}

	status, body = util.GetRequestWithStatus(context.Logger, "/admin/export/prizes", util.AdminAuth())
	if status != 200 {
		return util.NewResult(false, fmt.Sprintf("Export prizes failed (%d): %s", status, body))
	}
	r, err := zip.NewReader(bytes.NewReader([]byte(body)), int64(len(body)))
	if err != nil {
		return util.NewResult(false, "Prize export is not a valid zip: "+err.Error())
	}
	files := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return util.NewResult(false, "Could not open "+f.Name+": "+err.Error())
		}
		content, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(content)
	}

	// Only the team project opted into a prize (Design)
	if len(files) != 2 {
		return util.NewResult(false, fmt.Sprintf("Prize export should have 2 files, got %d", len(files)))
	}
	csvFile, ok := files["csv-with-emails/Design.csv"]
	if !ok || !strings.Contains(csvFile, "bob@example.com") || !strings.Contains(csvFile, "Member 1 Email") {
		return util.NewResult(false, "csv-with-emails/Design.csv should exist and include team member emails")
	}
	pdfFile, ok := files["pdf/Design.pdf"]
	if !ok || !strings.HasPrefix(pdfFile, "%PDF-") || !strings.Contains(pdfFile, "(Design) Tj") || !strings.Contains(pdfFile, "(Team Project) Tj") {
		return util.NewResult(false, "pdf/Design.pdf should be a PDF with the prize and project names")
	}
	if strings.Contains(pdfFile, "@example.com") {
		return util.NewResult(false, "Prize PDFs should not include emails")
	}

	// The export is admin only
	if status, _ := util.GetRequestWithStatus(context.Logger, "/admin/export/prizes", util.DefaultAuth()); status == 200 {
		return util.NewResult(false, "Prize export must require admin auth")
	}
	return util.ResultOk()
}

// devpostNvidiaCSV is a small Devpost PII export for the Nvidia export: Nvidia only, Nvidia + 1
// sponsor, a project without Nvidia, and Nvidia + 2 sponsors (both Spacetime prizes count once)
const devpostNvidiaCSV = `Project Title,Submission Url,Project Status,Judging Status,Highest Step Completed,Project Created At,Project Submitted At,Submission Last Submitted At,About The Project,"""Try it out"" Links",Video Demo Link,Image Gallery URLs,Attached File S3 Key,Opt-In Prizes,Built With,Submitter First Name,Submitter Last Name,Submitter Email,Notes,Additional Team Member Count,Team Member 1 First Name,Team Member 1 Last Name,Team Member 1 Email
GPU Pair,https://devpost.com/1-pair,Submitted (Gallery/Visible),Pending,Submit,,,,About,,,,,"Nvidia Track, PNC Track",go,Ann,Lee,ann@example.com,,1,Bob,Kim,bob@example.com
GPU Solo,https://devpost.com/2-solo,Submitted (Gallery/Visible),Pending,Submit,,,,About,,,,,"Nvidia Track, Best Design",go,Dee,Ray,dee@example.com,,0
No GPU,https://devpost.com/3-none,Submitted (Gallery/Visible),Pending,Submit,,,,About,,,,,PNC Track,go,Eve,Fox,eve@example.com,,0
GPU Trio,https://devpost.com/4-trio,Submitted (Gallery/Visible),Pending,Submit,,,,About,,,,,"Nvidia Track, Spacetime Track, Spacetime ScrabbleBot Prize, CBRE Track",go,Gus,Hale,gus@example.com,,0`

// ExportNvidiaZip verifies that /admin/export/nvidia returns nvidia.pdf and nvidia.csv with the
// Nvidia projects grouped by number of other sponsors and sorted by table, with emails only in
// the CSV, and that it is admin only
func ExportNvidiaZip(context *util.Context) util.Result {
	if res := resetProjects(context); !res.Success {
		return res
	}
	status, body := util.PostFileWithStatus(context.Logger, "/project/devpost", "csv", "devpost.csv", devpostNvidiaCSV, nil, util.AdminAuth())
	if status != 200 || !util.IsOk(body) {
		return util.NewResult(false, fmt.Sprintf("Devpost import failed (%d): %s", status, body))
	}

	status, body = util.GetRequestWithStatus(context.Logger, "/admin/export/nvidia", util.AdminAuth())
	if status != 200 {
		return util.NewResult(false, fmt.Sprintf("Export Nvidia failed (%d): %s", status, body))
	}
	r, err := zip.NewReader(bytes.NewReader([]byte(body)), int64(len(body)))
	if err != nil {
		return util.NewResult(false, "Nvidia export is not a valid zip: "+err.Error())
	}
	files := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return util.NewResult(false, "Could not open "+f.Name+": "+err.Error())
		}
		content, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(content)
	}
	pdf, okPdf := files["nvidia.pdf"]
	csvFile, okCsv := files["nvidia.csv"]
	if len(files) != 2 || !okPdf || !okCsv {
		return util.NewResult(false, fmt.Sprintf("Nvidia export should have nvidia.pdf and nvidia.csv, got %d files", len(files)))
	}

	// PDF: title, groups and projects, without the non-Nvidia project or any emails
	if !strings.HasPrefix(pdf, "%PDF-") || !strings.Contains(pdf, "(Nvidia Track) Tj") || !strings.Contains(pdf, "(GPU Solo) Tj") {
		return util.NewResult(false, "nvidia.pdf should be a PDF with the Nvidia prize and project names")
	}
	if strings.Contains(pdf, "(No GPU)") || strings.Contains(pdf, "@example.com") {
		return util.NewResult(false, "nvidia.pdf should only list Nvidia projects and have no emails")
	}

	// CSV: Nvidia only, then + 1, then + 2, with the team member emails
	rows, err := csv.NewReader(strings.NewReader(csvFile)).ReadAll()
	if err != nil {
		return util.NewResult(false, "Could not parse nvidia.csv: "+err.Error())
	}
	got := []string{}
	for _, row := range rows[1:] {
		got = append(got, row[0]+"|"+row[1]+"|"+row[2])
	}
	want := []string{"Nvidia only|2|GPU Solo", "Nvidia + 1 sponsor|1|GPU Pair", "Nvidia + 2 sponsors|4|GPU Trio"}
	if !slices.Equal(got, want) {
		return util.NewResult(false, fmt.Sprintf("nvidia.csv rows should be %v, got %v", want, got))
	}
	if !strings.Contains(csvFile, "bob@example.com") {
		return util.NewResult(false, "nvidia.csv should include team member emails")
	}

	// The export is admin only
	if status, _ := util.GetRequestWithStatus(context.Logger, "/admin/export/nvidia", util.DefaultAuth()); status == 200 {
		return util.NewResult(false, "Nvidia export must require admin auth")
	}
	return util.ResultOk()
}
