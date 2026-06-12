package l1cachebw

import "testing"

func TestInputFootprintScalesByReadGroup(t *testing.T) {
	tests := []struct {
		name        string
		numElements int
		wantElems   int
		wantBytes   uint64
	}{
		{name: "zero", numElements: 0, wantElems: 0, wantBytes: 0},
		{name: "one group", numElements: 1, wantElems: 256, wantBytes: 1024},
		{name: "exact group", numElements: 256, wantElems: 256, wantBytes: 1024},
		{name: "second group", numElements: 257, wantElems: 512, wantBytes: 2048},
		{name: "four groups", numElements: 1024, wantElems: 1024, wantBytes: 4096},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			benchmark := &Benchmark{NumElements: test.numElements}

			if got := benchmark.inputElements(); got != test.wantElems {
				t.Fatalf("inputElements() = %d, want %d", got, test.wantElems)
			}
			if got := benchmark.inputFootprintBytes(); got != test.wantBytes {
				t.Fatalf("inputFootprintBytes() = %d, want %d", got, test.wantBytes)
			}
		})
	}
}

func TestTotalReadBytesUsesNumRepeats(t *testing.T) {
	benchmark := &Benchmark{NumElements: 1024, NumRepeats: 7}
	want := uint64(1024 * 7 * bytesPerRead)

	if got := benchmark.TotalReadBytes(); got != want {
		t.Fatalf("TotalReadBytes() = %d, want %d", got, want)
	}
}
