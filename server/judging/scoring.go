package judging

import (
	"context"
	"server/database"
	"server/models"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AggregateRanking will take the ranking array from the judge and create
// the aggregated ranking model using the Copeland counting method
func AggregateRanking(judge *models.Judge) []models.AggRanking {
	seen := make([]primitive.ObjectID, 0, len(judge.SeenProjects))
	for _, p := range judge.SeenProjects {
		seen = append(seen, p.ProjectId)
	}
	return copelandScores(judge.Rankings, seen)
}

// copelandScores scores one judge's ranking with the Copeland counting method. Candidates are all
// projects the ranking could contain; the ones not in the ranking count as unranked.
//  1. Ranked project at index i: wins against everything below it and all unranked projects,
//     loses to everything above it, so score = (rc - i - 1) - i + uc = rc + uc - 1 - 2i
//  2. Unranked project: beaten by every ranked project, so score = -rc
func copelandScores(ranking []primitive.ObjectID, candidates []primitive.ObjectID) []models.AggRanking {
	// Get unranked projects
	var unranked []primitive.ObjectID
	for _, p := range candidates {
		if !slices.Contains(ranking, p) {
			unranked = append(unranked, p)
		}
	}

	// Create output
	rc := len(ranking)
	uc := len(unranked)
	out := make([]models.AggRanking, 0, rc+uc)

	// Calculate scores for ranked projects
	c := rc + uc - 1
	for i, p := range ranking {
		out = append(out, *models.NewAggRanking(p, int64(c-2*i)))
	}

	// Calculate scores for unranked projects
	for _, p := range unranked {
		out = append(out, *models.NewAggRanking(p, int64(-rc)))
	}

	return out
}

// PrizeProjectIds returns the IDs of the projects the judge has seen whose HackUTD track (the Devpost
// "What HackUTD track" answer, not the Opt-In Prizes) is the prize, ignoring case, in the order they
// were seen. Seen projects that no longer exist are left out.
func PrizeProjectIds(judge *models.Judge, prize string, projects map[primitive.ObjectID]*models.Project) []primitive.ObjectID {
	out := make([]primitive.ObjectID, 0)
	for _, seen := range judge.SeenProjects {
		project, ok := projects[seen.ProjectId]
		if ok && strings.EqualFold(strings.TrimSpace(project.HackUTDTrack), prize) {
			out = append(out, seen.ProjectId)
		}
	}
	return out
}

// AggregatePrizeScores calculates every project's score for each prize in options.PrizeRankings,
// keyed by project ID and then prize name. Every judge counts, general and track judges alike.
//  1. For each judge and prize, the candidates are the seen projects in that HackUTD track
//     (see PrizeProjectIds)
//  2. The judge's prize ranking is filtered to those candidates (drops deleted projects and
//     projects no longer in the track), then scored with the Copeland method
//  3. Scores are summed over all judges
func AggregatePrizeScores(judges []*models.Judge, projects []*models.Project, prizes []string) map[primitive.ObjectID]map[string]int64 {
	byId := make(map[primitive.ObjectID]*models.Project, len(projects))
	for _, p := range projects {
		byId[p.Id] = p
	}

	out := make(map[primitive.ObjectID]map[string]int64)
	for _, judge := range judges {
		for _, prize := range prizes {
			candidates := PrizeProjectIds(judge, prize, byId)
			if len(candidates) == 0 {
				continue
			}
			ranking := make([]primitive.ObjectID, 0)
			for _, id := range judge.GetPrizeRanking(prize) {
				if slices.Contains(candidates, id) && !slices.Contains(ranking, id) {
					ranking = append(ranking, id)
				}
			}
			for _, agg := range copelandScores(ranking, candidates) {
				if out[agg.ProjectId] == nil {
					out[agg.ProjectId] = make(map[string]int64)
				}
				out[agg.ProjectId][prize] += agg.Score
			}
		}
	}
	return out
}

// InitAggregateRankings will re-calculate all judge ranking aggregation objects
// when the Jury backend starts
func InitAggregateRankings(db *mongo.Database) error {
	return database.WithTransaction(db, func(sc mongo.SessionContext) error {
		// Get all judges
		judges, err := database.FindAllJudges(db, sc)
		if err != nil {
			return err
		}

		// Early return if no judges exist
		if len(judges) == 0 {
			return nil
		}

		// Use bulk write to update all judges' aggregated rankings
		models := make([]mongo.WriteModel, 0, len(judges))
		for _, judge := range judges {
			agg := AggregateRanking(judge)
			models = append(models, mongo.NewUpdateOneModel().SetFilter(gin.H{"_id": judge.Id}).SetUpdate(gin.H{"$set": gin.H{"rankings_agg": agg}}))
		}
		opts := options.BulkWrite().SetOrdered(false)
		_, err = db.Collection("judges").BulkWrite(sc, models, opts)
		return err
	})
}

type ProjectScores struct {
	Score       int64            `bson:"score" json:"score"`
	Stars       int64            `bson:"stars" json:"stars"`
	TrackStars  map[string]int64 `bson:"track_stars" json:"track_stars"`
	TrackScores map[string]int64 `bson:"track_scores" json:"track_scores"`
}

type ProjectScoresWithId struct {
	ProjectId   primitive.ObjectID `bson:"_id" json:"_id"`
	Score       int64              `bson:"score" json:"score"`
	Stars       int64              `bson:"stars" json:"stars"`
	TrackStars  map[string]int64   `bson:"track_stars" json:"track_stars"`
	TrackScores map[string]int64   `bson:"track_scores" json:"track_scores"`
}

func removeId(scoresWithId *ProjectScoresWithId) *ProjectScores {
	return &ProjectScores{
		Score:       scoresWithId.Score,
		Stars:       scoresWithId.Stars,
		TrackStars:  scoresWithId.TrackStars,
		TrackScores: scoresWithId.TrackScores,
	}
}

// AggregateScores takes the scores and stars from judges and aggregates them to form a final ranking.
// General scores and stars only come from general judges. Track scores and stars only come from
// the judges of that track, keyed by track name.
func AggregateScores(db *mongo.Database, ctx context.Context) (map[primitive.ObjectID]ProjectScores, error) {
	pipeline := mongo.Pipeline{
		// === Pipeline 1: Aggregate all general judges' ranking scores ===
		bson.D{{"$match", bson.D{{"track", ""}}}},

		bson.D{{"$unwind", "$rankings_agg"}},
		bson.D{{"$group", bson.D{
			{"_id", "$rankings_agg.project_id"},
			{"score", bson.D{{"$sum", "$rankings_agg.score"}}},
		}}},

		// === Pipeline 2: Aggregate all general judges' stars ===
		bson.D{{"$unionWith", gin.H{
			"coll": "judges",
			"pipeline": []gin.H{
				{"$match": gin.H{"track": ""}},
				{"$unwind": "$seen_projects"},
				{"$match": gin.H{"seen_projects.starred": true}},
				{"$group": gin.H{
					"_id":   "$seen_projects.project_id",
					"stars": gin.H{"$sum": 1},
				}},
			},
		}}},

		// === Combine scores and stars (first stage) ===
		bson.D{{"$group", bson.D{
			{"_id", "$_id"},
			{"score", bson.D{{"$sum", "$score"}}},
			{"stars", bson.D{{"$sum", "$stars"}}},
		}}},

		// // === Pipeline 3: Aggregate all track judges' stars, grouped by project and track ===
		bson.D{{"$unionWith", gin.H{
			"coll": "judges",
			"pipeline": []gin.H{
				{"$match": gin.H{"track": gin.H{"$ne": ""}}},
				{"$unwind": "$seen_projects"},
				{"$match": gin.H{"seen_projects.starred": true}},
				{"$group": gin.H{
					"_id": gin.H{
						"projectId": "$seen_projects.project_id",
						"track":     "$track",
					},
					"count": gin.H{"$sum": 1},
				}},
				{"$group": gin.H{
					"_id": "$_id.projectId",
					"track_stars": gin.H{
						"$push": gin.H{
							"k": "$_id.track",
							"v": "$count",
						},
					},
				}},
				{"$addFields": gin.H{
					"track_stars": gin.H{
						"$arrayToObject": "$track_stars",
					},
				}},
			},
		}}},

		// === Pipeline 4: Aggregate all track judges' ranking scores, grouped by project and track ===
		bson.D{{"$unionWith", gin.H{
			"coll": "judges",
			"pipeline": []gin.H{
				{"$match": gin.H{"track": gin.H{"$ne": ""}}},
				{"$unwind": "$rankings_agg"},
				{"$group": gin.H{
					"_id": gin.H{
						"projectId": "$rankings_agg.project_id",
						"track":     "$track",
					},
					"score": gin.H{"$sum": "$rankings_agg.score"},
				}},
				{"$group": gin.H{
					"_id": "$_id.projectId",
					"track_scores": gin.H{
						"$push": gin.H{
							"k": "$_id.track",
							"v": "$score",
						},
					},
				}},
				{"$addFields": gin.H{
					"track_scores": gin.H{
						"$arrayToObject": "$track_scores",
					},
				}},
			},
		}}},

		// // === Final merge of all fields ===
		bson.D{{"$group", bson.D{
			{"_id", "$_id"},
			{"score", bson.D{{"$sum", "$score"}}},
			{"stars", bson.D{{"$sum", "$stars"}}},
			{"track_stars", bson.D{{"$mergeObjects", "$track_stars"}}},
			{"track_scores", bson.D{{"$mergeObjects", "$track_scores"}}},
		}}},
	}

	// Run the aggregation and get results with cursor
	cursor, err := db.Collection("judges").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var results []ProjectScoresWithId
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	// Convert results to map
	out := make(map[primitive.ObjectID]ProjectScores)
	for _, p := range results {
		out[p.ProjectId] = *removeId(&p)
	}

	return out, nil
}
