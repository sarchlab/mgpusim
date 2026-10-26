package kmeans

import (
	"strings"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
)

func TestStepRejectsNonPositiveIterationLimitBeforeSubmitting(t *testing.T) {
	for _, limit := range []int{0, -1} {
		b := &Benchmark{State: &State{MaxIter: limit}}
		b.Saved = benchmarks.NewSaved(b.State, b.checkpointConfig, b.bind)
		if err := b.Step(); err == nil || !strings.Contains(err.Error(), "MaxIter > 0") {
			t.Fatalf("MaxIter=%d: error %v", limit, err)
		}
		if b.Phase != 0 {
			t.Fatal("invalid input advanced host continuation")
		}
	}
}

func TestCentroidMismatchReturnsError(t *testing.T) {
	b := &Benchmark{State: &State{NumClusters: 2, NumFeatures: 2}}
	err := b.compareCentroids([]float32{1, 2, 3, 4}, []float32{1, 2, 3, 5})
	if err == nil || !strings.Contains(err.Error(), "centroid 1 feature 1") {
		t.Fatalf("comparison error: %v", err)
	}
	if err := b.compareCentroids([]float32{1, 2, 3, 4}, []float32{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
}

func TestVerificationPropagatesCentroidMismatch(t *testing.T) {
	b := &Benchmark{
		State: &State{
			NumPoints:   2,
			NumClusters: 1,
			NumFeatures: 1,
			MaxIter:     1,
			HFeatures: []float32{
				1,
				3,
			},
			HClusters: []float32{
				5,
			},
		},
	}
	if err := b.verify(); err == nil || !strings.Contains(err.Error(), "centroid 0 feature 0") {
		t.Fatalf("verification error: %v", err)
	}
}
