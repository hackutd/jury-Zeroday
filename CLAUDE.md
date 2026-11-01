# CLAUDE.md

Guidance for working in this repo. Read it before making changes, and **match existing patterns over introducing new ones**.

## What this app is

**Jury** is a hackathon judging system built by the HackUTD tech team (upstream: `hackutd/jury`, docs at https://jury.mikz.dev). This repo is the **`hackutd/jury-Zeroday` fork**, deployed on DigitalOcean App Platform from `master`.

How judging works (it shapes almost every design decision):
- **Science-fair style.** Hackers stand at numbered tables. Judges walk the room, and Jury tells each judge which table to visit next.
- **Pairwise / ranking based**, inspired by Gavel. A judge sees one project, takes notes, can star it, and **drag-and-drop ranks** everything they've seen. Rankings are aggregated with **Copeland scoring** (`judging/scoring.go`). The goal is "more data from fewer judges".
- **Event-day flow:** admins import projects (usually a **Devpost CSV export**) and judges (CSV or form, or self-signup by QR code). Judges get a login code by email. An admin starts the clock, judges judge, admins watch scores and seen-counts live, pause/end, deliberate, then export results (incl. per-prize CSVs for sponsors).
- It runs **live during a stressful event** with many concurrent judges. Correctness under concurrency (transactions, "busy" projects), clear errors and fast admin visibility matter more than features. Don't break in-progress data. Prefer changes that tolerate existing documents (old fields, missing maps).

Key domain terms:
- **Project**: has `Location` (table number), `Group`, `ChallengeList` (Devpost "Opt-In Prizes"; these double as **tracks**), `Seen`, `TrackSeen[track]`, `Active`, `Prioritized`.
- **Judge**: authenticated by bearer token; has `Track` (`""` = general judge), `Group`, `SeenProjects`, `Rankings`, `RankingsAgg` (Copeland per judge), `Flagged`, `Current`.
- **Flag**: skip/report record with a `reason` (`busy`, `absent`, rule-break...). 3 `absent` flags auto-hide a project.
- **Groups** (`MultiGroup`): splits the room into zones so judges stay physically nearby, moving groups after `AutoSwitchProp` of a group is seen.
- **Tracks** (`JudgeTracks`): track judges see only projects whose `ChallengeList` contains their track, up to `TrackViews[i]` views (a cap, not a minimum). Next project = prioritized, then fewest `TrackSeen[track]`, then least compared using a matrix built on the fly from **that track's judges only**, then nearest table ahead (`PickNextTrackProject`). They rank like general judges: `track_scores[track]` (Copeland over that track's judges) plus `track_stars`. Track judges never affect the general `score`/`stars`. Track matching **ignores case** (`util.IndexFold` / `util.ContainsFold`, `util.ExactFoldRegex` for Mongo); judge tracks are stored with the casing from `options.Tracks`.
- **General judging off** (`DisableGeneralJudging`, only applies when `JudgeTracks` is on; use `options.GeneralJudgingEnabled()`): stored inverted so old options documents keep general on. When off, judges without a track are rejected on add (form, CSV, QR) and existing general judges get no project.
- **Deliberation**: option that freezes ranking edits.
- **Clock**: global judging timer / paused state (`models/clock.go`, `SafeClock`).
- **Comparisons**: in-memory matrix of how often each project pair was seen by the same judge. Used to pick the least-compared next project. Lives in router `State` and must be updated or reloaded when projects or judges change.

## Repo layout

