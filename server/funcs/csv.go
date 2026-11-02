package funcs

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"server/database"
	"server/models"
	"server/util"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/exp/slices"
)

// Read CSV file and return a slice of judge structs
func ParseJudgeCSV(content string, hasHeader bool) ([]*models.Judge, error) {
	r := csv.NewReader(strings.NewReader(content))

	// Empty CSV file
	if content == "" {
		return []*models.Judge{}, nil
	}

	// If the CSV file has a header, skip the first line
	if hasHeader {
		r.Read()
	}

	// Read the CSV file, looping through each record
	var judges []*models.Judge
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// Make sure the record has 2+ elements (name, email, tracks [optional], notes [optional])
		if len(record) < 2 {
			return nil, fmt.Errorf("record does not contain 2-3 (name, email, tracks [optional] notes [optional]) elements: '%s'", strings.Join(record, ","))
		}

		// Assign notes and tracks
		track := ""
		notes := ""
		if len(record) >= 3 {
			track = record[2]
		}
		if len(record) >= 4 {
			notes = record[3]
		}

		// Add judge to slice
		judges = append(judges, models.NewJudge(record[0], record[1], track, notes, -1))
	}

	return judges, nil
}

// Read CSV file and return a slice of project structs
func ParseProjectCsv(content string, hasHeader bool, db *mongo.Database) ([]*models.Project, error) {
	r := csv.NewReader(strings.NewReader(content))

	// Empty CSV file
	if content == "" {
		return []*models.Project{}, nil
	}

	// If the CSV file has a header, skip the first line
	if hasHeader {
		r.Read()
	}

	// Get the starting table number
	tableNum, err := database.GetMaxTableNum(db, context.Background())
	if err != nil {
		return nil, err
	}

	// Get options from the database
	options, err := database.GetOptions(db, context.Background())
	if err != nil {
		return nil, err
	}

	// Read the CSV file, looping through each record
	var projects []*models.Project
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// Make sure the record has at least 3 elements (name, description, URL)
		if len(record) < 3 {
			return nil, fmt.Errorf("record contains less than 3 elements: '%s'", strings.Join(record, ","))
		}

		// Get the challenge list
		challengeList := []string{}
		if len(record) > 5 && record[5] != "" {
			challengeList = strings.Split(record[5], ",")
		}
		for i := range challengeList {
			challengeList[i] = strings.TrimSpace(challengeList[i])
		}

		// If the challenge list contains the ignore track, skip the project
		ignore := false
		for _, ignoreTrack := range options.IgnoreTracks {
			if slices.Contains(challengeList, ignoreTrack) {
				ignore = true
				break
			}
		}
		if ignore {
			continue
		}

		// Optional fields
		var tryLink string
		if len(record) > 3 && record[3] != "" {
			tryLink = record[3]
		}
		var videoLink string
		if len(record) > 4 && record[4] != "" {
			videoLink = record[4]
		}

		// Increment the table number
		tableNum++

		// Add project to slice
		projects = append(projects, models.NewProject(record[0], tableNum, util.GroupFromTable(options, tableNum), record[1], record[2], tryLink, videoLink, challengeList))
	}

	return projects, nil
}

// Column positions in the Devpost project export (2025 format)
const (
	devpostColTitle       = 0
	devpostColUrl         = 1
	devpostColStatus      = 2
	devpostColDescription = 8
	devpostColTryLink     = 9
	devpostColVideoLink   = 10
	devpostColChallenges  = 13
)

// Expected (normalized) header names at the columns we read, used to detect format changes
var devpostExpectedHeaders = map[int]string{
	devpostColTitle:       "project title",
	devpostColUrl:         "submission url",
	devpostColStatus:      "project status",
	devpostColDescription: "about the project",
	devpostColTryLink:     "try it out links",
	devpostColVideoLink:   "video demo link",
	devpostColChallenges:  "opt-in prizes",
}

// normalizeDevpostHeader lowercases a header and strips the BOM, quotes, and surrounding whitespace
func normalizeDevpostHeader(h string) string {
	h = strings.TrimPrefix(h, "\xef\xbb\xbf")
	h = strings.ReplaceAll(h, "\"", "")
	return strings.ToLower(strings.TrimSpace(h))
}

// checkDevpostHeader verifies the header row matches the column positions we read,
// so a change in Devpost's export format fails loudly instead of importing garbage
func checkDevpostHeader(header []string) error {
	for col, expected := range devpostExpectedHeaders {
		if col >= len(header) {
			return fmt.Errorf("devpost csv header has %d columns, expected '%s' at column %d", len(header), expected, col)
		}
		if got := normalizeDevpostHeader(header[col]); got != expected {
			return fmt.Errorf("devpost csv column %d is '%s', expected '%s' (devpost export format may have changed)", col, header[col], expected)
		}
	}
	return nil
}

