package tests

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"tests/util"
)

// --- Track Judging Tests ---

// TrackTestSetup enables track judging with the "Design" and "Best Use of AI" tracks (keeping
// general judging) and makes sure judging is running
func TrackTestSetup(context *util.Context) util.Result {
	setRes := util.PostRequest(context.Logger, "/admin/options", util.H{
		"judge_tracks":            true,
		"multi_group":             false,
		"disable_general_judging": false,
	}, util.AdminAuth())
	if !util.IsOk(setRes) {
		return util.NewResult(false, "Failed to enable track judging: "+setRes)
	}

	tracksRes := util.PostRequest(context.Logger, "/admin/tracks", util.H{
		"tracks": []string{"Design", "Best Use of AI"},
	}, util.AdminAuth())
	if !util.IsOk(tracksRes) {
		return util.NewResult(false, "Failed to set tracks: "+tracksRes)
	}

	if res := setTrackViews(context, []int{3, 3}); !res.Success {
		return res
	}

	unpauseRes := util.PostRequest(context.Logger, "/admin/clock/unpause", nil, util.AdminAuth())
	if !util.IsOk(unpauseRes) {
		return util.NewResult(false, "Failed to unpause clock: "+unpauseRes)
	}

	return util.ResultOk()
}

// TrackJudgesRankAndGetTrackScores verifies that track judges only get projects in their track
// (matched ignoring case), and that their rankings produce track_scores without affecting the general score
func TrackJudgesRankAndGetTrackScores(context *util.Context) util.Result {
	setGeneralJudging(context, true)

	resetRes := util.PostRequest(context.Logger, "/admin/reset", util.H{"type": "projects"}, util.AdminAuth())
	if !util.IsOk(resetRes) {
		return util.NewResult(false, "Failed to reset projects: "+resetRes)
	}

	// 3 projects in the track (lowercase prize name) and 1 project not in the track
	trackNames := map[string]bool{}
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("Track Rank Project %d", i)
		trackNames[name] = true
		addTestProject(context, name, "design, Other Prize")
	}
	addTestProject(context, "Track Rank Non-Track Project", "Other Prize")

	// Track is matched ignoring case and stored with the options' casing
	token, result := createTrackJudge(context, "track_rank@example.com", "Track Rank Judge", "DESIGN")
	if !result.Success {
		return result
	}
	auth := util.JudgeAuth(token)

	// Judge all projects available to the track judge
	var seenIDs []string
	for i := 0; i < 4; i++ {
		nextRes := util.PostRequest(context.Logger, "/judge/next", nil, auth)
		projectID := util.ExtractString(nextRes, "project_id")
		if projectID == "" {
			break
		}
		seenIDs = append(seenIDs, projectID)
		finishRes := util.PostRequest(context.Logger, "/judge/finish", util.H{"notes": "", "starred": false}, auth)
		if !util.IsOk(finishRes) {
			return util.NewResult(false, "POST /judge/finish failed: "+finishRes)
		}
	}
	if len(seenIDs) != 3 {
		return util.NewResult(false, fmt.Sprintf("Track judge should see exactly the 3 track projects, saw %d", len(seenIDs)))
	}

	// Rank the seen projects in order
	rankRes := util.PostRequest(context.Logger, "/judge/rank", util.H{"ranking": seenIDs}, auth)
	if !util.IsOk(rankRes) {
		return util.NewResult(false, "POST /judge/rank failed: "+rankRes)
	}

	// Check scores
	var projects []struct {
		Id          string           `json:"id"`
		Name        string           `json:"name"`
		Score       int64            `json:"score"`
		TrackScores map[string]int64 `json:"track_scores"`
	}
	listRes := util.GetRequest(context.Logger, "/project/list", util.AdminAuth())
	if err := json.Unmarshal([]byte(listRes), &projects); err != nil {
		return util.NewResult(false, "Could not parse project list: "+err.Error())
	}

	byId := map[string]int{}
	for i, p := range projects {
		byId[p.Id] = i
		if !trackNames[p.Name] && p.Name != "Track Rank Non-Track Project" {
			continue
		}
		if p.Score != 0 {
			return util.NewResult(false, fmt.Sprintf("Track judge rankings should not affect general score, '%s' has score %d", p.Name, p.Score))
		}
	}

	// Copeland over 3 ranked projects: 2, 0, -2
	want := []int64{2, 0, -2}
	for i, id := range seenIDs {
		got := projects[byId[id]].TrackScores["Design"]
		if got != want[i] {
			return util.NewResult(false, fmt.Sprintf("Expected track score %d for ranked project %d, got %d", want[i], i+1, got))
		}
	}

	return util.ResultOk()
}

