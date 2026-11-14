package sample

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
)

func TestFinishCompletesISADebugLogs(t *testing.T) {
	t.Chdir(t.TempDir())
	oldDebug, oldMonitor := *isaDebug, *disableAkitaRTM
	*isaDebug, *disableAkitaRTM = true, true
	t.Cleanup(func() { *isaDebug, *disableAkitaRTM = oldDebug, oldMonitor })

	config := Config{GPUIDs: []int{1, 2}}
	s, rt, finish := config.Build()
	t.Cleanup(func() {
		if err := finish(); err != nil {
			t.Error(err)
		}
	})
	b := fir.NewBenchmark(rt)
	b.Length, b.NumTapsParam = 1024, 4
	b.SelectGPU([]int{1})
	b.SetVerification(true)
	r := runner.New(s, rt)
	r.AddBenchmark(b)
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	if err := finish(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		gpu   string
		waves int
	}{{"1", 16}, {"2", 0}} {
		path := filepath.Join(".", "isa_GPU["+tc.gpu+"].CU.debug")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		assertWavefrontLog(t, data, tc.waves)
		if err := finish(); err != nil {
			t.Fatal(err)
		}
		again, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(data) {
			t.Fatal("repeated finish modified ISA output")
		}
	}
}

func assertWavefrontLog(t *testing.T, data []byte, want int) {
	t.Helper()
	var rows []struct {
		WG [3]int `json:"wg"`
		WF int    `json:"wf"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	waves := make(map[[4]int]bool)
	for _, row := range rows {
		waves[[4]int{row.WG[0], row.WG[1], row.WG[2], row.WF}] = true
	}
	if len(waves) != want {
		t.Fatalf("logged %d wavefronts, want %d", len(waves), want)
	}
}
