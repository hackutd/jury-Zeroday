package tests

import (
	"encoding/json"
	"fmt"
	"slices"
	"tests/util"
)

// --- Prize Ranking Tests ---

// testJudgePrize mirrors one entry of GET /judge/prizes
type testJudgePrize struct {
	Prize      string   `json:"prize"`
	ProjectIds []string `json:"project_ids"`
	Ranking    []string `json:"ranking"`
}

// getJudgePrizes returns the prizes a judge can rank for, keyed by prize name
func getJudgePrizes(context *util.Context, auth string) (map[string]testJudgePrize, util.Result) {
	var prizes []testJudgePrize
	body := util.GetRequest(context.Logger, "/judge/prizes", auth)
	if err := json.Unmarshal([]byte(body), &prizes); err != nil {
		return nil, util.NewResult(false, "Could not parse judge prizes: "+err.Error()+" ("+body+")")
	}
	byName := map[string]testJudgePrize{}
	for _, p := range prizes {
		byName[p.Prize] = p
	}
	return byName, util.ResultOk()
}

// getPrizeScores returns every project's prize scores, keyed by project ID
func getPrizeScores(context *util.Context) (map[string]map[string]int64, util.Result) {
	var projects []struct {
		Id          string           `json:"id"`
		PrizeScores map[string]int64 `json:"prize_scores"`
	}
	listRes := util.GetRequest(context.Logger, "/project/list", util.AdminAuth())
	if err := json.Unmarshal([]byte(listRes), &projects); err != nil {
		return nil, util.NewResult(false, "Could not parse project list: "+err.Error())
	}
	out := map[string]map[string]int64{}
	for _, p := range projects {
		out[p.Id] = p.PrizeScores
	}
	return out, util.ResultOk()
}

// devpostTrackCSV is a small Devpost export with HackUTD's "What HackUTD track" custom question.
// "Prize None" has Best Design in its Opt-In Prizes but no track, and must not be ranked for it.
const devpostTrackCSV = `Project Title,Submission Url,Project Status,Judging Status,Highest Step Completed,Project Created At,Project Submitted At,Submission Last Submitted At,About The Project,"""Try it out"" Links",Video Demo Link,Image Gallery URLs,Attached File S3 Key,Opt-In Prizes,Built With,What HackUTD track would you like to apply to? (You will automatically be considered for General)
Prize Design,https://devpost.com/1,Submitted (Gallery/Visible),Pending,Submit,,,,About,,,,,,go,Best Design
Prize Design 2,https://devpost.com/2,Submitted (Gallery/Visible),Pending,Submit,,,,About,,,,,,go,best design
Prize AI,https://devpost.com/3,Submitted (Gallery/Visible),Pending,Submit,,,,About,,,,,,go,Best AI
Prize None,https://devpost.com/4,Submitted (Gallery/Visible),Pending,Submit,,,,About,,,,,Best Design,go,
Prize Hardware,https://devpost.com/5,Submitted (Gallery/Visible),Pending,Submit,,,,About,,,,,,go,Best Hardware`