// GeneralJudgingDisabledBlocksGeneralJudges verifies that when general judging is turned off,
// new general judges are rejected and existing general judges don't get projects
func GeneralJudgingDisabledBlocksGeneralJudges(context *util.Context) util.Result {
	setGeneralJudging(context, true)

	resetRes := util.PostRequest(context.Logger, "/admin/reset", util.H{"type": "projects"}, util.AdminAuth())
	if !util.IsOk(resetRes) {
		return util.NewResult(false, "Failed to reset projects: "+resetRes)
	}
	addTestProject(context, "General Disabled Project", "")

	// Create a general judge while general judging is still on
	token, result := createNamedJudge(context, "general_disabled@example.com", "General Disabled Judge")
	if !result.Success {
		return result
	}

	setGeneralJudging(context, false)
	defer setGeneralJudging(context, true)

	// Existing general judge gets no project
	nextRes := util.PostRequest(context.Logger, "/judge/next", nil, util.JudgeAuth(token))
	if util.ExtractString(nextRes, "project_id") != "" {
		return util.NewResult(false, "General judge should not get a project when general judging is disabled: "+nextRes)
	}

	// New general judges are rejected
	status, _ := util.PostRequestWithStatus(context.Logger, "/judge/new", util.H{
		"name":    "Rejected General Judge",
		"email":   "general_rejected@example.com",
		"track":   "",
		"notes":   "",
		"no_send": true,
	}, util.AdminAuth())
	if res := util.AssertStatus(status, 400, "Adding a general judge should be rejected when general judging is disabled"); !res.Success {
		return res
	}

	// Judges with unknown tracks are rejected
	status, _ = util.PostRequestWithStatus(context.Logger, "/judge/new", util.H{
		"name":    "Rejected Track Judge",
		"email":   "track_rejected@example.com",
		"track":   "Not A Track",
		"notes":   "",
		"no_send": true,
	}, util.AdminAuth())
	return util.AssertStatus(status, 400, "Adding a judge with an unknown track should be rejected")
}

// TrackJudgingDisabledRejectsTracks verifies that when track judging is turned off,
// judges can't be added with a track and track QR codes are rejected
func TrackJudgingDisabledRejectsTracks(context *util.Context) util.Result {
	util.PostRequest(context.Logger, "/admin/options", util.H{"judge_tracks": false}, util.AdminAuth())
	defer util.PostRequest(context.Logger, "/admin/options", util.H{"judge_tracks": true}, util.AdminAuth())

	// Judges with a track are rejected
	status, _ := util.PostRequestWithStatus(context.Logger, "/judge/new", util.H{
		"name":    "Tracks Off Judge",
		"email":   "tracks_off@example.com",
		"track":   "Design",
		"notes":   "",
		"no_send": true,
	}, util.AdminAuth())
	if res := util.AssertStatus(status, 400, "Adding a track judge should be rejected when track judging is disabled"); !res.Success {
		return res
	}

	// Track QR codes are rejected when scanned
	status, _ = util.PostRequestWithStatus(context.Logger, "/qr/check/Design", util.H{"code": "anything"}, util.DefaultAuth())
	return util.AssertStatus(status, 400, "Track QR check should be rejected when track judging is disabled")
}

// setGeneralJudging turns general judging on or off (only matters when track judging is on)
func setGeneralJudging(context *util.Context, enabled bool) {
	util.PostRequest(context.Logger, "/admin/options", util.H{
		"disable_general_judging": !enabled,
	}, util.AdminAuth())
}

// addTestProject adds a project with the given challenge list
func addTestProject(context *util.Context, name string, challengeList string) {
	util.PostRequest(context.Logger, "/project/new", util.H{
		"name":           name,
		"description":    "Track judging test project",
		"url":            "https://example.com",
		"try_link":       "",
		"video_link":     "",
		"challenge_list": challengeList,
	}, util.AdminAuth())
}

