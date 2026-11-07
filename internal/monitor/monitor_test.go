package monitor

import (
	"testing"
	"time"

	"github.com/pdacton/visoto/internal/config"
)

func TestDailyUptime(t *testing.T) {
	cfg := &config.ApplicationConfig{SparqlEndpoints: []config.SparqlEndpoint{
		{Name: "A", Slug: "a", URL: "http://a/q", Monitor: true},
		{Name: "Off", Slug: "off", URL: "http://off/q", Monitor: false},
		{Name: "B", Slug: "b", URL: "http://b/q", Monitor: true},
	}}
	m, err := New(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.db.Close()

	now := time.Date(2026, 11, 7, 15, 0, 0, 0, time.UTC)
	insert := func(ep string, at time.Time, status string) {
		t.Helper()
		if _, err := m.db.Exec(
			`INSERT INTO endpoint_metrics (endpoint, measured_at, duration_ms, status) VALUES (?,?,?,?)`,
			ep, at.Unix(), 10, status); err != nil {
			t.Fatal(err)
		}
	}
	insert("http://a/q", now.Add(-time.Hour), "ok")
	insert("http://a/q", now.Add(-2*time.Hour), "timeout")
	insert("http://a/q", now.AddDate(0, 0, -2), "ok")                        // Nov 5
	insert("http://a/q", now.AddDate(0, 0, -3), "ok")                        // Nov 4: before the window
	insert("http://off/q", now.Add(-time.Hour), "ok")                        // unmonitored: never listed
	insert("http://a/q", time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), "ok") // midnight belongs to today

	got, err := m.DailyUptime(3, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Slug != "a" || got[1].Slug != "b" {
		t.Fatalf("want monitored endpoints a, b in config order, got %+v", got)
	}

	want := []DayUptime{
		{Date: "2026-11-05", Probes: 1, OK: 1},
		{Date: "2026-11-06", Probes: 0, OK: 0},
		{Date: "2026-11-07", Probes: 3, OK: 2},
	}
	if len(got[0].Days) != len(want) {
		t.Fatalf("a: want %d days, got %+v", len(want), got[0].Days)
	}
	for i, w := range want {
		if got[0].Days[i] != w {
			t.Errorf("a day %d: want %+v, got %+v", i, w, got[0].Days[i])
		}
	}
	for i, d := range got[1].Days {
		if d.Probes != 0 || d.Date != want[i].Date {
			t.Errorf("b day %d: want empty %s, got %+v", i, want[i].Date, d)
		}
	}
}
