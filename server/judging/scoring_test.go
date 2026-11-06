package judging

import (
	"server/models"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// seenJudge returns a judge who has seen the given projects, in order
func seenJudge(projects ...*models.Project) *models.Judge {
	judge := models.NewJudge("Judge", "judge@example.com", "", "", 0)
	for _, p := range projects {
		judge.SeenProjects = append(judge.SeenProjects, *models.JudgeProjectFromProject(p, "", false))
	}
	return judge
}

// newScoringProject returns a project with a fresh ID and the given opt-in prizes
func newScoringProject(name string, prizes ...string) *models.Project {
	p := models.NewProject(name, 1, 0, "", "", "", "", prizes)
	p.Id = primitive.NewObjectID()
	return p
}

func TestAggregateRankingCopeland(t *testing.T) {
	a, b, c := newScoringProject("A"), newScoringProject("B"), newScoringProject("C")
	judge := seenJudge(a, b, c)
	judge.Rankings = []primitive.ObjectID{b.Id, a.Id}

	// Ranked: B = 2 + 1 - 1 - 0 = 2, A = 2 - 2 = 0; unranked C = -2
	want := map[primitive.ObjectID]int64{b.Id: 2, a.Id: 0, c.Id: -2}
	for _, agg := range AggregateRanking(judge) {
		if want[agg.ProjectId] != agg.Score {
			t.Fatalf("project %s: got score %d, want %d", agg.ProjectId.Hex(), agg.Score, want[agg.ProjectId])
		}
	}
}

func TestAggregatePrizeScores(t *testing.T) {
	design1 := newScoringProject("Design 1", "Best Design")
	design2 := newScoringProject("Design 2", "best design", "Best AI") // case differs
	ai := newScoringProject("AI", "Best AI")
	other := newScoringProject("Other")
	deleted := newScoringProject("Deleted", "Best Design")
	projects := []*models.Project{design1, design2, ai, other} // deleted is not in the list

	// Judge 1 ranks Design 2 above Design 1 for Best Design, and also ranked a deleted project
	// and a project that never opted in, which must both be ignored
	j1 := seenJudge(design1, design2, ai, other, deleted)
	j1.SetPrizeRanking("Best Design", []primitive.ObjectID{deleted.Id, design2.Id, other.Id, design1.Id})

	// Judge 2 ranks Design 1 first, and never ranked for Best AI
	j2 := seenJudge(design1, design2)
	j2.SetPrizeRanking("Best Design", []primitive.ObjectID{design1.Id})

	// Old judge document with no prize rankings at all
	j3 := seenJudge(design1)
	j3.PrizeRankings = nil

	scores := AggregatePrizeScores([]*models.Judge{j1, j2, j3}, projects, []string{"Best Design", "Best AI"})

	// Best Design:
	//   j1: candidates [design1, design2], ranking [design2, design1] -> design2 = 1, design1 = -1
	//   j2: candidates [design1, design2], ranking [design1]          -> design1 = 1, design2 = -1
	//   j3: candidates [design1], no ranking                         -> design1 = 0
	// Best AI:
	//   j1: candidates [design2, ai], no ranking -> both 0
	//   j2: candidates [design2], no ranking     -> 0
	checks := []struct {
		project *models.Project
		prize   string
		want    int64
	}{
		{design1, "Best Design", 0},
		{design2, "Best Design", 0},
		{design2, "Best AI", 0},
		{ai, "Best AI", 0},
	}
	for _, c := range checks {
		got, ok := scores[c.project.Id][c.prize]
		if !ok || got != c.want {
			t.Fatalf("%s %s: got %d (present %v), want %d", c.project.Name, c.prize, got, ok, c.want)
		}
	}

	// Projects that did not opt in get no score for the prize
	if _, ok := scores[other.Id]["Best Design"]; ok {
		t.Fatalf("project that did not opt in should not have a Best Design score")
	}
	if _, ok := scores[ai.Id]["Best Design"]; ok {
		t.Fatalf("AI project should not have a Best Design score")
	}
	if _, ok := scores[deleted.Id]; ok {
		t.Fatalf("deleted project should not have scores")
	}

	// A single judge's ranking decides the order when they're the only one
	solo := AggregatePrizeScores([]*models.Judge{j1}, projects, []string{"Best Design"})
	if solo[design2.Id]["Best Design"] <= solo[design1.Id]["Best Design"] {
		t.Fatalf("design2 should beat design1 for j1, got %v", solo)
	}
}
