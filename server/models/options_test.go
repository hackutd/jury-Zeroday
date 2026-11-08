package models

import (
	"slices"
	"testing"
)

func TestRankedPrizesSkipsBestBeginner(t *testing.T) {
	o := NewOptions()
	o.PrizeRankings = []string{"Best Design", "best beginner", "Best Beginner Hack", "Best AI"}

	if got := o.RankedPrizes(); !slices.Equal(got, []string{"Best Design", "Best AI"}) {
		t.Fatalf("RankedPrizes should leave out Best Beginner, got %v", got)
	}
	if _, ok := o.FindPrize("Best Beginner"); ok {
		t.Fatalf("Best Beginner should not be rankable")
	}
	if prize, ok := o.FindPrize("best design"); !ok || prize != "Best Design" {
		t.Fatalf("FindPrize(best design) = %q, %v", prize, ok)
	}
}

func TestRankedPrizesSkipsNone(t *testing.T) {
	o := NewOptions()
	o.PrizeRankings = []string{"Best Design", "None", " none ", "Virality"}

	if got := o.RankedPrizes(); !slices.Equal(got, []string{"Best Design", "Virality"}) {
		t.Fatalf("RankedPrizes should leave out None, got %v", got)
	}
	if _, ok := o.FindPrize("None"); ok {
		t.Fatalf("None should not be rankable")
	}
	if IsNoHackUTDTrack("None of the above") {
		t.Fatalf("only the exact answer None means no track")
	}
}
