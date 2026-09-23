package runner_test

import (
	"path/filepath"
	"testing"

	"github.com/sarchlab/mgpusim/v5/nvidia/platform"
	"github.com/sarchlab/mgpusim/v5/nvidia/runner"
)

func TestRunVectorAddTrace(t *testing.T) {
	for _, device := range []platform.Device{platform.H100(), platform.A100()} {
		t.Run(device.Name, func(t *testing.T) {
			result, err := runner.Run(runner.Options{
				TraceDir:   "../trace/testdata/vectoradd",
				Device:     device,
				OutputFile: filepath.Join(t.TempDir(), "akita_sim"),
			})
			if err != nil {
				t.Fatal(err)
			}

			if result.NumKernels != 1 {
				t.Errorf("expected 1 kernel, got %d", result.NumKernels)
			}

			if result.NumWarps != 16 {
				t.Errorf("expected 16 warps, got %d", result.NumWarps)
			}

			if result.NumInsts != 272 {
				t.Errorf("expected all 272 instructions to run, got %d",
					result.NumInsts)
			}

			if result.Cycles() <= device.LaunchOverheadLatency {
				t.Errorf("expected more than %d cycles, got %d",
					device.LaunchOverheadLatency, result.Cycles())
			}
		})
	}
}
