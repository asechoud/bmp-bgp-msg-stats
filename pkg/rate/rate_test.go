package rate

import (
	"testing"
	"time"

	"github.com/asechoud/bmp-bgp-msg-stats/pkg/stats"
)

func TestRateHappyPath(t *testing.T) {
	t0 := time.Unix(100, 0)
	t1 := time.Unix(110, 0)
	d := Diff(
		Sample{Value: 1000, At: t0, Width: stats.Width64, Present: true},
		Sample{Value: 1200, At: t1, Width: stats.Width64, Present: true},
	)
	if d.Discontinuity || d.Delta != 200 || d.RatePerSecond != 20 {
		t.Fatalf("%+v", d)
	}
}

func Test64BitDecreaseIsDiscontinuity(t *testing.T) {
	t0 := time.Unix(100, 0)
	t1 := time.Unix(110, 0)
	d := Diff(
		Sample{Value: 1000, At: t0, Width: stats.Width64, Present: true},
		Sample{Value: 10, At: t1, Width: stats.Width64, Present: true},
	)
	if !d.Discontinuity {
		t.Fatal("expected discontinuity")
	}
}

func Test32BitWrap(t *testing.T) {
	t0 := time.Unix(100, 0)
	t1 := time.Unix(110, 0)
	d := Diff(
		Sample{Value: 0xfffffffe, At: t0, Width: stats.Width32, Present: true},
		Sample{Value: 1, At: t1, Width: stats.Width32, Present: true},
	)
	if d.Discontinuity {
		t.Fatalf("wrap should not be discontinuity: %+v", d)
	}
	if d.Delta != 3 {
		t.Fatalf("delta want 3 got %d", d.Delta)
	}
}

func TestAbsentType(t *testing.T) {
	t0 := time.Unix(100, 0)
	t1 := time.Unix(110, 0)
	d := Diff(
		Sample{Value: 1, At: t0, Width: stats.Width32, Present: false},
		Sample{Value: 2, At: t1, Width: stats.Width32, Present: true},
	)
	if !d.Discontinuity {
		t.Fatal("absent type must not be treated as zero")
	}
}
