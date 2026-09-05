// Package signature gives a field a content-derived identity that is shared
// across distributions, publishers and portals.
//
// The problem it solves: after profiling 50 000 distributions the store holds
// roughly a million field descriptions, each sealed inside its own distribution.
// Nothing in that says the `bfs_nr` column of a population file and the
// `gemeinde_id` column of an energy file hold values from the same universe.
// Without that link the harvest is a pile of isolated schemas.
//
// A signature is one node many fields point at. It buys three things: join
// discovery across publishers who never agreed on a column name; classification
// transfer, since annotating the signature annotates every field beneath it
// (~10^4 classifications instead of ~10^6); and code-list detection, because a
// low-cardinality signature recurring across dozens of publishers is an
// undocumented code list.
//
// The sketch has to be computed while the values stream past during profiling —
// retrofitting it would mean re-downloading and re-profiling the whole corpus.
// That is why this package exists before the profiler that will feed it.
package signature

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
)

// SketchSize is the number of MinHash permutations. 128 puts the standard error
// of the Jaccard estimate at about 1/sqrt(128) ≈ 9 %, which is ample for ranking
// join candidates, and costs 512 bytes per field — small enough to keep one for
// every field in the corpus.
const SketchSize = 128

// Sketch is a MinHash sketch over a field's distinct values. It estimates the
// Jaccard similarity of two value sets without either set being retained, which
// is what makes cross-distribution comparison affordable — and what keeps the
// pipeline from having to store other people's data to compare it.
type Sketch struct {
	mins [SketchSize]uint32
	// seen counts values added, so an empty sketch is distinguishable from one
	// whose values all hashed high.
	seen uint64
}

// NewSketch returns an empty sketch. Every position starts at the maximum, so
// the first value added always lowers it.
func NewSketch() *Sketch {
	s := &Sketch{}
	for i := range s.mins {
		s.mins[i] = math.MaxUint32
	}
	return s
}

// Add folds one value into the sketch. Adding the same value twice is a no-op by
// construction, so the caller need not deduplicate first.
func (s *Sketch) Add(value string) {
	h := hash64(value)
	s.seen++
	// One real hash per value, then k cheap universal-hash permutations of it.
	// Hashing k times per value would dominate profiling cost for no accuracy.
	for i := range s.mins {
		p := permute(h, uint64(i))
		if p < s.mins[i] {
			s.mins[i] = p
		}
	}
}

// Count returns how many values were added, distinct or not.
func (s *Sketch) Count() uint64 { return s.seen }

// Empty reports whether the sketch has seen no values.
func (s *Sketch) Empty() bool { return s == nil || s.seen == 0 }

// Jaccard estimates the Jaccard similarity of the two underlying value sets, as
// the fraction of positions where the minima agree. Two empty sketches are
// reported as dissimilar rather than identical: "both fields are empty" is not
// evidence that they hold the same thing.
func (s *Sketch) Jaccard(other *Sketch) float64 {
	if s.Empty() || other.Empty() {
		return 0
	}
	var agree int
	for i := range s.mins {
		if s.mins[i] == other.mins[i] {
			agree++
		}
	}
	return float64(agree) / float64(SketchSize)
}

// Merge folds other into s, so the result sketches the union of the two value
// sets. This is what lets one signature accumulate the value universe of every
// field that maps to it, rather than keeping a sketch per field forever.
func (s *Sketch) Merge(other *Sketch) {
	if other == nil {
		return
	}
	for i := range s.mins {
		if other.mins[i] < s.mins[i] {
			s.mins[i] = other.mins[i]
		}
	}
	s.seen += other.seen
}

// Clone returns an independent copy.
func (s *Sketch) Clone() *Sketch {
	c := *s
	return &c
}

// Encode serializes the sketch for storage: the value count followed by the
// minima, little-endian.
func (s *Sketch) Encode() []byte {
	buf := make([]byte, 8+SketchSize*4)
	binary.LittleEndian.PutUint64(buf[:8], s.seen)
	for i, m := range s.mins {
		binary.LittleEndian.PutUint32(buf[8+i*4:], m)
	}
	return buf
}

// DecodeSketch reads a sketch back. A wrong length is an error rather than a
// partial sketch, because a silently truncated sketch would produce plausible
// but wrong similarity scores.
func DecodeSketch(buf []byte) (*Sketch, error) {
	if len(buf) != 8+SketchSize*4 {
		return nil, fmt.Errorf("sketch is %d bytes, want %d", len(buf), 8+SketchSize*4)
	}
	s := &Sketch{seen: binary.LittleEndian.Uint64(buf[:8])}
	for i := range s.mins {
		s.mins[i] = binary.LittleEndian.Uint32(buf[8+i*4:])
	}
	return s, nil
}

// hash64 is the single hash per value; the permutations derive from it.
func hash64(v string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(v))
	return h.Sum64()
}

// permute applies the i-th universal hash to h and folds it to 32 bits.
//
// The odd multipliers are arbitrary but fixed: changing them re-mints every
// sketch in the store, so they are as load-bearing as the IRI base.
func permute(h, i uint64) uint32 {
	a := 2*i + 0x9E3779B97F4A7C15
	b := 2*i + 0xBF58476D1CE4E5B9
	x := h*a + b
	// Final avalanche, so the low bits of the multiply do not dominate.
	x ^= x >> 33
	x *= 0xFF51AFD7ED558CCD
	x ^= x >> 33
	return uint32(x >> 32)
}
