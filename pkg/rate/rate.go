// Package rate derives PDU rates and detects counter discontinuities
// per draft-choudhary-grow-bmp-bgp-msg-stats-00 Section 7–8.
package rate

import (
	"math"
	"time"

	"github.com/asechoud/bmp-bgp-msg-stats/pkg/stats"
)

// Sample is one observation of a counter.
type Sample struct {
	Value     uint64
	At        time.Time
	Width     stats.Width
	Present   bool
}

// Delta is the meaningful difference between two samples.
type Delta struct {
	Delta         uint64
	Seconds       float64
	RatePerSecond float64
	Discontinuity bool
	Reason        string
}

// Diff computes (c2−c1)/(t2−t1) with wrap and discontinuity handling.
//
// Rules from Section 7:
//   - any decrease of a 64-bit Counter → discontinuity
//   - decrease of a 32-bit Counter other than wrap at 2^32 → discontinuity
//   - 32-bit wrap (c2 < c1 with Width32) → treated as wrap, not discontinuity
func Diff(prev, curr Sample) Delta {
	if !prev.Present || !curr.Present {
		return Delta{Discontinuity: true, Reason: "type not present in both samples"}
	}
	if curr.At.Before(prev.At) || curr.At.Equal(prev.At) {
		return Delta{Discontinuity: true, Reason: "non-increasing timestamp"}
	}
	sec := curr.At.Sub(prev.At).Seconds()
	if curr.Value >= prev.Value {
		d := curr.Value - prev.Value
		return Delta{Delta: d, Seconds: sec, RatePerSecond: float64(d) / sec}
	}
	// Decrease.
	if prev.Width == stats.Width32 {
		// Likely wrap: treat as continuous wrap.
		d := (math.MaxUint32 - prev.Value) + curr.Value + 1
		return Delta{Delta: d, Seconds: sec, RatePerSecond: float64(d) / sec, Reason: "32-bit wrap"}
	}
	return Delta{
		Discontinuity: true,
		Reason:        "64-bit counter decreased (reset/clear/failover)",
		Seconds:       sec,
	}
}

// SessionDiscontinuity is true when a new BMP Initiation arrives
// (Section 7: start of a new BMP session).
func SessionDiscontinuity() string {
	return "new BMP session (Initiation)"
}
