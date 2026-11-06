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

// PrizeRankingsEndToEnd verifies that every judge can rank the projects they saw for each prize in
// the settings (only the projects that opted into it), that bad prize rankings are rejected, that
// prize scores show up in the project list, and that resetting rankings clears prize rankings
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

	// "Best Hardware" is an opt-in prize that isn't ranked, and the case of "best design" differs
	addTestProject(context, "Prize Design", "Best Design")
	addTestProject(context, "Prize Design AI", "best design, Best AI")
	addTestProject(context, "Prize None", "")
	addTestProject(context, "Prize Hardware", "Best Hardware")
	ids, res := getProjectIDs(context, "Prize Design", "Prize Design AI", "Prize None", "Prize Hardware")
	if !res.Success {
		return res
	}
	design, designAI, none := ids[0], ids[1], ids[2]

	token, res := createNamedJudge(context, "prize_judge@example.com", "Prize Judge")
	if !res.Success {
		return res
	}
	auth := util.JudgeAuth(token)
	if _, res := judgeUntilDone(context, auth, 6); !res.Success {
		return res
	}

	// Only the ranked prizes the judge's seen projects opted into are listed
	prizes, res := getJudgePrizes(context, auth)
	if !res.Success {
		return res
	}
	if len(prizes) != 2 || !sameIDs(prizes["Best Design"].ProjectIds, design, designAI) || !sameIDs(prizes["Best AI"].ProjectIds, designAI) {
		return util.NewResult(false, fmt.Sprintf("Judge prizes should be Best Design [design, design AI] and Best AI [design AI], got %v", prizes))
	}

	// Bad prize rankings are rejected
	bad := []struct {
		why  string
		body util.H
	}{
		{"prize not in the settings", util.H{"prize": "Best Hardware", "ranking": []string{}}},
		{"project that did not opt in", util.H{"prize": "Best Design", "ranking": []string{designAI, none}}},
		{"duplicate project", util.H{"prize": "Best Design", "ranking": []string{design, design}}},
	}
	for _, b := range bad {
		if status, body := util.PostRequestWithStatus(context.Logger, "/judge/rank/prize", b.body, auth); status != 400 {
			return util.NewResult(false, fmt.Sprintf("Prize ranking with a %s should fail with 400, got %d: %s", b.why, status, body))
		}
	}

	// Rank Design AI above Design for Best Design (prize name case doesn't matter)
	rankRes := util.PostRequest(context.Logger, "/judge/rank/prize", util.H{"prize": "best design", "ranking": []string{designAI, design}}, auth)
	if !util.IsOk(rankRes) {
		return util.NewResult(false, "Prize ranking failed: "+rankRes)
	}
	prizes, res = getJudgePrizes(context, auth)
	if !res.Success {
		return res
	}
	if !slices.Equal(prizes["Best Design"].Ranking, []string{designAI, design}) {
		return util.NewResult(false, fmt.Sprintf("Best Design ranking should be saved, got %v", prizes["Best Design"].Ranking))
	}

	// Prize scores are Copeland scores over the prize ranking; general scores are untouched
	scores, res := getPrizeScores(context)
	if !res.Success {
		return res
	}
	if scores[designAI]["Best Design"] != 1 || scores[design]["Best Design"] != -1 {
		return util.NewResult(false, fmt.Sprintf("Best Design scores should be design AI 1, design -1, got %v / %v", scores[designAI], scores[design]))
	}
	if score, ok := scores[designAI]["Best AI"]; !ok || score != 0 {
		return util.NewResult(false, fmt.Sprintf("Unranked Best AI should score 0, got %v", scores[designAI]))
	}
	if _, ok := scores[none]["Best Design"]; ok {
		return util.NewResult(false, "A project that did not opt in should not get a Best Design score")
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