```
server/            Go 1.23 backend (Gin + official MongoDB driver), module name `server`
  main.go          env check -> InitDb -> logger -> router -> Run(:PORT, default 8080)
  config/          env var checks (JURY_ADMIN_PASSWORD, EMAIL_FROM, and SMTP *or* SendGrid set)
  router/          HTTP handlers, grouped by area: admin.go, judge.go, project.go
                   init.go = ALL route registration; middleware.go = auth; state.go = shared State
  database/        All Mongo access, one file per collection (judge, project, options, flag, admin)
                   util.go = WithTransaction / WithTransactionItem
  models/          Structs + New* constructors (bson + json tags, snake_case)
  judging/         Core algorithms: judging_flow.go (next-project picking, skips, groups),
                   scoring.go (Copeland + score aggregation pipeline), comparisons.go
  funcs/           CSV parse/export, emails, project-number helpers
  logging/         Logger persisted to Mongo (SystemLogf / AdminLogf / JudgeLogf)
  util/            small generic helpers (Map, All, slices, time)
client/            React 18 + TypeScript + Vite + Tailwind + zustand, yarn
  src/index.tsx    all routes (react-router createBrowserRouter)
  src/api.ts       getRequest/postRequest/putRequest/deleteRequest -> FetchResponse<T>
  src/store.tsx    zustand stores (useAdminStore, useOptionsStore, ...)
  src/types.d.ts   global API types (mirror Go json tags exactly, snake_case)
  src/pages/       admin/*, judge/*, Expo (public table list), AddSelf (QR signup), Info, Print
  src/components/  hand-built shared components; admin/ and judge/ subfolders
tests/             Go integration test suite (separate module `tests`), runs against a live server
docs/              Docusaurus docs site (usage, internals, API reference)
.do/deploy.template.yaml   DigitalOcean "Deploy to DO" app spec
docker-compose*.yml        prod / dev (hot reload) / test / local mongo
scripts/           test-runner.sh, deploy.sh, run.sh
test-data/         sample judges.csv / projects.csv (Jury format)
```

## Backend patterns (follow these)

**Route handlers** (`router/*.go`):
- Comment above each handler with method + path: `// POST /judge/rank - JudgeRank ...`.
- Start with `state := GetState(ctx)`. Get the authed judge with `ctx.MustGet("judge").(*models.Judge)`.
- Bind with `ctx.BindJSON(&req)` using request structs defined next to the handler.
- Errors: `ctx.JSON(http.StatusXxx, gin.H{"error": "lowercase description: " + err.Error()})` then `return`.
- Success with no data: `ctx.JSON(http.StatusOK, gin.H{"ok": 1})`.
- Log admin and judge actions: `state.Logger.AdminLogf(...)` / `state.Logger.JudgeLogf(judge, ...)`.
- Multi-step writes go in `database.WithTransaction(state.Db, func(sc mongo.SessionContext) error { ... })`. Inside, write the HTTP error response *and* return the error.
- Register every route in `router/init.go` under the right group: `adminRouter` (Basic auth), `judgeRouter` (Bearer token) or `defaultRouter` (public). All are under `/api`.

**Database layer** (`database/*.go`):
- Functions take `(db *mongo.Database, ctx context.Context, ...)`. Pass the `mongo.SessionContext` as `ctx` inside transactions.
- Use `gin.H{...}` for filters and updates (not `bson.M`). Use `bson.D` only where order matters (aggregation pipelines).
- Return `[]*models.X`, initialized with `make(..., 0)` so JSON is `[]` and not `null`.
- Keep `last_activity: util.Now()` updated on judge and project writes.
- Functions that must run inside a transaction say so in their comment ("This should be run in a transaction") and often have a `...WithTx` variant.

**Models**: struct fields use `bson:"snake_case" json:"snake_case"`, plus a `NewX(...)` constructor that sets every field (including empty maps and slices). Options use a pointer-field `OptionalOptions` struct for partial updates.

**Avoid import cycles**: `judging` imports `database`. Logic needing both lives in `judging` (see the `SkipCurrentProjectWithTx` comment).

**Style**: `gofmt`, comments on every exported function, numbered comment steps for algorithms (see `PickNextProject` / `FindAvailableItems` doc comments). Keep those doc comments in sync when changing logic.

## Frontend patterns (follow these)

- **No component libraries.** Use the existing components in `src/components/` (Button, Popup, ConfirmPopup, Dropdown, Checkbox, TextInput, Container, ...) or build small ones in the same style.
- Styling: **Tailwind only**, with the theme colors and fonts in `client/tailwind.config.js` (`primary`, `gold`, `error`, `light`, `lighter`, `background`, ...). Combine classes with `twMerge(defaults, props.className)`.
- API calls go through `src/api.ts` helpers with `'admin'` or `'judge'` auth. Check `res.status !== 200` and then `errorAlert(res)` (from `util.tsx`).
- Shared admin data lives in zustand stores (`store.tsx`). After a mutation, call the store's `fetchX()` to refresh.
- Track-aware admin UI reads `useOptionsStore` `selectedTrack` and `options.judge_tracks`.
- Prettier: single quotes, 4-space indent, 100 print width (`.prettierrc`). ESLint via `yarn lint`.
- `VITE_*` env vars are **build-time** (baked into the bundle). `VITE_JURY_URL` is `/api` in prod.

