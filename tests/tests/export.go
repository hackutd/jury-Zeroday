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

// devpostPIICSV is a small Devpost PII export: one team of three, one solo submitter, and a draft
// (custom question columns shortened). Like the real export, only Team Member 1's columns are named,
// followed by a "..." column where the team's second member's unlabeled cells start
const devpostPIICSV = `Project Title,Submission Url,Project Status,Judging Status,Highest Step Completed,Project Created At,Project Submitted At,Submission Last Submitted At,About The Project,"""Try it out"" Links",Video Demo Link,Image Gallery URLs,Attached File S3 Key,Opt-In Prizes,Built With,Submitter First Name,Submitter Last Name,Submitter Email,Notes,Some Custom Question,Additional Team Member Count,Team Member 1 First Name,Team Member 1 Last Name,Team Member 1 Email,...
Team Project,https://devpost.com/1-team,Submitted (Gallery/Visible),Pending,Submit,,,,About,,,,,Design,go,Ann ,Lee,ann@example.com,,,2,Bob,Kim,bob@example.com,carlusername,,carl@example.com
Solo Project,https://devpost.com/2-solo,Submitted (Hidden),Pending,Submit,,,,About,,,,,,go,Dee,Ray,dee@example.com,,,0
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
