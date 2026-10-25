package kmeans

import (
	"strings"
	"testing"
)

func TestStepRejectsNonPositiveIterationLimitBeforeSubmitting(t *testing.T) {
	for _, limit := range []int{0, -1} {
		b := &Benchmark{MaxIter: limit}
		if err := b.Step(); err == nil || !strings.Contains(err.Error(), "MaxIter > 0") {
			t.Fatalf("MaxIter=%d: error %v", limit, err)
		}
		if b.Phase != 0 || b.configCaptured {
			t.Fatal("invalid input advanced host continuation")
		}
	}
}

func TestCentroidMismatchReturnsError(t *testing.T) {
	b := &Benchmark{NumClusters: 2, NumFeatures: 2}
	err := b.compareCentroids([]float32{1, 2, 3, 4}, []float32{1, 2, 3, 5})
	if err == nil || !strings.Contains(err.Error(), "centroid 1 feature 1") {
		t.Fatalf("comparison error: %v", err)
	}
	if err := b.compareCentroids([]float32{1, 2, 3, 4}, []float32{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
}

func TestVerificationPropagatesCentroidMismatch(t *testing.T) {
	b := &Benchmark{NumPoints: 2, NumClusters: 1, NumFeatures: 1, MaxIter: 1,
		hFeatures: []float32{1, 3}, hClusters: []float32{5}}
	if err := b.verify(); err == nil || !strings.Contains(err.Error(), "centroid 0 feature 0") {
		t.Fatalf("verification error: %v", err)
	}
}
