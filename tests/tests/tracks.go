package tests

import (
	"encoding/json"
	"fmt"
	"tests/util"
)

// --- Track Judging Tests ---

// TrackTestSetup enables track judging with a single "Design" track (keeping general judging)
// and makes sure judging is running
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
		"tracks": []string{"Design"},
	}, util.AdminAuth())
	if !util.IsOk(tracksRes) {
		return util.NewResult(false, "Failed to set tracks: "+tracksRes)
	}

	viewsRes := util.PostRequest(context.Logger, "/admin/track-views", util.H{
		"track_views": []int{3},
	}, util.AdminAuth())
	if !util.IsOk(viewsRes) {
		return util.NewResult(false, "Failed to set track views: "+viewsRes)
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
