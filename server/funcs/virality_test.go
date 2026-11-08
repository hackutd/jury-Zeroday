package funcs

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"io"
	"server/models"
	"slices"
	"strings"
	"testing"
)

func TestCreateViralityExportZip(t *testing.T) {
	newProject := func(name string, table int64, track string) *models.Project {
		p := models.NewProject(name, table, 0, "", "https://devpost.com/"+name, "", "", []string{})
		p.HackUTDTrack = track
		p.TeamMembers = []models.TeamMember{{Name: name + " Member", Email: name + "@example.com"}}
		return p
	}
	projects := []*models.Project{
		newProject("Viral B", 7, "Virality"),
		newProject("Design", 1, "Best Design"),
		newProject("Viral A", 3, " virality "),
		newProject("Opted Out", 2, "None"),
		newProject("No Answer", 4, ""),
	}
	// A Virality opt-in prize without the track doesn't count
	projects[1].ChallengeList = []string{"Virality"}

	zipBytes, err := CreateViralityExportZip(projects)
	if err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range r.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(data)
	}
	if len(files) != 2 {
		t.Fatalf("expected virality.pdf and virality.csv, got %d files", len(files))
	}

	pdf := files["virality.pdf"]
	if !strings.HasPrefix(pdf, "%PDF-") || !strings.Contains(pdf, "(Virality) Tj") || !strings.Contains(pdf, "(Viral A) Tj") {
		t.Error("virality.pdf should be a PDF with the Virality title and projects")
	}
	if strings.Contains(pdf, "(Design)") || strings.Contains(pdf, "@example.com") {
		t.Error("virality.pdf should only list Virality projects and have no emails")
	}

	rows, err := csv.NewReader(strings.NewReader(files["virality.csv"])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, row := range rows[1:] {
		got = append(got, row[0]+"|"+row[1]+"|"+row[5])
	}
	want := []string{"3|Viral A|Viral A@example.com", "7|Viral B|Viral B@example.com"}
	if !slices.Equal(got, want) {
		t.Errorf("virality.csv rows should be %v, got %v", want, got)
	}
}
