package runner_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/mgpusim/v5/nvidia/platform"
	"github.com/sarchlab/mgpusim/v5/nvidia/runner"
)

func TestExplicitOutputReplacesPreviousRun(t *testing.T) {
	for _, name := range []string{"recording", "recording.sqlite3"} {
		t.Run(name, func(t *testing.T) { testOutputRerun(t, name) })
	}
}

func testOutputRerun(t *testing.T, name string) {
	t.Helper()
	base := filepath.Join(t.TempDir(), name)
	opts := runner.Options{
		TraceDir: "../trace/testdata/vectoradd", Device: platform.H100(),
		VisTracing: true, OutputFile: base,
	}
	first, err := runner.Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	path := base + ".sqlite3" // Preserve beta.10's base-name convention.
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE previous_run (value INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := runner.Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("rerun changed results: %+v != %+v", first, second)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	err = db.QueryRowContext(t.Context(),
		"SELECT count(*) FROM sqlite_master WHERE name = 'previous_run'").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("rerun retained the previous recording")
	}
}

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

// A thread block whose warps were not parsed used to stall the SM forever,
// so run the real-format traces end to end.
func TestRunRealTraceFormats(t *testing.T) {
	for _, dir := range []string{"atax-v5", "atax-v6"} {
		t.Run(dir, func(t *testing.T) {
			result, err := runner.Run(runner.Options{
				TraceDir: "../trace/testdata/" + dir,
				Device:   platform.H100(),
			})
			if err != nil {
				t.Fatal(err)
			}

			if result.NumWarps != 2 || result.NumInsts != 60 {
				t.Errorf("expected 2 warps and 60 instructions, got %d and %d",
					result.NumWarps, result.NumInsts)
			}
		})
	}
}
