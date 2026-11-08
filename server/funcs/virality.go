package funcs

import (
	"archive/zip"
	"bytes"
	"server/models"
	"strings"
)

// ViralityTrack is the HackUTD track answer (Devpost "What Hack Utd Track" question) exported by
// the Virality export
const ViralityTrack = "Virality"

// ViralityProjects returns the projects whose HackUTD track is Virality (ignoring case and spaces).
// Opt-In Prizes are not checked: only the HackUTD track question counts.
func ViralityProjects(projects []*models.Project) []*models.Project {
	virality := make([]*models.Project, 0)
	for _, project := range projects {
		if strings.EqualFold(strings.TrimSpace(project.HackUTDTrack), ViralityTrack) {
			virality = append(virality, project)
		}
	}
	return virality
}

// CreateViralityExportZip creates the Virality export from the projects that applied to the
// Virality HackUTD track, sorted by table:
//   - virality.pdf: a printable list of table numbers and project names (see CreatePrizePDF)
//   - virality.csv: table, name, Devpost and Try It Out links, then each team member's name and
//     email (see CreateChallengeProjectCSV, contains PII)
func CreateViralityExportZip(projects []*models.Project) ([]byte, error) {
	virality := ViralityProjects(projects)

	zipBuffer := &bytes.Buffer{}
	w := zip.NewWriter(zipBuffer)
	files := []struct {
		name string
		data []byte
	}{
		{"virality.pdf", CreatePrizePDF(ViralityTrack, virality)},
		{"virality.csv", CreateChallengeProjectCSV(virality, true)},
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