// createTrackJudge creates a judge in the given track, logs them in, and returns their token
func createTrackJudge(context *util.Context, email string, name string, track string) (string, util.Result) {
	addRes := util.PostRequest(context.Logger, "/judge/new", util.H{
		"name":    name,
		"email":   email,
		"track":   track,
		"notes":   "",
		"no_send": true,
	}, util.AdminAuth())
	if !util.IsOk(addRes) {
		return "", util.NewResult(false, "Failed to create track judge '"+name+"': "+addRes)
	}

	listRes := util.GetRequest(context.Logger, "/judge/list", util.AdminAuth())
	code := extractJudgeCode(listRes, email)
	if code == "" {
		return "", util.NewResult(false, "Could not find code for judge: "+email)
	}

	loginRes := util.PostRequest(context.Logger, "/judge/login", util.H{"code": code}, util.DefaultAuth())
	token := util.ExtractString(loginRes, "token")
	if token == "" {
		return "", util.NewResult(false, "Judge login did not return a token for: "+email)
	}

	return token, util.ResultOk()
}

// TracksAndGeneralRunTogether verifies the "tracks + general" mode end to end: general judges still
// see track projects, each judge type only updates its own seen counts, and general and track
// rankings/stars are scored separately
func TracksAndGeneralRunTogether(context *util.Context) util.Result {
	if res := resetProjects(context); !res.Success {
		return res
	}
	addTestProject(context, "Mixed Design 1", "Design")
	addTestProject(context, "Mixed Design 2", "Design")
	addTestProject(context, "Mixed General Only", "")
	ids, res := getProjectIDs(context, "Mixed Design 1", "Mixed Design 2", "Mixed General Only")
	if !res.Success {
		return res
	}
	d1, d2, general := ids[0], ids[1], ids[2]

	generalToken, res := createNamedJudge(context, "mixed_general@example.com", "Mixed General Judge")
	if !res.Success {
		return res
	}
	trackToken, res := createTrackJudge(context, "mixed_track@example.com", "Mixed Track Judge", "Design")
	if !res.Success {
		return res
	}

	// General judge sees every project, including track projects
	generalSeen, res := judgeUntilDone(context, util.JudgeAuth(generalToken), 5, general)
	if !res.Success {
		return res
	}
	if !sameIDs(generalSeen, d1, d2, general) {
		return util.NewResult(false, fmt.Sprintf("General judge should see all 3 projects, saw %v", generalSeen))
	}

	// Track judge only sees the track's projects
	trackSeen, res := judgeUntilDone(context, util.JudgeAuth(trackToken), 5, d1)
	if !res.Success {
		return res
	}
	if !sameIDs(trackSeen, d1, d2) {
		return util.NewResult(false, fmt.Sprintf("Track judge should only see the 2 Design projects, saw %v", trackSeen))
	}

	// Rank: general [general, d1, d2], track [d2, d1]
	if rankRes := util.PostRequest(context.Logger, "/judge/rank", util.H{"ranking": []string{general, d1, d2}}, util.JudgeAuth(generalToken)); !util.IsOk(rankRes) {
		return util.NewResult(false, "General judge ranking failed: "+rankRes)
	}
	if rankRes := util.PostRequest(context.Logger, "/judge/rank", util.H{"ranking": []string{d2, d1}}, util.JudgeAuth(trackToken)); !util.IsOk(rankRes) {
		return util.NewResult(false, "Track judge ranking failed: "+rankRes)
	}

	projects, res := getTestProjects(context)
	if !res.Success {
		return res
	}

	// Seen counts: general views only update seen, track views only update track_seen
	checks := []struct {
		id        string
		seen      int64
		trackSeen int64
		score     int64
		trackScr  int64
		stars     int64
		trackStar int64
	}{
		{d1, 1, 1, 0, -1, 0, 1},
		{d2, 1, 1, -2, 1, 0, 0},
		{general, 1, 0, 2, 0, 1, 0},
	}
	for _, c := range checks {
		p := projects[c.id]
		if p.Seen != c.seen || p.TrackSeen["Design"] != c.trackSeen {
			return util.NewResult(false, fmt.Sprintf("'%s' should have seen=%d, track_seen=%d, got seen=%d, track_seen=%d", p.Name, c.seen, c.trackSeen, p.Seen, p.TrackSeen["Design"]))
		}
		if p.Score != c.score || p.TrackScores["Design"] != c.trackScr {
			return util.NewResult(false, fmt.Sprintf("'%s' should have score=%d, track score=%d, got score=%d, track score=%d", p.Name, c.score, c.trackScr, p.Score, p.TrackScores["Design"]))
		}
		if p.Stars != c.stars || p.TrackStars["Design"] != c.trackStar {
			return util.NewResult(false, fmt.Sprintf("'%s' should have stars=%d, track stars=%d, got stars=%d, track stars=%d", p.Name, c.stars, c.trackStar, p.Stars, p.TrackStars["Design"]))
		}
	}

	return util.ResultOk()
}

