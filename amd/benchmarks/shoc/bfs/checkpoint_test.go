package bfs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"testing"

	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp"
)

type recoveryResult struct {
	Costs  [][]uint32            `json:"costs"`
	Depths []int                 `json:"depths"`
	Time   timing.VTimeInPicoSec `json:"time"`
	Marker timing.VTimeInPicoSec `json:"marker"`
}

func newRecoverySystem(
	t *testing.T,
	mode string,
) (*simulation.Simulation, *driver.Runtime, *runner.Runner, []*Benchmark) {
	t.Helper()
	s := simulation.MakeBuilder().WithTaskTracking().Build()
	var rt *driver.Runtime
	if mode == "d2h" {
		rt = timingconfig.MakeBuilder().WithSimulation(s).WithNumGPUs(1).Build()
	} else {
		rt = emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(1).Build()
	}
	r := runner.New(s, rt)
	var bs []*Benchmark
	for i := 0; i < 2; i++ {
		b := NewBenchmark(rt)
		b.NumNode = 32 + i*16
		b.Degree = 3
		b.MaxDepth = 10
		b.SelectGPU([]int{1})
		b.SetVerification(true)
		r.AddBenchmark(b)
		bs = append(bs, b)
	}
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	return s, rt, r, bs
}
func writeRecoveryResult(
	t *testing.T,
	s *simulation.Simulation,
	rt *driver.Runtime,
	r *runner.Runner,
	bs []*Benchmark,
	event driver.Event,
	path string,
) {
	t.Helper()
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	result := recoveryResult{Time: s.Engine().CurrentTime()}
	for _, b := range bs {
		if !b.IsEnded() || b.Depth < 2 {
			t.Fatalf("BFS continuation did not finish multiple frontiers: phase=%d depth=%d", b.Phase, b.Depth)
		}
		b.cpuBFS()
		for i, v := range b.HCost {
			if v != b.CpuCost[i] {
				t.Fatalf("BFS output[%d]=%d, want %d", i, v, b.CpuCost[i])
			}
		}
		result.Costs = append(result.Costs, b.HCost)
		result.Depths = append(result.Depths, b.Depth)
	}
	var err error
	result.Marker, err = rt.EventTime(event)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".json", data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	t.Logf("two BFS resources verified at %d ps, depths %v, marker %d ps", result.Time, result.Depths, result.Marker)
}

// Each child reconstructs the same named resources but does not execute setup
// or resubmit a benchmark phase before LoadCheckpoint and Runner.Run.
//
//nolint:gocognit,gocyclo,funlen // Keep each recovery/protocol scenario with its complete assertions.
func TestBFSFreshProcessRecovery(t *testing.T) {
	if path := os.Getenv("MGPUSIM_BFS_RESTORE"); path != "" {
		mode := os.Getenv("MGPUSIM_BFS_BOUNDARY")
		id, err := strconv.ParseUint(os.Getenv("MGPUSIM_BFS_MARKER"), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		s, rt, r, bs := newRecoverySystem(t, mode)
		if err := s.LoadCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		writeRecoveryResult(t, s, rt, r, bs, driver.Event(id), path+".restored")
		return
	}
	for _, mode := range []string{"submitted", "kernel", "d2h", "drained"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "boundary.tar.gz")
			var marker driver.Event
			if !t.Run("uninterrupted", func(t *testing.T) {
				s, rt, r, bs := newRecoverySystem(t, mode)
				active, err := r.Step()
				if err != nil || !active {
					t.Fatalf("initial round: %v %v", active, err)
				}
				marker = rt.EnqueueRecordEvent(bs[0].Queue)
				if mode == "drained" {
					if err := s.Run(); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "kernel" || mode == "d2h" {
					engine := s.Engine().(*timing.SerialEngine)
					found := false
					for cycle := 1; cycle <= 100000; cycle++ {
						if err := engine.RunUntil(timing.VTimeInPicoSec(cycle) * 1000); err != nil {
							t.Fatal(err)
						}
						for _, component := range s.Components() {
							if cu, ok := component.(*emu.Comp); ok && mode == "kernel" && len(cu.State.QueuedWGs) > 0 {
								if err := engine.RunUntil(cu.State.NextEmulationAt); err != nil {
									t.Fatal(err)
								}
								found = len(cu.State.PendingCompletions) > 0
							}
							if dma, ok := component.(*cp.DMAComp); ok && mode == "d2h" {
								for _, copy := range dma.State.ProcessingReqs {
									if _, ok := copy.SuperiorRequest.Payload.(protocol.MemCopyD2HReq); ok && copy.SubordinateCount > 0 {
										found = true
									}
								}
							}
						}
						if found {
							break
						}
					}
					if !found {
						t.Fatal("did not reach requested in-flight boundary")
					}
				}
				if err := r.SaveCheckpoint(path); err != nil {
					t.Fatal(err)
				}
				writeRecoveryResult(t, s, rt, r, bs, marker, path+".uninterrupted")
			}) {
				return
			}
			debug.FreeOSMemory()
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestBFSFreshProcessRecovery$", "-test.v")
			cmd.Env = append(
				os.Environ(),
				"MGPUSIM_BFS_RESTORE="+path,
				"MGPUSIM_BFS_BOUNDARY="+mode,
				fmt.Sprintf("MGPUSIM_BFS_MARKER=%d", marker),
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("fresh restore: %v\n%s", err, output)
			}
			for _, suffix := range []string{"", ".json"} {
				want, err := os.ReadFile(path + ".uninterrupted" + suffix)
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(path + ".restored" + suffix)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(want, got) {
					t.Fatalf("restored continuation differs (%s)", suffix)
				}
			}
			t.Logf("%s: complete final state and outputs match after fresh-process restore", mode)
		})
	}
}
