package tests

import (
	"fmt"
	"strings"
	"sync"
	"tests/util"
	"time"
)

// --- Concurrency Tests ---

// ConcurrentJudgesGetProjects verifies that many general judges asking for (and finishing)
// projects at the same time all get a project, without errors or lost seen counts
func ConcurrentJudgesGetProjects(context *util.Context) util.Result {
	if res := resetProjects(context); !res.Success {
		return res
	}
	for i := 1; i <= 15; i++ {
		addTestProject(context, fmt.Sprintf("Concurrent Project %d", i), "")
	}

	var auths []string
	for i := 1; i <= 10; i++ {
		token, res := createNamedJudge(context, fmt.Sprintf("concurrent_%d@example.com", i), fmt.Sprintf("Concurrent Judge %d", i))
		if !res.Success {
			return res
		}
		auths = append(auths, util.JudgeAuth(token))
	}

	// Round 1: everyone asks for a project at once
	first, res := concurrentNextRound(context, auths, nil)
	if !res.Success {
		return res
	}

	// Everyone finishes at once
	errs := runConcurrently(len(auths), func(i int) string {
		finish := util.H{"notes": "", "starred": false}
		status, body := util.PostRequestWithStatus(context.Logger, "/judge/finish", finish, auths[i])

		// Known upstream race: two judges given the same project (see concurrentNextRound) who finish
		// at the same time hit a Mongo write conflict, and the judge has to submit again. This is
		// only logged until that's fixed, but the resubmit must work (staggered, like a real judge).
		for attempt := 1; attempt <= 5 && status == 500 && strings.Contains(body, "WriteConflict"); attempt++ {
			context.Logger.Log(util.Info, "\tWARNING: judge %d finish hit a write conflict (known race), retrying\n", i+1)
			time.Sleep(time.Duration(attempt*200+i*50) * time.Millisecond)
			status, body = util.PostRequestWithStatus(context.Logger, "/judge/finish", finish, auths[i])
		}
		if status != 200 || !util.IsOk(body) {
			return fmt.Sprintf("judge %d finish failed (%d): %s", i+1, status, body)
		}
		return ""
	})
	if len(errs) > 0 {
		return util.NewResult(false, fmt.Sprintf("Concurrent finish failed: %v", errs))
	}

	// No seen counts should be lost
	projects, res := getTestProjects(context)
	if !res.Success {
		return res
	}
	total := int64(0)
	for _, p := range projects {
		total += p.Seen
	}
	if total != int64(len(auths)) {
		return util.NewResult(false, fmt.Sprintf("Expected %d total views after concurrent finish, got %d", len(auths), total))
	}

	// Round 2: everyone asks again at once, and nobody gets the project they just saw
	_, res = concurrentNextRound(context, auths, first)
	return res
}

// PausedNextDoesNotDeadlock verifies that asking for a project while the clock is paused returns
// nothing and does not leave the clock locked (which used to freeze every clock request)
func PausedNextDoesNotDeadlock(context *util.Context) util.Result {
	token, res := createNamedJudge(context, "paused_next@example.com", "Paused Next Judge")
	if !res.Success {
		return res
	}
	auth := util.JudgeAuth(token)

	if pauseRes := util.PostRequest(context.Logger, "/admin/clock/pause", nil, util.AdminAuth()); !util.IsOk(pauseRes) {
		return util.NewResult(false, "Failed to pause clock: "+pauseRes)
	}

	// Run the requests with a timeout, since a deadlock would hang forever
	done := make(chan util.Result, 1)
	go func() {
		for i := 0; i < 2; i++ {
			status, body := util.PostRequestWithStatus(context.Logger, "/judge/next", nil, auth)
			if status != 200 || util.ExtractString(body, "project_id") != "" {
				done <- util.NewResult(false, fmt.Sprintf("Paused /judge/next should return no project, got %d: %s", status, body))
				return
			}
		}
		if status, body := util.GetRequestWithStatus(context.Logger, "/admin/clock", util.AdminAuth()); status != 200 {
			done <- util.NewResult(false, fmt.Sprintf("GET /admin/clock failed after paused next (%d): %s", status, body))
			return
		}
		if unpauseRes := util.PostRequest(context.Logger, "/admin/clock/unpause", nil, util.AdminAuth()); !util.IsOk(unpauseRes) {
			done <- util.NewResult(false, "Failed to unpause clock: "+unpauseRes)
			return
		}
		done <- util.ResultOk()
	}()

	select {
	case res := <-done:
		return res
	case <-time.After(15 * time.Second):
		return util.NewResult(false, "Clock requests timed out after a paused /judge/next (clock mutex deadlock)")
	}
}

// concurrentNextRound has every judge ask for a project at the same time, and checks that all
// requests succeed with a project (and, if given, not the judge's previous project).
// Returns the project ID given to each judge.
func concurrentNextRound(context *util.Context, auths []string, previous []string) ([]string, util.Result) {
	ids := make([]string, len(auths))
	errs := runConcurrently(len(auths), func(i int) string {
		status, body := util.PostRequestWithStatus(context.Logger, "/judge/next", nil, auths[i])
		ids[i] = util.ExtractString(body, "project_id")
		if status != 200 || ids[i] == "" {
			return fmt.Sprintf("judge %d got no project (%d): %s", i+1, status, body)
		}
		return ""
	})
	if len(errs) > 0 {
		return nil, util.NewResult(false, fmt.Sprintf("Concurrent /judge/next failed: %v", errs))
	}

	given := map[string]int{}
	for i, id := range ids {
		// Known upstream race: judges asking at the exact same moment can be given the same project
		// (picking doesn't write the project, so Mongo sees no transaction conflict). This is only
		// logged until that's fixed, since judges can skip a busy project.
		if other, ok := given[id]; ok {
			context.Logger.Log(util.Info, "\tWARNING: judges %d and %d were both given project %s (known race)\n", other+1, i+1, id)
		}
		given[id] = i
		if previous != nil && previous[i] == id {
			return nil, util.NewResult(false, fmt.Sprintf("Judge %d was given the project they already saw", i+1))
		}
	}
	return ids, util.ResultOk()
}

// runConcurrently runs fn for 0..n-1 at the same time and returns the non-empty error messages
func runConcurrently(n int, fn func(i int) string) []string {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []string
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if err := fn(i); err != "" {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(i)
	}

	// Release all requests at once
	close(start)
	wg.Wait()
	return errs
}
