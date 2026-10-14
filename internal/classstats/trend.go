package classstats

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Trend is the geometry of a metric card's sparkline: one class's daily counts
// over a window, laid out in a TrendWidth × TrendHeight viewBox. The SVG markup
// lives in the sparqlMetricValue template; only the numbers are computed here.
type Trend struct {
	Path  string  // SVG path data; a failed day (nil count) starts a new segment
	EndX  float64 // the latest counted point, drawn as a dot
	EndY  float64
	Days  int      // window length
	From  string   // oldest counted day in the window
	To    string   // latest counted day
	First int64    // count on From
	Last  int64    // count on To
	Pct   *float64 // change From→To in percent; nil with a single point or First == 0
	Dir   int      // sign of Pct after rounding to one decimal: 1, -1 or 0
}

const (
	TrendWidth  = 100
	TrendHeight = 30
	trendPad    = 3 // keeps the stroke and dot inside the viewBox vertically
)

// Trend builds the sparkline for classIRI on the endpoint slug (resolved
// through class_stats_from) over the last `days` days. nil when the endpoint
// has no statistics or the class was never counted in the window.
func (c *Collector) Trend(slug, classIRI string, days int) (*Trend, error) {
	_, src := c.For(slug)
	if src == "" {
		return nil, nil
	}
	pts, err := c.Series(src, classIRI, days)
	if err != nil {
		return nil, err
	}
	return buildTrend(pts, c.now().UTC(), days), nil
}

// buildTrend lays out pts (oldest first, as Series returns them). X is the
// calendar day within the window ending today — not the point index — so sparse
// history is spaced honestly and a lone point sits at its real date. Y spans the
// window's min..max; a flat series is drawn at mid-height.
func buildTrend(pts []Point, today time.Time, days int) *Trend {
	if days < 2 {
		days = 2
	}
	start := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1))

	var lo, hi int64 = math.MaxInt64, math.MinInt64
	t := &Trend{Days: days}
	for _, p := range pts {
		if p.Count == nil {
			continue
		}
		n := *p.Count
		lo, hi = min(lo, n), max(hi, n)
		if t.From == "" {
			t.From, t.First = p.Day, n
		}
		t.To, t.Last = p.Day, n
	}
	if t.From == "" {
		return nil
	}

	x := func(day string) (float64, bool) {
		d, err := time.Parse("2006-01-02", day)
		if err != nil {
			return 0, false
		}
		i := d.Sub(start).Hours() / 24
		return round2(i / float64(days-1) * TrendWidth), true
	}
	y := func(n int64) float64 {
		if hi == lo {
			return TrendHeight / 2
		}
		frac := float64(n-lo) / float64(hi-lo)
		return round2(TrendHeight - trendPad - frac*(TrendHeight-2*trendPad))
	}

	var b strings.Builder
	pen := false // false: the next point opens a segment (start, or after a gap)
	for _, p := range pts {
		if p.Count == nil {
			pen = false
			continue
		}
		px, ok := x(p.Day)
		if !ok {
			continue
		}
		py := y(*p.Count)
		cmd := "L"
		if !pen {
			cmd = "M"
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%s%g,%g", cmd, px, py)
		pen = true
		t.EndX, t.EndY = px, py
	}
	t.Path = b.String()

	if t.From != t.To && t.First != 0 {
		pct := float64(t.Last-t.First) / float64(t.First) * 100
		t.Pct = &pct
		switch r := math.Round(pct * 10); {
		case r > 0:
			t.Dir = 1
		case r < 0:
			t.Dir = -1
		}
	}
	return t
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// PctText is Pct for display: one decimal, signed, "0%" when it rounds to zero.
func (t *Trend) PctText() string {
	if t == nil || t.Pct == nil {
		return ""
	}
	if t.Dir == 0 {
		return "0%"
	}
	return fmt.Sprintf("%+.1f%%", *t.Pct)
}