// parseDevpostChallenges splits the Opt-In Prizes cell into a list, trimming entries and dropping empty ones
func parseDevpostChallenges(cell string) []string {
	challengeList := []string{}
	for _, challenge := range strings.Split(cell, ",") {
		challenge = strings.TrimSpace(challenge)
		if challenge != "" {
			challengeList = append(challengeList, challenge)
		}
	}
	return challengeList
}

// devpostTeamCols holds the column indexes of one person's name and email in the Devpost PII export
// (-1 when the column is missing)
type devpostTeamCols struct {
	first, last, email int
}

// findDevpostTeamColumns finds the submitter and team member columns by header name, since they come
// after the hackathon's custom questions and move between exports. Returns nothing for the non-PII export.
//
// Devpost only names Team Member 1's columns: further members are unlabeled (first, last, email)
// cells past the end of the header. When the last named member block ends the header, extraFrom is
// the index those unlabeled blocks start at (otherwise -1).
func findDevpostTeamColumns(header []string) (cols []devpostTeamCols, extraFrom int) {
	index := map[string]int{}
	for i, h := range header {
		name := normalizeDevpostHeader(h)
		if _, ok := index[name]; !ok {
			index[name] = i
		}
	}
	col := func(name string) int {
		if i, ok := index[name]; ok {
			return i
		}
		return -1
	}

	cols = []devpostTeamCols{}
	extraFrom = -1
	if first := col("submitter first name"); first != -1 {
		cols = append(cols, devpostTeamCols{first, col("submitter last name"), col("submitter email")})
	}
	for n := 1; ; n++ {
		prefix := fmt.Sprintf("team member %d ", n)
		first := col(prefix + "first name")
		if first == -1 {
			break
		}
		last, email := col(prefix+"last name"), col(prefix+"email")
		cols = append(cols, devpostTeamCols{first, last, email})
		if last == first+1 && email == first+2 && email == len(header)-1 {
			extraFrom = email + 1
		} else {
			extraFrom = -1
		}
	}
	return cols, extraFrom
}

// parseDevpostTeam reads the submitter and team members of a row (including the unlabeled member
// cells from extraFrom on, see findDevpostTeamColumns), trimming names and skipping empty or
// repeated people (rows only have cells for the members the team has)
func parseDevpostTeam(record []string, cols []devpostTeamCols, extraFrom int) []models.TeamMember {
	cell := func(i int) string {
		if i < 0 || i >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[i])
	}

	// Copy before adding this row's unlabeled member blocks, since cols is shared across rows
	if extraFrom != -1 {
		cols = append([]devpostTeamCols{}, cols...)
		for i := extraFrom; i < len(record); i += 3 {
			cols = append(cols, devpostTeamCols{i, i + 1, i + 2})
		}
	}

	members := []models.TeamMember{}
	seen := map[string]bool{}
	for _, c := range cols {
		name := strings.TrimSpace(cell(c.first) + " " + cell(c.last))
		email := cell(c.email)
		if name == "" && email == "" {
			continue
		}
		key := strings.ToLower(email)
		if key == "" {
			key = strings.ToLower(name)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		members = append(members, models.TeamMember{Name: name, Email: email})
	}
	return members
}

