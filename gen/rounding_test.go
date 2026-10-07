package gen

import "testing"

func TestNoiseInterpolationRoundsBeforeAddition(t *testing.T) {
	for _, sample := range []struct {
		t, a, b, want float64
	}{
		{1 + 0x1p-27, -1, 0x1p-27, 0x1p-26},
	} {
		if got := lerp(sample.t, sample.a, sample.b); got != sample.want {
			t.Fatalf("interpolation fused multiply and add: got %.17g, want %.17g", got, sample.want)
		}
		if got := clampedLerp(1/sample.t, sample.a, sample.b); got != sample.a+float64((1/sample.t)*(sample.b-sample.a)) {
			t.Fatalf("clamped interpolation did not round its product: %.17g", got)
		}
	}
}

func TestSplineInterpolationRoundsBeforeAddition(t *testing.T) {
	for _, sample := range []struct {
		t, a, b, want float32
	}{
		{1 + 0x1p-13, -1, 0x1p-13, 0x1p-12},
	} {
		if got := lerpF(sample.t, sample.a, sample.b); got != sample.want {
			t.Fatalf("spline interpolation fused multiply and add: got %.9g, want %.9g", got, sample.want)
		}
	}
}