// TrackViewsCapIsEnforced verifies that track views are a cap: a project at the cap (or being
// judged by a judge in the same track when one view away from the cap) is not given out again
func TrackViewsCapIsEnforced(context *util.Context) util.Result {
	if res := setTrackViews(context, []int{1, 3}); !res.Success {
		return res
	}
	defer setTrackViews(context, []int{3, 3})

	if res := resetProjects(context); !res.Success {
		return res
	}
	addTestProject(context, "Cap Project", "Design")

	var auths []string
	for i := 1; i <= 3; i++ {
		token, res := createTrackJudge(context, fmt.Sprintf("cap_%d@example.com", i), fmt.Sprintf("Cap Judge %d", i), "Design")
		if !res.Success {
			return res
		}
		auths = append(auths, util.JudgeAuth(token))
	}

	// Judge 1 gets the project
	if id := util.ExtractString(util.PostRequest(context.Logger, "/judge/next", nil, auths[0]), "project_id"); id == "" {
		return util.NewResult(false, "First track judge should get the project")
	}

	// Judge 2 doesn't, since judge 1 viewing it will reach the cap
	if id := util.ExtractString(util.PostRequest(context.Logger, "/judge/next", nil, auths[1]), "project_id"); id != "" {
		return util.NewResult(false, "Second track judge should not get a project that is about to reach the cap")
	}

	// After judge 1 finishes, judge 3 doesn't get it either
	if finishRes := util.PostRequest(context.Logger, "/judge/finish", util.H{"notes": "", "starred": false}, auths[0]); !util.IsOk(finishRes) {
		return util.NewResult(false, "POST /judge/finish failed: "+finishRes)
	}
	if id := util.ExtractString(util.PostRequest(context.Logger, "/judge/next", nil, auths[2]), "project_id"); id != "" {
		return util.NewResult(false, "Track judge should not get a project that has reached the cap")
	}

	projects, res := getTestProjects(context)
	if !res.Success {
		return res
	}
	for _, p := range projects {
		if p.Name == "Cap Project" && p.TrackSeen["Design"] != 1 {
			return util.NewResult(false, fmt.Sprintf("Cap Project should have 1 Design view, got %d", p.TrackSeen["Design"]))
		}
	}
	return util.ResultOk()
}