## CSV import (important, it's fragile)

`server/funcs/csv.go` reads columns **by position, not header name**:
- Judges: `name, email, track?, notes?`. The track is validated in the router (`validateJudgeTrack`): trimmed, matched ignoring case against `options.Tracks`, rejected if unknown (tracks on), empty (general off) or set at all while tracks are off (track judges never count toward general scores). `/qr/check/:track` also fails when tracks are off.
- Projects (Jury format): `name, description, url, try_link?, video_link?, challenge_list?` (comma-separated, quoted).
- Devpost (2025 export format): still positional, but the header row is validated (`checkDevpostHeader`) so a layout change fails loudly. Uses col 0 title, 1 url, 2 status, 8 About The Project, 9 "Try it out" links, 10 video, **13 Opt-In Prizes** → `ChallengeList`. Drafts are skipped. `IgnoreTracks` filters at import time.
- "Export by Challenges" zips one CSV per prize (`Table, Name, Devpost, Try It Out`, sorted by table). Prize names are still used unsanitized as zip file names.

## Commands

```bash
# Dev (hot reload): backend :8000, frontend :3000. Needs .env (copy .env.template)
docker compose -f docker-compose.dev.yml up
# Local Mongo for dev, if not using Atlas
docker compose -f docker-compose-mongo.dev.yml up

# Backend checks
cd server && go build ./... && go vet ./... && go test ./...

# Frontend checks
cd client && yarn install && yarn lint && yarn build

# Integration tests (spins up mongo + server + tests/ runner via docker-compose.test.yml)
./scripts/test-runner.sh
```

Integration tests live in `tests/tests/*.go` as `func X(context *util.Context) util.Result` and are registered in a `TestGroup` in `tests/tests/all.go`. They hit the real API with `util.PostRequest(..., util.AdminAuth())`, etc. Add a test there for new endpoints or behavior.

## Fork-specific context and gotchas

- Deploy target is **DigitalOcean App Platform** from `hackutd/jury-Zeroday` `master`, built from the root `Dockerfile`. The `.do/deploy.template.yaml` source URL points to this fork. The "Deploy to DO" button in `docs/docs/usage/deploy.md` still points to upstream.
- `.github/workflows/deploy.yml` is inherited from upstream. It pushes `ghcr.io/hackutd/jury:latest` (it **would overwrite upstream's image**) and SSHes into upstream's server. Don't rely on it, and don't extend it. A CI/CD cleanup plan exists and is on hold.
- `scripts/test-runner.sh`: the `timeout` counter never increments, and empty logs count as success.
- `UpdateProjectComparisonCount` has no callers, so the shared general `Comparisons` matrix is only rebuilt at startup and on project add/edit/delete (stale during judging). Track picking doesn't use it.
- `CreateComparisons` maps unknown project IDs (eg. deleted projects in a judge's `SeenProjects`) to index 0. Track picking filters these out first (`withExistingSeenProjects`); general picking doesn't.
- Planned work under discussion: single-prize CSV export.

## Working rules

1. **Consistency first** (core principle in `docs/docs/details/principles.md`): copy the shape of a neighboring handler, DB function or component.
2. **Few dependencies**: don't add Go or npm packages without a strong reason. The prod image is meant to stay tiny.
3. **Keep Go json tags and `client/src/types.d.ts` in sync**. Renaming a field means updating both, plus every admin table or sort that reads it.
4. **Think about live events**: changes to scoring, assignment or models must work with existing Mongo documents and concurrent judges. Use transactions for multi-document writes, and keep `Comparisons` in sync.
5. **Document**: update the relevant page in `docs/docs/` when user-facing behavior changes.