// Generate a workable CSV for Jury based on the output CSV from Devpost
// Columns (by position, verified against the header row):
//  0. Project Title - title
//  1. Submission Url - url
//  2. Project Status - Draft or Submitted (ignore drafts)
//  3. Judging Status - ignore
//  4. Highest Step Completed - ignore
//  5. Project Created At - ignore
//  6. Project Submitted At - ignore
//  7. Submission Last Submitted At - ignore
//  8. About The Project - description
//  9. "Try it out" Links - try_link
//  10. Video Demo Link - video_link
//  11. Image Gallery URLs - ignore
//  12. Attached File S3 Key - ignore
//  13. Opt-In Prizes - challenge_list
//  14. Built With - ignore
//  15. (and remaining columns) Notes, team info, and custom questions - ignore, except
//     Submitter / Team Member N First Name, Last Name, Email (PII export only, found by header name)
func ParseDevpostCSV(content string, db *mongo.Database) ([]*models.Project, error) {
	r := csv.NewReader(strings.NewReader(content))
	r.FieldsPerRecord = -1 // PII export rows end after the team's last member, so lengths vary

	// Empty CSV file
	if content == "" {
		return []*models.Project{}, nil
	}

	// Read and validate the header row
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("could not read devpost csv header: %s", err.Error())
	}
	if err := checkDevpostHeader(header); err != nil {
		return nil, err
	}
	teamCols, teamExtraFrom := findDevpostTeamColumns(header)

	// Get the starting table number
	tableNum, err := database.GetMaxTableNum(db, context.Background())
	if err != nil {
		return nil, err
	}

	// Get options from the database
	options, err := database.GetOptions(db, context.Background())
	if err != nil {
		return nil, err
	}

	// Read the CSV file, looping through each record
	var projects []*models.Project
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// Make sure the record has 14 or more elements (see above)
		if len(record) <= devpostColChallenges {
			return nil, fmt.Errorf("record does not contain %d or more elements (invalid devpost csv): '%s'", devpostColChallenges+1, record[devpostColTitle])
		}

		// If the project is a Draft, skip it
		if record[devpostColStatus] == "Draft" {
			continue
		}

		// Split challenge list into a slice and trim them
		challengeList := parseDevpostChallenges(record[devpostColChallenges])

		// If the challenge list contains the ignore track, skip the project
		ignore := false
		for _, ignoreTrack := range options.IgnoreTracks {
			if slices.Contains(challengeList, ignoreTrack) {
				ignore = true
				break
			}
		}
		if ignore {
			continue
		}

		// Increment the table number
		tableNum++

		// Add project to slice
		projects = append(projects, models.NewProject(
			record[devpostColTitle],
			tableNum,
			util.GroupFromTable(options, tableNum),
			record[devpostColDescription],
			record[devpostColUrl],
			record[devpostColTryLink],
			record[devpostColVideoLink],
			challengeList,
		))
		projects[len(projects)-1].TeamMembers = parseDevpostTeam(record, teamCols, teamExtraFrom)
	}

	return projects, nil
}

// AddCSVData adds a CSV file to the response
func AddCsvData(name string, content []byte, ctx *gin.Context) {
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", name))
	ctx.Header("Content-Type", "text/csv")
	ctx.Data(http.StatusOK, "text/csv", content)
}

// AddZipFile adds a zip file to the response
func AddZipFile(name string, content []byte, ctx *gin.Context) {
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.zip", name))
	ctx.Header("Content-Type", "application/octet-stream")
	ctx.Data(http.StatusOK, "application/octet-stream", content)
}

// Create a CSV file from a list of judges
func CreateJudgeCSV(judges []*models.Judge) []byte {
	csvBuffer := &bytes.Buffer{}

	// Create a new CSV writer
	w := csv.NewWriter(csvBuffer)

	// Write the header
	// TODO: Add judge rankings to output
	w.Write([]string{"Name", "Email", "Notes", "Code", "Active", "ReadWelcome", "Seen", "LastActivity"})

	// Write each judge
	for _, judge := range judges {
		w.Write([]string{judge.Name, judge.Email, judge.Notes, judge.Code, fmt.Sprintf("%t", judge.Active), fmt.Sprintf("%t", judge.ReadWelcome), fmt.Sprintf("%d", judge.Seen), fmt.Sprintf("%d", judge.LastActivity)})
	}

	// Flush the writer
	w.Flush()

	return csvBuffer.Bytes()
}

// Create a CSV file from the judges but only the rankings
func CreateJudgeRankingCSV(judges []*models.Judge) []byte {
	csvBuffer := &bytes.Buffer{}

	// Create a new CSV writer
	w := csv.NewWriter(csvBuffer)

	// Write the header
	w.Write([]string{"Name", "Code", "Ranked", "Unranked"})

	// Write each judge
	for _, judge := range judges {
		// Don't include if their rankings are empty :/
		if len(judge.Rankings) == 0 {
			continue
		}

		// Create a list of all ranked projects (just their location)
		ranked := make([]int64, 0, len(judge.Rankings))
		for _, projId := range judge.Rankings {
			idx := util.IndexFunc(judge.SeenProjects, func(p models.JudgedProject) bool {
				return p.ProjectId == projId
			})
			if idx == -1 {
				continue
			}

			ranked = append(ranked, judge.SeenProjects[idx].Location)
		}

		// Create a list of all unranked projects (filter using ranked projects)
		unranked := make([]int64, 0, len(judge.SeenProjects)-len(judge.Rankings))
		for _, proj := range judge.SeenProjects {
			if util.ContainsFunc(ranked, func(table int64) bool { return table == proj.Location }) {
				unranked = append(unranked, proj.Location)
			}
		}

		// Convert arrays to strings
		rankedStr := util.IntToString(ranked)
		unrankedStr := util.IntToString(unranked)

		// Write line to CSV
		w.Write([]string{judge.Name, judge.Code, strings.Join(rankedStr, ","), strings.Join(unrankedStr, ",")})
	}

	// Flush the writer
	w.Flush()

	return csvBuffer.Bytes()
}

