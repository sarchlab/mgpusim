package memlatencychase

import "testing"

func TestChaseIterationsUsesHardwareDefaultForL2Region(t *testing.T) {
	benchmark := &Benchmark{
		NumElements: l1ResidentChaseElements + 1,
		NumChases:   256,
	}

	if got := benchmark.chaseIterations(); got != hardwareDefaultNumHops {
		t.Fatalf(
			"L2-region chase iterations = %d, want hardware default %d",
			got, hardwareDefaultNumHops)
	}
}

func TestChaseIterationsPreservesL1ConfiguredCount(t *testing.T) {
	benchmark := &Benchmark{
		NumElements: l1ResidentChaseElements,
		NumChases:   123,
	}

	if got := benchmark.chaseIterations(); got != 123 {
		t.Fatalf("L1-region chase iterations = %d, want configured count 123", got)
	}
}

func TestChaseIterationsHonorsExplicitL2Count(t *testing.T) {
	benchmark := &Benchmark{NumElements: l1ResidentChaseElements + 1}
	benchmark.SetNumChases(512)

	if got := benchmark.chaseIterations(); got != 512 {
		t.Fatalf("explicit L2-region chase iterations = %d, want 512", got)
	}
}
