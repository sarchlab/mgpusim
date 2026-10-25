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