// TracksStaySeparate verifies that judges in different tracks only see their own track's projects
// (a project in both tracks is seen by both), and that each track is scored separately
func TracksStaySeparate(context *util.Context) util.Result {
	if res := resetProjects(context); !res.Success {
		return res
	}
	addTestProject(context, "Sep Design Only", "Design")
	addTestProject(context, "Sep AI Only", "best use of ai")
	addTestProject(context, "Sep Both", "Design, Best Use of AI")
	ids, res := getProjectIDs(context, "Sep Design Only", "Sep AI Only", "Sep Both")
	if !res.Success {
		return res
	}
	designOnly, aiOnly, both := ids[0], ids[1], ids[2]

	designToken, res := createTrackJudge(context, "sep_design@example.com", "Sep Design Judge", "Design")
	if !res.Success {
		return res
	}
	aiToken, res := createTrackJudge(context, "sep_ai@example.com", "Sep AI Judge", "best use of AI")
	if !res.Success {
		return res
	}

	designSeen, res := judgeUntilDone(context, util.JudgeAuth(designToken), 5)
	if !res.Success {
		return res
	}
	if !sameIDs(designSeen, designOnly, both) {
		return util.NewResult(false, fmt.Sprintf("Design judge should see only Design projects, saw %v", designSeen))
	}
	aiSeen, res := judgeUntilDone(context, util.JudgeAuth(aiToken), 5)
	if !res.Success {
		return res
	}
	if !sameIDs(aiSeen, aiOnly, both) {
		return util.NewResult(false, fmt.Sprintf("AI judge should see only AI projects, saw %v", aiSeen))
	}

	if rankRes := util.PostRequest(context.Logger, "/judge/rank", util.H{"ranking": []string{both, designOnly}}, util.JudgeAuth(designToken)); !util.IsOk(rankRes) {
		return util.NewResult(false, "Design judge ranking failed: "+rankRes)
	}
	if rankRes := util.PostRequest(context.Logger, "/judge/rank", util.H{"ranking": []string{aiOnly, both}}, util.JudgeAuth(aiToken)); !util.IsOk(rankRes) {
		return util.NewResult(false, "AI judge ranking failed: "+rankRes)
	}

	projects, res := getTestProjects(context)
	if !res.Success {
		return res
	}
	checks := []struct {
		id                 string
		design, ai         int64
		designSeen, aiSeen int64
	}{
		{designOnly, -1, 0, 1, 0},
		{aiOnly, 0, 1, 0, 1},
		{both, 1, -1, 1, 1},
	}
	for _, c := range checks {
		p := projects[c.id]
		if p.TrackScores["Design"] != c.design || p.TrackScores["Best Use of AI"] != c.ai {
			return util.NewResult(false, fmt.Sprintf("'%s' should have track scores Design=%d AI=%d, got %v", p.Name, c.design, c.ai, p.TrackScores))
		}
		if p.TrackSeen["Design"] != c.designSeen || p.TrackSeen["Best Use of AI"] != c.aiSeen {
			return util.NewResult(false, fmt.Sprintf("'%s' should have track seen Design=%d AI=%d, got %v", p.Name, c.designSeen, c.aiSeen, p.TrackSeen))
		}
		if p.Seen != 0 || p.Score != 0 {
			return util.NewResult(false, fmt.Sprintf("Track judges should not affect general seen/score, '%s' has seen=%d score=%d", p.Name, p.Seen, p.Score))
		}
	}
	return util.ResultOk()
}

