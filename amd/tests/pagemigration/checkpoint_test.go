package pagemigration

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

const (
	restorePathEnv = "MGPUSIM_PAGE_MIGRATION_RESTORE"
	restoreGPUEnv  = "MGPUSIM_PAGE_MIGRATION_GPU"

	checkpointFloats = 1024
)

// pendingRead is the host side of the checkpointed workload: the readback
// that is queued behind the kernel, and the data it must return. It is saved
// with the simulation, so a fresh process finds it in the checkpoint.
type pendingRead struct {
	Buffer gpuruntime.HostBuffer
	Want   []float32
}

func (r *pendingRead) Name() string { return "PendingRead" }

func (r *pendingRead) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, r, nil)
}

func (r *pendingRead) LoadCheckpoint(reader io.Reader) error {
	return modeling.ReadCheckpoint(reader, r, nil)
}

// A checkpoint that is taken while a page is being copied restores exactly:
// in the same simulation, and in a fresh process that loads it, the run ends
// with the same data and the same final state as the run that was never
// interrupted.
func TestCheckpointInTheMiddleOfAMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("page-migration acceptance builds and runs full simulations; run without -short")
	}

	if path := os.Getenv(restorePathEnv); path != "" {
		pending := &pendingRead{}
		s := newSystem(t, os.Getenv(restoreGPUEnv), resource(pending))

		if err := s.sim.LoadCheckpoint(path); err != nil {
			t.Fatal(err)
		}

		finishCheckpointed(t, s, pending, path+".restored")

		return
	}

	for _, gpuType := range platforms {
		t.Run(gpuType, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mid-migration.tar.gz")

			// The simulation of the parent ends with this subtest, before
			// the child builds its own.
			if !t.Run("uninterrupted and rewound", func(t *testing.T) {
				checkpointMidMigration(t, gpuType, path)
			}) {
				return
			}

			debug.FreeOSMemory()

			cmd := exec.CommandContext(t.Context(), os.Args[0],
				"-test.run=^TestCheckpointInTheMiddleOfAMigration$", "-test.v")
			cmd.Env = append(os.Environ(), restorePathEnv+"="+path, restoreGPUEnv+"="+gpuType)

			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("restore in a fresh process: %v\n%s", err, out)
			}

			mustBeSameFile(t, path+".uninterrupted", path+".restored")
		})
	}
}

// checkpointMidMigration saves a checkpoint while the page of the workload
// is in the data mover, runs to the end, and rewinds the same simulation to
// the checkpoint.
func checkpointMidMigration(t *testing.T, gpuType, path string) {
	t.Helper()

	pending := &pendingRead{Want: ramp(1, checkpointFloats)}
	s := newSystem(t, gpuType, resource(pending))

	// GPU 1 writes the page with a kernel, so its lines are dirty in the L2;
	// GPU 2 then reads it, and the host reads what GPU 2 got.
	staging := s.local(1, checkpointFloats*floatBytes)
	shared := s.unified(checkpointFloats * floatBytes)
	result := s.local(2, checkpointFloats*floatBytes)

	s.upload(staging, pending.Want)
	s.touch(1, shared, staging, checkpointFloats)
	s.run()

	s.touch(2, result, shared, checkpointFloats)
	pending.Buffer = benchmarks.Download(s.rt, s.queues[1], result, checkpointFloats*floatBytes)
	s.start()

	for len(s.inbound[1].State.Transactions) == 0 {
		if s.done() {
			t.Fatal("the page was never in the data mover")
		}

		if err := s.engine.RunUntil(s.engine.CurrentTime() + 1000); err != nil {
			t.Fatal(err)
		}
	}

	m := s.driver.State.PageMigration
	if len(m.Pending) == 0 || s.page(shared).DeviceID != 1 {
		t.Fatalf("the checkpoint is not in the middle of a migration: %+v", m.Stats)
	}

	if err := s.sim.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}

	t.Logf("%s: checkpoint at %d ps with the page in the data mover", gpuType, s.engine.CurrentTime())
	finishCheckpointed(t, s, pending, path+".uninterrupted")

	if err := s.sim.LoadCheckpoint(path); err != nil {
		t.Fatal(err)
	}

	finishCheckpointed(t, s, pending, path+".rewound")
	mustBeSameFile(t, path+".uninterrupted", path+".rewound")
}

// finishCheckpointed runs a simulation to its end, checks what the host read
// and that the page migrated, and saves the final state.
func finishCheckpointed(t *testing.T, s *system, pending *pendingRead, path string) {
	t.Helper()

	if err := s.engine.Run(); err != nil {
		t.Fatal(err)
	}

	s.mustBeDone()

	got := make([]float32, checkpointFloats)
	if err := benchmarks.Read(s.rt, pending.Buffer, got); err != nil {
		t.Fatal(err)
	}

	mustEqualFloats(t, "what GPU 2 read", got, pending.Want)

	if st := s.stats(); st.Migrations != 1 || st.PagesMigrated != 1 {
		t.Fatalf("stats %+v", st)
	}

	if err := s.sim.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
}

func mustBeSameFile(t *testing.T, want, got string) {
	t.Helper()

	a, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(a, b) {
		t.Fatalf("%s differs from %s", filepath.Base(got), filepath.Base(want))
	}
}
