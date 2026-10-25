package sample

import (
	"database/sql"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

func TestExplicitMetricFileReplacesPreviousRun(t *testing.T) {
	oldFlags, oldFilename, oldDisableRTM := flag.CommandLine, filenameFlag, disableAkitaRTM
	t.Cleanup(func() {
		flag.CommandLine, filenameFlag, disableAkitaRTM = oldFlags, oldFilename, oldDisableRTM
	})

	for _, name := range []string{"results", "results.sqlite3"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			flag.CommandLine = flag.NewFlagSet("runner", flag.ContinueOnError)
			filenameFlag = flag.CommandLine.String("metric-file-name", "metrics", "")
			disableAkitaRTM = flag.CommandLine.Bool("disable-rtm", true, "")
			if err := flag.CommandLine.Parse([]string{"-metric-file-name", path}); err != nil {
				t.Fatal(err)
			}

			first := new(assembly)
			first.initSimulation()
			first.simulation.DataRecorder().CreateTable("previous_run", struct{ Value int }{})
			if err := first.simulation.DataRecorder().Close(); err != nil {
				t.Fatal(err)
			}

			second := new(assembly)
			second.initSimulation()
			t.Cleanup(func() { _ = second.simulation.DataRecorder().Close() })
			// beta.10 treats the option as a base name and always appends
			// the suffix, even when the base name already ends in it.
			path += ".sqlite3"
			if _, err := os.Stat(path); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var tables int
			err = db.QueryRowContext(t.Context(),
				"SELECT count(*) FROM sqlite_master WHERE name = 'previous_run'").Scan(&tables)
			if err != nil {
				t.Fatal(err)
			}
			if tables != 0 {
				t.Fatal("the second run retained the previous recording")
			}
		})
	}
}

func TestEmptyMetricFileUsesUniqueDefault(t *testing.T) {
	oldFlags, oldFilename, oldDisableRTM := flag.CommandLine, filenameFlag, disableAkitaRTM
	t.Cleanup(func() {
		flag.CommandLine, filenameFlag, disableAkitaRTM = oldFlags, oldFilename, oldDisableRTM
	})
	t.Chdir(t.TempDir())
	flag.CommandLine = flag.NewFlagSet("runner", flag.ContinueOnError)
	filenameFlag = flag.CommandLine.String("metric-file-name", "metrics", "")
	disableAkitaRTM = flag.CommandLine.Bool("disable-rtm", true, "")
	if err := flag.CommandLine.Parse([]string{"-metric-file-name="}); err != nil {
		t.Fatal(err)
	}
	r := new(assembly)
	r.initSimulation()
	t.Cleanup(func() { _ = r.simulation.DataRecorder().Close() })
	files, err := filepath.Glob("akita_sim_*.sqlite3")
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one default recording, got %v: %v", files, err)
	}
}

// Akita converts service cleanup panics into Terminate errors. The runner must
// surface those errors instead of returning successfully after a failed close.
type cleanupMonitor struct {
	fail    bool
	stopped bool
}

func (*cleanupMonitor) Start(*sim.Simulation) {}
func (m *cleanupMonitor) Stop() {
	m.stopped = true
	if m.fail {
		panic("test cleanup failure")
	}
}

func TestRunReportsTerminationFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "cleanup failure"
		}
		t.Run(name, func(t *testing.T) {
			monitor := &cleanupMonitor{fail: fail}
			s := sim.MakeBuilder().WithTaskTracking().WithMonitor(monitor).Build()
			d := emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(0).Build()
			if err := s.Initialize(); err != nil {
				t.Fatal(err)
			}
			_ = d
			caught := finishSimulation(s, nil)

			if !monitor.stopped {
				t.Fatal("sample cleanup did not terminate simulation services")
			}
			if !fail {
				if caught != nil {
					t.Fatalf("successful cleanup panicked: %v", caught)
				}
				return
			}
			if caught == nil || !strings.Contains(caught.Error(), "test cleanup failure") {
				t.Fatalf("sample cleanup did not surface the cleanup error: %v", caught)
			}
		})
	}
}