// QRSignupAssignsTrack verifies the QR self-signup flow for both general and track QR codes,
// and that a track's QR code only works for that track
func QRSignupAssignsTrack(context *util.Context) util.Result {
	trackPath := url.PathEscape("Best Use of AI")

	// Track QR code
	trackCode := util.ExtractString(util.PostRequest(context.Logger, "/admin/qr/"+trackPath, nil, util.AdminAuth()), "qr_code")
	if trackCode == "" {
		return util.NewResult(false, "Failed to generate track QR code")
	}
	if checkRes := util.PostRequest(context.Logger, "/qr/check/"+trackPath, util.H{"code": trackCode}, util.DefaultAuth()); !util.IsOk(checkRes) {
		return util.NewResult(false, "Track QR code should pass its own track's check: "+checkRes)
	}
	if checkRes := util.PostRequest(context.Logger, "/qr/check/Design", util.H{"code": trackCode}, util.DefaultAuth()); util.IsOk(checkRes) {
		return util.NewResult(false, "Track QR code should not pass another track's check")
	}
	if addRes := util.PostRequest(context.Logger, "/qr/add", util.H{
		"code":    trackCode,
		"name":    "QR Track Judge",
		"email":   "qr_track@example.com",
		"track":   "Best Use of AI",
		"no_send": true,
	}, util.DefaultAuth()); !util.IsOk(addRes) {
		return util.NewResult(false, "Adding a judge from a track QR code failed: "+addRes)
	}

	// General QR code
	generalCode := util.ExtractString(util.PostRequest(context.Logger, "/admin/qr", nil, util.AdminAuth()), "qr_code")
	if generalCode == "" {
		return util.NewResult(false, "Failed to generate general QR code")
	}
	if checkRes := util.PostRequest(context.Logger, "/qr/check", util.H{"code": generalCode}, util.DefaultAuth()); !util.IsOk(checkRes) {
		return util.NewResult(false, "General QR code should pass the general check: "+checkRes)
	}
	if addRes := util.PostRequest(context.Logger, "/qr/add", util.H{
		"code":    generalCode,
		"name":    "QR General Judge",
		"email":   "qr_general@example.com",
		"track":   "",
		"no_send": true,
	}, util.DefaultAuth()); !util.IsOk(addRes) {
		return util.NewResult(false, "Adding a judge from the general QR code failed: "+addRes)
	}

	// The general code can't be used to sign up as a track judge
	if addRes := util.PostRequest(context.Logger, "/qr/add", util.H{
		"code":    generalCode,
		"name":    "QR Sneaky Judge",
		"email":   "qr_sneaky@example.com",
		"track":   "Design",
		"no_send": true,
	}, util.DefaultAuth()); util.IsOk(addRes) {
		return util.NewResult(false, "General QR code should not add a track judge")
	}

	listRes := util.GetRequest(context.Logger, "/judge/list", util.AdminAuth())
	if track := findJudgeFieldByEmail(listRes, "qr_track@example.com", "track"); track != "Best Use of AI" {
		return util.NewResult(false, "QR track judge should have track 'Best Use of AI', got '"+track+"'")
	}
	if track := findJudgeFieldByEmail(listRes, "qr_general@example.com", "track"); track != "" {
		return util.NewResult(false, "QR general judge should have no track, got '"+track+"'")
	}
	if email := findJudgeFieldByEmail(listRes, "qr_sneaky@example.com", "email"); email != "" {
		return util.NewResult(false, "Judge should not be created from a mismatched QR code")
	}
	return util.ResultOk()
}

// JudgeCsvValidatesTracks verifies judge CSV upload: tracks are matched ignoring case and
// whitespace, unknown tracks reject the whole file, and blank tracks are rejected when general is off
func JudgeCsvValidatesTracks(context *util.Context) util.Result {
	setGeneralJudging(context, true)
	fields := map[string]string{"hasHeader": "false", "noSend": "true"}

	// Valid file: track in a different case, plus a general judge
	status, body := util.PostFileWithStatus(context.Logger, "/judge/csv", "csv", "judges.csv",
		"CSV Track Judge,csv_track@example.com,  design ,\nCSV General Judge,csv_general@example.com,,some notes\n",
		fields, util.AdminAuth())
	if status != 200 {
		return util.NewResult(false, fmt.Sprintf("Valid judge CSV should upload, got %d: %s", status, body))
	}
	listRes := util.GetRequest(context.Logger, "/judge/list", util.AdminAuth())
	if track := findJudgeFieldByEmail(listRes, "csv_track@example.com", "track"); track != "Design" {
		return util.NewResult(false, "CSV track judge should have track 'Design', got '"+track+"'")
	}
	if email := findJudgeFieldByEmail(listRes, "csv_general@example.com", "email"); email == "" {
		return util.NewResult(false, "CSV general judge should be added")
	}

	// Unknown track rejects the whole file (no partial import)
	status, _ = util.PostFileWithStatus(context.Logger, "/judge/csv", "csv", "judges.csv",
		"CSV Good Judge,csv_good@example.com,,\nCSV Bad Judge,csv_bad@example.com,Hardware,\n",
		fields, util.AdminAuth())
	if res := util.AssertStatus(status, 400, "Judge CSV with an unknown track should be rejected"); !res.Success {
		return res
	}
	listRes = util.GetRequest(context.Logger, "/judge/list", util.AdminAuth())
	if email := findJudgeFieldByEmail(listRes, "csv_good@example.com", "email"); email != "" {
		return util.NewResult(false, "No judges should be added from a rejected CSV")
	}

	// Blank track is rejected when general judging is off
	setGeneralJudging(context, false)
	defer setGeneralJudging(context, true)
	status, _ = util.PostFileWithStatus(context.Logger, "/judge/csv", "csv", "judges.csv",
		"CSV Blank Judge,csv_blank@example.com,,\n", fields, util.AdminAuth())
	return util.AssertStatus(status, 400, "Judge CSV with a blank track should be rejected when general judging is off")
}

