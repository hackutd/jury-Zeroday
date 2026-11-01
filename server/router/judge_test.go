package router

import (
	"server/models"
	"testing"
)

func TestValidateJudgeTrack(t *testing.T) {
	options := models.NewOptions()
	options.JudgeTracks = true
	options.Tracks = []string{"Design", "Best Use of AI"}

	// Tracks match ignoring case and whitespace, and use the options' casing
	got, err := validateJudgeTrack(options, "  best use of ai ")
	if err != nil || got != "Best Use of AI" {
		t.Fatalf("expected 'Best Use of AI', got '%s' (err: %v)", got, err)
	}

	// Unknown tracks are rejected
	if _, err := validateJudgeTrack(options, "Hardware"); err == nil {
		t.Fatal("expected unknown track to be rejected")
	}

	// General judges are allowed while general judging is kept
	if _, err := validateJudgeTrack(options, ""); err != nil {
		t.Fatalf("expected general judge to be allowed, got: %s", err.Error())
	}

	// General judges are rejected when general judging is disabled
	options.DisableGeneralJudging = true
	if _, err := validateJudgeTrack(options, ""); err == nil {
		t.Fatal("expected general judge to be rejected when general judging is disabled")
	}

	// With track judging off, general judging can't be disabled and tracks are rejected
	options.JudgeTracks = false
	if _, err := validateJudgeTrack(options, ""); err != nil {
		t.Fatalf("expected general judge to be allowed with tracks off, got: %s", err.Error())
	}
	if _, err := validateJudgeTrack(options, "Design"); err == nil {
		t.Fatal("expected track to be rejected with tracks off")
	}
}
