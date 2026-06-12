package backprop

import (
	"fmt"
	"strings"
	"testing"
)

func TestVerifyChecksHiddenOutputWhenLayerforwardRuns(t *testing.T) {
	benchmark := newVerificationBenchmarkForTest(4, 3)
	benchmark.hidden[1] += 1

	expectPanicContaining(t, func() {
		benchmark.Verify()
	}, "hidden output verification failed")
}

func TestVerifySkipsHiddenOutputWhenLayerforwardSkipped(t *testing.T) {
	benchmark := newVerificationBenchmarkForTest(4, 3)
	benchmark.SkipLayerforward = true
	benchmark.hidden = nil

	benchmark.Verify()
}

func TestVerifyChecksAdjustedWeightsWhenLayerforwardSkipped(t *testing.T) {
	benchmark := newVerificationBenchmarkForTest(4, 3)
	benchmark.SkipLayerforward = true
	benchmark.hidden = nil
	benchmark.weights[0] += 1

	expectPanicContaining(t, func() {
		benchmark.Verify()
	}, "adjusted-weight verification failed")
}

func newVerificationBenchmarkForTest(numInput, numHidden int) *Benchmark {
	benchmark := &Benchmark{
		NumInput:     numInput,
		numHidden:    numHidden,
		learningRate: 0.3,
		momentum:     0.5,
	}

	benchmark.cpuBackprop()
	benchmark.hidden = append([]float32(nil), benchmark.cpuHidden...)
	benchmark.weights = append([]float32(nil), benchmark.cpuWeights...)
	benchmark.cpuHidden = nil
	benchmark.cpuWeights = nil

	return benchmark
}

func expectPanicContaining(t *testing.T, fn func(), want string) {
	t.Helper()

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic containing %q", want)
		}

		got := fmt.Sprint(recovered)
		if !strings.Contains(got, want) {
			t.Fatalf("panic = %q, want substring %q", got, want)
		}
	}()

	fn()
}
