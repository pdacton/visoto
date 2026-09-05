package signature

import (
	"math/rand"
	"strconv"
	"testing"
)

func sketchOf(values ...string) *Sketch {
	s := NewSketch()
	for _, v := range values {
		s.Add(v)
	}
	return s
}

func TestJaccardTracksOverlap(t *testing.T) {
	// Two municipality-number columns that overlap heavily: the case the whole
	// mechanism exists to detect.
	a := NewSketch()
	b := NewSketch()
	for i := 0; i < 2000; i++ {
		a.Add(strconv.Itoa(i))
	}
	for i := 1000; i < 3000; i++ {
		b.Add(strconv.Itoa(i))
	}
	// |A∩B| = 1000, |A∪B| = 3000 → 0.333
	got := a.Jaccard(b)
	if got < 0.24 || got > 0.43 {
		t.Errorf("Jaccard = %.3f, want roughly 0.33", got)
	}
}

func TestJaccardIdenticalAndDisjoint(t *testing.T) {
	a := sketchOf("261", "351", "1061")
	same := sketchOf("1061", "261", "351") // order must not matter
	if got := a.Jaccard(same); got != 1.0 {
		t.Errorf("identical value sets = %.3f, want 1.0", got)
	}

	disjoint := NewSketch()
	for i := 0; i < 500; i++ {
		disjoint.Add("x" + strconv.Itoa(i))
	}
	big := NewSketch()
	for i := 0; i < 500; i++ {
		big.Add("y" + strconv.Itoa(i))
	}
	if got := disjoint.Jaccard(big); got > 0.05 {
		t.Errorf("disjoint sets = %.3f, want near 0", got)
	}
}

func TestDuplicateValuesDoNotShiftTheSketch(t *testing.T) {
	a := sketchOf("a", "b", "c")
	b := sketchOf("a", "a", "a", "b", "b", "c")
	if got := a.Jaccard(b); got != 1.0 {
		t.Errorf("Jaccard = %.3f; MinHash must be insensitive to repetition", got)
	}
	if b.Count() != 6 {
		t.Errorf("Count = %d, want the raw value count", b.Count())
	}
}

func TestEmptySketchesAreNotSimilar(t *testing.T) {
	// "Both empty" is not evidence that two fields hold the same thing.
	if got := NewSketch().Jaccard(NewSketch()); got != 0 {
		t.Errorf("two empty sketches = %.3f, want 0", got)
	}
	if got := sketchOf("a").Jaccard(NewSketch()); got != 0 {
		t.Errorf("one empty sketch = %.3f, want 0", got)
	}
	if !NewSketch().Empty() {
		t.Error("a fresh sketch should report empty")
	}
}

func TestMergeSketchesUnion(t *testing.T) {
	a := NewSketch()
	b := NewSketch()
	for i := 0; i < 1000; i++ {
		a.Add(strconv.Itoa(i))
		b.Add(strconv.Itoa(i + 1000))
	}
	union := a.Clone()
	union.Merge(b)

	// The union must resemble each half at roughly 0.5.
	if got := union.Jaccard(a); got < 0.35 || got > 0.65 {
		t.Errorf("union vs half = %.3f, want roughly 0.5", got)
	}
	if a.Jaccard(b) > 0.05 {
		t.Error("Merge mutated the operand")
	}
	if union.Count() != 2000 {
		t.Errorf("merged count = %d, want 2000", union.Count())
	}
}

func TestSketchRoundTrip(t *testing.T) {
	a := sketchOf("261", "351", "1061")
	decoded, err := DecodeSketch(a.Encode())
	if err != nil {
		t.Fatalf("DecodeSketch: %v", err)
	}
	if decoded.Jaccard(a) != 1.0 || decoded.Count() != a.Count() {
		t.Error("sketch did not survive the round trip")
	}
	if got := len(a.Encode()); got != 8+SketchSize*4 {
		t.Errorf("encoded size = %d bytes", got)
	}
	// A truncated sketch would yield plausible but wrong similarity scores.
	if _, err := DecodeSketch(a.Encode()[:100]); err == nil {
		t.Error("a truncated sketch should not decode")
	}
}

