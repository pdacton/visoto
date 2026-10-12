package classstats

import (
	"strings"
	"testing"
	"time"
)

func pt(day string, n int64) Point { return Point{Day: day, Count: &n} }

var trendToday = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

func TestBuildTrendSinglePoint(t *testing.T) {
	tr := buildTrend([]Point{pt("2026-10-11", 42)}, trendToday, 30)
	if tr == nil {
		t.Fatal("single point must still produce a trend")
	}
	if tr.Path != "M100,15" {
		t.Errorf("path = %q, want a lone point at the right edge, mid-height", tr.Path)
	}
	if tr.EndX != 100 || tr.EndY != 15 {
		t.Errorf("end = %v,%v", tr.EndX, tr.EndY)
	}
	if tr.Pct != nil || tr.PctText() != "" {
		t.Errorf("single point has no change, got %v", tr.Pct)
	}
}

func TestBuildTrendXFollowsDates(t *testing.T) {
	// 30-day window ending 10-11 starts 09-12: 09-27 is day 15 of 29.
	tr := buildTrend([]Point{pt("2026-09-27", 100), pt("2026-09-30", 100), pt("2026-10-11", 200)}, trendToday, 30)
	want := "M51.72,27 L62.07,27 L100,3"
	if tr.Path != want {
		t.Errorf("path = %q, want %q", tr.Path, want)
	}
	if tr.Dir != 1 || tr.PctText() != "+100.0%" {
		t.Errorf("dir/pct = %d %q", tr.Dir, tr.PctText())
	}
}

func TestBuildTrendFlatAndDown(t *testing.T) {
	flat := buildTrend([]Point{pt("2026-10-10", 5), pt("2026-10-11", 5)}, trendToday, 30)
	if !strings.HasSuffix(flat.Path, ",15") || flat.Dir != 0 || flat.PctText() != "0%" {
		t.Errorf("flat: path %q dir %d pct %q", flat.Path, flat.Dir, flat.PctText())
	}
	down := buildTrend([]Point{pt("2026-10-10", 10), pt("2026-10-11", 5)}, trendToday, 30)
	if down.Dir != -1 || down.PctText() != "-50.0%" {
		t.Errorf("down: dir %d pct %q", down.Dir, down.PctText())
	}
}

func TestBuildTrendGapBreaksLine(t *testing.T) {
	tr := buildTrend([]Point{pt("2026-10-01", 1), {Day: "2026-10-02"}, pt("2026-10-03", 2), pt("2026-10-04", 3)}, trendToday, 30)
	if strings.Count(tr.Path, "M") != 2 {
		t.Errorf("a failed day must start a new segment: %q", tr.Path)
	}
	if tr.First != 1 || tr.Last != 3 {
		t.Errorf("first/last = %d/%d", tr.First, tr.Last)
	}
}

func TestBuildTrendNoCounts(t *testing.T) {
	if buildTrend(nil, trendToday, 30) != nil || buildTrend([]Point{{Day: "2026-10-11"}}, trendToday, 30) != nil {
		t.Error("no counted point must yield nil")
	}
}