// Create a CSV file from a list of projects
func CreateProjectCSV(projects []*models.Project) []byte {
	csvBuffer := &bytes.Buffer{}

	// Create a new CSV writer
	w := csv.NewWriter(csvBuffer)

	// Write the header
	w.Write([]string{"Name", "Table", "Description", "URL", "TryLink", "VideoLink", "ChallengeList", "Seen", "Active", "LastActivity"})

	// Write each project
	for _, project := range projects {
		w.Write([]string{project.Name, fmt.Sprintf("Table %d", project.Location), project.Description, project.Url, project.TryLink, project.VideoLink, strings.Join(project.ChallengeList, ","), fmt.Sprintf("%d", project.Seen), fmt.Sprintf("%t", project.Active), fmt.Sprintf("%d", project.LastActivity)})
	}

	// Flush the writer
	w.Flush()

	return csvBuffer.Bytes()
}

// CreateChallengeProjectCSV creates a printable CSV of projects for a single challenge,
// with only table, name, and project links (sized to print on A4 landscape), sorted by table number
func CreateChallengeProjectCSV(projects []*models.Project) []byte {
	csvBuffer := &bytes.Buffer{}

	// Create a new CSV writer
	w := csv.NewWriter(csvBuffer)

	// Write the header
	w.Write([]string{"Table", "Name", "Devpost", "Try It Out"})

	// Sort a copy of the projects by table number
	sorted := make([]*models.Project, len(projects))
	copy(sorted, projects)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Location < sorted[j].Location })

	// Write each project
	for _, project := range sorted {
		w.Write([]string{fmt.Sprintf("%d", project.Location), project.Name, project.Url, project.TryLink})
	}

	// Flush the writer
	w.Flush()

	return csvBuffer.Bytes()
}

// CreateParticipantsCSV creates a CSV with one row per project and every team member's name and
// email, for emailing teams their table, sorted by table number. Contains PII: admin only.
func CreateParticipantsCSV(projects []*models.Project) []byte {
	csvBuffer := &bytes.Buffer{}

	// Create a new CSV writer
	w := csv.NewWriter(csvBuffer)

	// Sort a copy of the projects by table number
	sorted := make([]*models.Project, len(projects))
	copy(sorted, projects)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Location < sorted[j].Location })

	// One name and one email column per member, sized to the biggest team
	maxMembers := 0
	for _, project := range sorted {
		maxMembers = max(maxMembers, len(project.TeamMembers))
	}

	// Write the header
	header := []string{"Table", "Project", "Devpost"}
	for i := 1; i <= maxMembers; i++ {
		header = append(header, fmt.Sprintf("Member %d Name", i))
	}
	for i := 1; i <= maxMembers; i++ {
		header = append(header, fmt.Sprintf("Member %d Email", i))
	}
	w.Write(header)

	// Write each project, leaving the extra member columns empty for smaller teams
	for _, project := range sorted {
		names := make([]string, maxMembers)
		emails := make([]string, maxMembers)
		for i, member := range project.TeamMembers {
			names[i] = member.Name
			emails[i] = member.Email
		}
		row := []string{fmt.Sprintf("%d", project.Location), project.Name, project.Url}
		row = append(row, names...)
		row = append(row, emails...)
		w.Write(row)
	}

	// Flush the writer
	w.Flush()

	return csvBuffer.Bytes()
}

// CreateProjectChallengeZip creates a zip file with a CSV for each challenge
func CreateProjectChallengeZip(projects []*models.Project) ([]byte, error) {
	csvList := [][]byte{}

	// Get list of challenges
	challengeList := []string{}
	for _, project := range projects {
		for _, challenge := range project.ChallengeList {
			if !contains(challengeList, challenge) {
				challengeList = append(challengeList, challenge)
			}
		}
	}

	// Create a CSV for each challenge
	for _, challenge := range challengeList {
		currChallengeProjects := []*models.Project{}
		for _, project := range projects {
			if contains(project.ChallengeList, challenge) {
				currChallengeProjects = append(currChallengeProjects, project)
			}
		}

		// Create CSV for the challenge
		csv := CreateChallengeProjectCSV(currChallengeProjects)
		csvList = append(csvList, csv)
	}

	// Create buffer for zip file
	zipBuffer := &bytes.Buffer{}

	// Create a new zip writer
	w := zip.NewWriter(zipBuffer)

	// Write each CSV to the zip file
	for i, csv := range csvList {
		f, err := w.Create(fmt.Sprintf("%s.csv", challengeList[i]))
		if err != nil {
			return nil, err
		}

		_, err = f.Write(csv)
		if err != nil {
			return nil, err
		}
	}

	// Close the zip writer
	err := w.Close()
	if err != nil {
		return nil, err
	}

	return zipBuffer.Bytes(), nil
}

// contains checks if a string is in a list of strings
func contains(list []string, str string) bool {
	for _, s := range list {
		if s == str {
			return true
		}
	}
	return false
}