// PrizeRankingsEndToEnd verifies that every judge can rank the projects they saw for each prize in
// the settings (only the projects whose HackUTD track is that prize, not Opt-In Prizes), that bad
// prize rankings are rejected, that prize scores show up in the project list, and that resetting
// rankings clears prize rankings
func PrizeRankingsEndToEnd(context *util.Context) util.Result {
	if res := resetProjects(context); !res.Success {
		return res
	}

	// Leave prize rankings off for the other tests, whatever happens here
	defer util.PostRequest(context.Logger, "/admin/prize-rankings", util.H{"prize_rankings": []string{}}, util.AdminAuth())

	// Names are trimmed, blanks dropped, duplicates (ignoring case) removed and Best Beginner (filter only,
	// never ranked) dropped
	setRes := util.PostRequest(context.Logger, "/admin/prize-rankings", util.H{
		"prize_rankings": []string{"Best Design", "best design", " Best AI ", "", "Best Beginner Hack"},
	}, util.AdminAuth())
	if !util.IsOk(setRes) {
		return util.NewResult(false, "Failed to set prize rankings: "+setRes)
	}
	var options struct {
		PrizeRankings []string `json:"prize_rankings"`
	}
	if err := json.Unmarshal([]byte(util.GetRequest(context.Logger, "/admin/options", util.AdminAuth())), &options); err != nil {
		return util.NewResult(false, "Could not parse options: "+err.Error())
	}
	if !slices.Equal(options.PrizeRankings, []string{"Best Design", "Best AI"}) {
		return util.NewResult(false, fmt.Sprintf("Prize rankings should be [Best Design, Best AI], got %v", options.PrizeRankings))
	}

	// "Best Hardware" is a track that isn't ranked, and the case of "best design" differs
	status, body := util.PostFileWithStatus(context.Logger, "/project/devpost", "csv", "devpost.csv", devpostTrackCSV, nil, util.AdminAuth())
	if status != 200 || !util.IsOk(body) {
		return util.NewResult(false, fmt.Sprintf("Devpost import with HackUTD tracks failed (%d): %s", status, body))
	}
	ids, res := getProjectIDs(context, "Prize Design", "Prize Design 2", "Prize AI", "Prize None", "Prize Hardware")
	if !res.Success {
		return res
	}
	design, design2, ai, none := ids[0], ids[1], ids[2], ids[3]

	// The HackUTD track answers are listed exactly as typed in Devpost
	var tracks []string
	if err := json.Unmarshal([]byte(util.GetRequest(context.Logger, "/admin/hackutd-tracks", util.AdminAuth())), &tracks); err != nil {
		return util.NewResult(false, "Could not parse HackUTD tracks: "+err.Error())
	}
	if !slices.Equal(tracks, []string{"Best AI", "Best Design", "Best Hardware", "best design"}) {
		return util.NewResult(false, fmt.Sprintf("HackUTD tracks should be [Best AI, Best Design, Best Hardware, best design], got %v", tracks))
	}
	if status, _ := util.GetRequestWithStatus(context.Logger, "/admin/hackutd-tracks", util.DefaultAuth()); status == 200 {
		return util.NewResult(false, "HackUTD tracks must require admin auth")
	}

	token, res := createNamedJudge(context, "prize_judge@example.com", "Prize Judge")
	if !res.Success {
		return res
	}
	auth := util.JudgeAuth(token)
	if _, res := judgeUntilDone(context, auth, 7); !res.Success {
		return res
	}

	// Only the ranked prizes the judge's seen projects are in (by HackUTD track) are listed
	prizes, res := getJudgePrizes(context, auth)
	if !res.Success {
		return res
	}
	if len(prizes) != 2 || !sameIDs(prizes["Best Design"].ProjectIds, design, design2) || !sameIDs(prizes["Best AI"].ProjectIds, ai) {
		return util.NewResult(false, fmt.Sprintf("Judge prizes should be Best Design [design, design 2] and Best AI [ai], got %v", prizes))
	}

	// Bad prize rankings are rejected
	bad := []struct {
		why  string
		body util.H
	}{
		{"prize not in the settings", util.H{"prize": "Best Hardware", "ranking": []string{}}},
		{"project not in the track (only in Opt-In Prizes)", util.H{"prize": "Best Design", "ranking": []string{design2, none}}},
		{"duplicate project", util.H{"prize": "Best Design", "ranking": []string{design, design}}},
	}
	for _, b := range bad {
		if status, body := util.PostRequestWithStatus(context.Logger, "/judge/rank/prize", b.body, auth); status != 400 {
			return util.NewResult(false, fmt.Sprintf("Prize ranking with a %s should fail with 400, got %d: %s", b.why, status, body))
		}
	}

	// Rank Design 2 above Design for Best Design (prize name case doesn't matter)
	rankRes := util.PostRequest(context.Logger, "/judge/rank/prize", util.H{"prize": "best design", "ranking": []string{design2, design}}, auth)
	if !util.IsOk(rankRes) {
		return util.NewResult(false, "Prize ranking failed: "+rankRes)
	}
	prizes, res = getJudgePrizes(context, auth)
	if !res.Success {
		return res
	}
	if !slices.Equal(prizes["Best Design"].Ranking, []string{design2, design}) {
		return util.NewResult(false, fmt.Sprintf("Best Design ranking should be saved, got %v", prizes["Best Design"].Ranking))
	}

	// Prize scores are Copeland scores over the prize ranking; general scores are untouched
	scores, res := getPrizeScores(context)
	if !res.Success {
		return res
	}
	if scores[design2]["Best Design"] != 1 || scores[design]["Best Design"] != -1 {
		return util.NewResult(false, fmt.Sprintf("Best Design scores should be design 2 1, design -1, got %v / %v", scores[design2], scores[design]))
	}
	if score, ok := scores[ai]["Best AI"]; !ok || score != 0 {
		return util.NewResult(false, fmt.Sprintf("Unranked Best AI should score 0, got %v", scores[ai]))
	}
	if _, ok := scores[none]["Best Design"]; ok {
		return util.NewResult(false, "A project with Best Design only in its Opt-In Prizes should not get a Best Design score")
	}

	// A judge who hasn't seen a project can't rank it
	otherToken, res := createNamedJudge(context, "prize_judge_2@example.com", "Prize Judge 2")
	if !res.Success {
		return res
	}
	if status, body := util.PostRequestWithStatus(context.Logger, "/judge/rank/prize", util.H{"prize": "Best Design", "ranking": []string{design}}, util.JudgeAuth(otherToken)); status != 400 {
		return util.NewResult(false, fmt.Sprintf("Ranking an unseen project should fail with 400, got %d: %s", status, body))
	}

	// Clearing rankings also clears prize rankings
	if resetRes := util.PostRequest(context.Logger, "/admin/reset", util.H{"type": "rankings"}, util.AdminAuth()); !util.IsOk(resetRes) {
		return util.NewResult(false, "Failed to reset rankings: "+resetRes)
	}
	prizes, res = getJudgePrizes(context, auth)
	if !res.Success {
		return res
	}
	if len(prizes["Best Design"].Ranking) != 0 {
		return util.NewResult(false, fmt.Sprintf("Resetting rankings should clear prize rankings, got %v", prizes["Best Design"].Ranking))
	}

	return util.ResultOk()
}
