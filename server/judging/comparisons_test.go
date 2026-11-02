package judging

import (
	"server/models"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func newTestProject(location int64) *models.Project {
	p := models.NewProject("Project", location, 0, "", "", "", "", []string{"Design"})
	p.Id = primitive.NewObjectID()
	return p
}

func newTestJudge(track string, seen ...*models.Project) *models.Judge {
	j := models.NewJudge("Judge", "judge@example.com", track, "", 0)
	for _, p := range seen {
		j.SeenProjects = append(j.SeenProjects, models.JudgedProject{ProjectId: p.Id, Location: p.Location})
	}
	return j
}

func TestFindLeastComparedAllUsesOnlyTrackJudges(t *testing.T) {
	p1, p2, p3, p4 := newTestProject(1), newTestProject(2), newTestProject(3), newTestProject(4)
	projects := []*models.Project{p1, p2, p3, p4}

	// Another Design judge compared p1 with p2; a general judge compared p1 with p3
	otherTrackJudge := newTestJudge("Design", p1, p2)
	generalJudge := newTestJudge("", p1, p3)
	current := newTestJudge("Design", p1)

	// Only the track's judges go into the matrix, so the general judge's p1-p3 pair is ignored
	trackComps := CreateComparisons(projects, []*models.Judge{otherTrackJudge, current})
	got := trackComps.FindLeastComparedAll([]*models.Project{p2, p3, p4}, current.SeenProjects)
	if len(got) != 2 || got[0] != p3 || got[1] != p4 {
		t.Fatalf("expected p3 and p4 to be tied for least compared, got %d projects", len(got))
	}

	// Sanity check: with the general judge included, p3 would no longer be tied
	allComps := CreateComparisons(projects, []*models.Judge{otherTrackJudge, generalJudge, current})
	got = allComps.FindLeastComparedAll([]*models.Project{p2, p3, p4}, current.SeenProjects)
	if len(got) != 1 || got[0] != p4 {
		t.Fatalf("expected only p4 when general judge is counted, got %d projects", len(got))
	}
}

func TestGetNextFreeProjectPicksNearestTableAhead(t *testing.T) {
	p3, p7, p12 := newTestProject(3), newTestProject(7), newTestProject(12)
	tied := []*models.Project{p12, p3, p7}

	got, _ := GetNextFreeProject(5, tied)
	if got != p7 {
		t.Fatalf("expected table 7 after table 5, got table %d", got.Location)
	}

	// Wraps around to the lowest table after the last one
	got, _ = GetNextFreeProject(12, tied)
	if got != p3 {
		t.Fatalf("expected wrap around to table 3, got table %d", got.Location)
	}
}
