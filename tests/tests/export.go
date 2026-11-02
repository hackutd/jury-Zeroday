package tests

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"tests/util"
)

// --- Export Tests ---

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
	r, err := zip.NewReader(bytes.NewReader([]byte(body)), int64(len(body)))
	if err != nil {
		return util.NewResult(false, "Export by challenges is not a valid zip: "+err.Error())
	}

	// Read every CSV in the zip
	files := map[string][][]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return util.NewResult(false, "Could not open "+f.Name+": "+err.Error())
		}
		content, _ := io.ReadAll(rc)
		rc.Close()
		rows, err := csv.NewReader(bytes.NewReader(content)).ReadAll()
		if err != nil {
			return util.NewResult(false, "Could not parse "+f.Name+": "+err.Error())
		}
		files[f.Name] = rows
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