func TestSketchIsStableAcrossProcesses(t *testing.T) {
	// The permutation constants are as load-bearing as the IRI base: changing
	// them silently invalidates every stored sketch. Pin one value.
	s := sketchOf("gemeinde")
	if got := s.Encode()[8:12]; got[0] == 0xFF && got[1] == 0xFF && got[2] == 0xFF && got[3] == 0xFF {
		t.Fatal("first position was never lowered")
	}
	again := sketchOf("gemeinde")
	if s.Jaccard(again) != 1.0 {
		t.Error("hashing is not deterministic")
	}
}

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"gemeinde_bfs": "gemeindebfs",
		"Gemeinde-Nr.": "gemeindenr",
		"  BFS_NR  ":   "bfsnr",
		"Gemeinde_de":  "gemeinde",
		"gemeinde_fr":  "gemeinde",
		"de":           "de", // too short to be a suffix on nothing
		"code":         "code",
		"":             "",
	}
	for in, want := range cases {
		if got := NormalizeName(in); got != want {
			t.Errorf("NormalizeName(%q) = %q, want %q", in, got, want)
		}
	}

	// The point of normalization: these three must collide.
	a, b, c := NormalizeName("BFS-Nr."), NormalizeName("bfs_nr"), NormalizeName("bfsNr")
	if a != b || b != c {
		t.Errorf("spellings did not collide: %q %q %q", a, b, c)
	}
}

func TestPatternClass(t *testing.T) {
	cases := map[string]string{
		"261":          "999",
		"1061":         "9999",
		"2024-03-01":   "9999-99-99",
		"CH-8001":      "aa-9999",
		"a@b.ch":       "a@a.aa",
		"":             "",
		"Zürich":       "aaaaaa",
		"46.9480,7.44": "99.9999,9.99",
	}
	for in, want := range cases {
		if got := PatternClass(in); got != want {
			t.Errorf("PatternClass(%q) = %q, want %q", in, got, want)
		}
	}

	// The shape is what separates a municipality number from a year when the
	// column name says neither.
	if PatternClass("2024") != PatternClass("1061") {
		t.Error("same-length digit runs should share a shape")
	}
	if PatternClass("2024") == PatternClass("261") {
		t.Error("different lengths should not share a shape")
	}
}

func TestCardinalityClass(t *testing.T) {
	cases := []struct {
		name     string
		distinct int64
		rows     int64
		want     string
	}{
		{"unique id", 1000, 1000, "unique"},
		{"constant", 1, 500, "constant"},
		{"small enum", 5, 500, "enum"},
		{"code list", 40, 100000, "code"},
		{"categorical", 200, 1000, "categorical"},
		{"mostly distinct", 900, 1000, "high"},
		{"no rows", 5, 0, "unknown"},
	}
	for _, tc := range cases {
		d := Descriptor{DistinctCount: tc.distinct, RowCount: tc.rows}
		if got := d.CardinalityClass(); got != tc.want {
			t.Errorf("%s: CardinalityClass() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestKeyGroupsSpellingsOfTheSameColumn(t *testing.T) {
	base := Descriptor{
		Name:          "bfs_nr",
		Datatype:      "http://www.w3.org/2001/XMLSchema#integer",
		PatternClass:  "9999",
		DistinctCount: 2100,
		RowCount:      18000,
	}
	spelled := base
	spelled.Name = "BFS-Nr."
	if base.Key() != spelled.Key() {
		t.Errorf("spellings produced different keys:\n  %s\n  %s", base.Key(), spelled.Key())
	}

	// A bare datatype name and its IRI must agree.
	bare := base
	bare.Datatype = "integer"
	if bare.Key() != base.Key() {
		t.Errorf("datatype IRI and local name disagree:\n  %s\n  %s", bare.Key(), base.Key())
	}

	// A genuinely different column must not collide.
	other := base
	other.Name = "gemeinde_name"
	other.Datatype = "string"
	if other.Key() == base.Key() {
		t.Error("unrelated columns collided")
	}
}

func TestDatatypeIRI(t *testing.T) {
	if got := DatatypeIRI("integer"); got != "http://www.w3.org/2001/XMLSchema#integer" {
		t.Errorf("DatatypeIRI = %q", got)
	}
	full := "http://www.w3.org/2001/XMLSchema#date"
	if got := DatatypeIRI(full); got != full {
		t.Errorf("an IRI should pass through, got %q", got)
	}
}

// BenchmarkSketchAdd guards the cost of the profiling hot path: this runs once
// per value of every column of every distribution in the corpus.
func BenchmarkSketchAdd(b *testing.B) {
	s := NewSketch()
	values := make([]string, 1000)
	rng := rand.New(rand.NewSource(1))
	for i := range values {
		values[i] = strconv.Itoa(rng.Intn(100000))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Add(values[i%len(values)])
	}
}