// ConcurrentTrackJudgesGetProjects verifies that track judges asking for a project at the same
// time are all given a project without errors
func ConcurrentTrackJudgesGetProjects(context *util.Context) util.Result {
	if res := resetProjects(context); !res.Success {
		return res
	}
	for i := 1; i <= 8; i++ {
		addTestProject(context, fmt.Sprintf("Concurrent Track Project %d", i), "Design")
	}

	var auths []string
	for i := 1; i <= 6; i++ {
		token, res := createTrackJudge(context, fmt.Sprintf("concurrent_track_%d@example.com", i), fmt.Sprintf("Concurrent Track Judge %d", i), "Design")
		if !res.Success {
			return res
		}
		auths = append(auths, util.JudgeAuth(token))
	}

	_, res := concurrentNextRound(context, auths, nil)
	return res
}

// testProject is the subset of project fields checked by the track tests
type testProject struct {
	Id          string           `json:"id"`
	Name        string           `json:"name"`
	Seen        int64            `json:"seen"`
	TrackSeen   map[string]int64 `json:"track_seen"`
	Score       int64            `json:"score"`
	Stars       int64            `json:"stars"`
	TrackScores map[string]int64 `json:"track_scores"`
	TrackStars  map[string]int64 `json:"track_stars"`
}

// getTestProjects returns all projects keyed by ID
func getTestProjects(context *util.Context) (map[string]testProject, util.Result) {
	var projects []testProject
	listRes := util.GetRequest(context.Logger, "/project/list", util.AdminAuth())
	if err := json.Unmarshal([]byte(listRes), &projects); err != nil {
		return nil, util.NewResult(false, "Could not parse project list: "+err.Error())
	}
	byId := map[string]testProject{}
	for _, p := range projects {
		byId[p.Id] = p
	}
	return byId, util.ResultOk()
}

// getProjectIDs returns the IDs of the projects with the given names, in order
func getProjectIDs(context *util.Context, names ...string) ([]string, util.Result) {
	var ids []string
	for _, name := range names {
		id, res := findProjectIDByName(context, name)
		if !res.Success {
			return nil, res
		}
		ids = append(ids, id)
	}
	return ids, util.ResultOk()
}

// judgeUntilDone has a judge get and finish projects until there are none left (at most max),
// starring the given projects, and returns the IDs of the projects seen in order
func judgeUntilDone(context *util.Context, auth string, max int, starred ...string) ([]string, util.Result) {
	var seen []string
	for i := 0; i <= max; i++ {
		id := util.ExtractString(util.PostRequest(context.Logger, "/judge/next", nil, auth), "project_id")
		if id == "" {
			return seen, util.ResultOk()
		}
		if i == max {
			return nil, util.NewResult(false, fmt.Sprintf("Judge was given more than %d projects", max))
		}
		seen = append(seen, id)
		finishRes := util.PostRequest(context.Logger, "/judge/finish", util.H{"notes": "", "starred": slices.Contains(starred, id)}, auth)
		if !util.IsOk(finishRes) {
			return nil, util.NewResult(false, "POST /judge/finish failed: "+finishRes)
		}
	}
	return seen, util.ResultOk()
}

// sameIDs checks that got contains exactly the wanted IDs, in any order
func sameIDs(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for _, id := range want {
		if !slices.Contains(got, id) {
			return false
		}
	}
	return true
}

// resetProjects deletes all projects and judging data
func resetProjects(context *util.Context) util.Result {
	resetRes := util.PostRequest(context.Logger, "/admin/reset", util.H{"type": "projects"}, util.AdminAuth())
	if !util.IsOk(resetRes) {
		return util.NewResult(false, "Failed to reset projects: "+resetRes)
	}
	return util.ResultOk()
}

// setTrackViews sets the number of views per track
func setTrackViews(context *util.Context, views []int) util.Result {
	viewsRes := util.PostRequest(context.Logger, "/admin/track-views", util.H{"track_views": views}, util.AdminAuth())
	if !util.IsOk(viewsRes) {
		return util.NewResult(false, "Failed to set track views: "+viewsRes)
	}
	return util.ResultOk()
}
