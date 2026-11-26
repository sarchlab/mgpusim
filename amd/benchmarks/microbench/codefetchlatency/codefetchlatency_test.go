package codefetchlatency

import (
	"strings"
	"testing"
)

func TestCheckParameters(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		body, loops, blocks, threads int
		rejected                     string
	}{
		{"default launch", 256, 1, 1, 64, ""},
		{"largest chain", 65536, 16, 1, 64, ""},
		{"largest work-group", 16, 1, 1, 1024, ""},
		{"largest grid", 16, 1, 4194303, 1024, ""},
		{"unsupported body size", 100, 1, 1, 64, "no kernel with 100"},
		{"negative body size", -16, 1, 1, 64, "no kernel with -16"},
		{"chain too long", 65536, 17, 1, 64, "operations per thread"},
		{"negative loops", 16, -1, 1, 64, "operations per thread"},
		{"work-group above the gfx942 limit", 16, 1, 1, 1025, "threads per block"},
		{"work-group that narrows to zero", 16, 1, 1, 65536, "threads per block"},
		{"negative work-group", 16, 1, 1, -64, "threads per block"},
		{"negative block count", 16, 1, -1, 64, "1 block or more"},
		{"grid beyond uint32", 16, 1, 4194304, 1024, "1 block or more"},
		{"grid that wraps an int", 16, 1, 1 << 62, 1024, "1 block or more"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkParameters(tc.body, tc.loops, tc.blocks, tc.threads)
			switch {
			case tc.rejected == "" && err != nil:
				t.Fatalf("expected the launch to be accepted, got %v", err)
			case tc.rejected != "" && err == nil:
				t.Fatal("expected the launch to be rejected")
			case tc.rejected != "" && !strings.Contains(err.Error(), tc.rejected):
				t.Fatalf("expected an error about %q, got %v", tc.rejected, err)
			}
		})
	}
}

func TestZeroParametersTakeTheDefaults(t *testing.T) {
	b := &Benchmark{State: &State{}}
	got := [4]int{b.bodyOps(), b.loops(), b.numBlocks(), b.threadsPerBlock()}
	if want := [4]int{256, 1, 1, 64}; got != want {
		t.Fatalf("expected the defaults %v, got %v", want, got)
	}
}
