package cp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

func checkpointCP(t *testing.T, alg string, change func(*Spec)) (*sim.Simulation, *Comp) {
	t.Helper()
	s := sim.MakeBuilder().WithTaskTracking().Build()
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	spec := DefaultSpec()
	spec.Alg = alg
	spec.NumDispatchers = 2
	spec.NumDies = 2
	spec.ConstantKernelOverhead = 3
	spec.WavefrontDispatchCycles = 20
	spec.Driver = "Driver.ToGPU"
	spec.DMAEngine = "DMA.ToCP"
	spec.RDMA = "RDMA.Ctrl"
	for i := 0; i < 4; i++ {
		spec.CUs = append(spec.CUs, CUConfig{Port: messaging.RemotePort(fmt.Sprintf("CU%d.Top", i)),
			WavefrontPools: []int{1},
			VectorCounts:   []int{256},
			ScalarCount:    256,
			LDSCapacity:    1024})
	}
	if change != nil {
		change(&spec)
	}
	c := MakeBuilder().WithSimulation(s).WithSpec(spec).Build("CP")
	for _, name := range []string{"ToDriver", "ToCUs", "ToDMA", "ToCaches", "ToRDMA"} {
		p := twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(twowaybuffered.Spec{IncomingCapacity: 64,
			OutgoingCapacity: 1}).Build()
		c.BindPort(name, p)
		(&noopConn{}).BindPort(p)
	}
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	return s, c
}

func launchCheckpointKernels(c *Comp) {
	for i := 0; i < 2; i++ {
		c.Ports.ToDriver.Deliver(messaging.Msg{ID: c.NewID(),
			Src: c.Spec.Driver,
			Dst: c.Ports.ToDriver.AsRemote(),
			Payload: protocol.LaunchKernelReq{
				CodeObject: insts.CodeObjectSnapshot(&insts.KernelCodeObject{
					KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{WFSgprCount: 16,
						WIVgprCount:          4,
						GroupSegmentByteSize: 256}}),
				Packet: kernels.HsaKernelDispatchPacket{GridSizeX: 1024,
					GridSizeY:      1,
					GridSizeZ:      1,
					WorkgroupSizeX: 64,
					WorkgroupSizeY: 1,
					WorkgroupSizeZ: 1},
			}})
	}
}

type dispatchObservation struct {
	At      timing.VTimeInPicoSec `json:"at"`
	Message messaging.Msg         `json:"message"`
}

func finishCheckpointCP(t *testing.T, s *sim.Simulation, c *Comp, path string) {
	t.Helper()
	var observations []dispatchObservation
	completed := 0
	for cycle := 11; cycle < 500; cycle++ {
		if req, ok := c.Ports.ToCUs.RetrieveOutgoing(); ok {
			observations = append(observations, dispatchObservation{c.CurrentTime(), req})
			c.Ports.ToCUs.Deliver(messaging.Msg{ID: c.NewID(),
				Src:     req.Dst,
				Dst:     req.Src,
				Payload: protocol.WGCompletionMsg{RspToIDs: []uint64{req.ID}}})
		}
		if rsp, ok := c.Ports.ToDriver.RetrieveOutgoing(); ok {
			completed++
			observations = append(observations, dispatchObservation{c.CurrentTime(), rsp})
		}
		if err := s.Engine().(*timing.SerialEngine).RunUntil(timing.VTimeInPicoSec(cycle) * 1000); err != nil {
			t.Fatal(err)
		}
		if completed == 2 {
			break
		}
	}
	if completed != 2 {
		t.Fatalf("only %d kernels completed", completed)
	}
	for _, cu := range c.State.CUResources {
		if len(cu.Reserved) != 0 || cu.FreeWavefronts[0] != 1 {
			t.Fatal("shared reservations not released")
		}
	}
	if err := s.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(observations)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".json", data, 0600); err != nil {
		t.Fatal(err)
	}
}

// Both dispatchers are active while the one-entry outgoing port holds an
// undispatched response. A new process must preserve the shared pool, held WG,
// grid cursors, per-die rate gates, task IDs, request IDs and exact timing.
//
//nolint:gocognit,funlen // Keep subprocess setup and exact continuation comparisons together.
func TestCPFreshProcessCheckpoint(t *testing.T) {
	if path := os.Getenv("MGPUSIM_CP_RESTORE"); path != "" {
		s, c := checkpointCP(t, os.Getenv("MGPUSIM_CP_ALG"), nil)
		if err := s.LoadCheckpoint(path); err != nil {
			t.Fatal(err)
		}
		finishCheckpointCP(t, s, c, path+".restored")
		return
	}
	for _, alg := range []string{"round-robin", "greedy", "partition", "per-die"} {
		t.Run(alg, func(t *testing.T) {
			s, c := checkpointCP(t, alg, nil)
			launchCheckpointKernels(c)
			if err := s.Engine().(*timing.SerialEngine).RunUntil(10000); err != nil {
				t.Fatal(err)
			}
			for _, d := range c.State.Dispatchers {
				if !d.IsDispatching {
					t.Fatal("checkpoint is not inside two concurrent kernels")
				}
			}
			if alg == "per-die" {
				gated := false
				for _, d := range c.State.Dispatchers {
					for _, left := range d.DieCyclesLeft {
						gated = gated || left > 0
					}
				}
				if !gated {
					t.Fatal("checkpoint has no active per-die rate gate")
				}
			}
			reserved := 0
			for _, cu := range c.State.CUResources {
				reserved += len(cu.Reserved)
			}
			if reserved == 0 {
				t.Fatal("checkpoint has no active reservations")
			}
			path := filepath.Join(t.TempDir(), "active.tar.gz")
			if err := s.SaveCheckpoint(path); err != nil {
				t.Fatal(err)
			}
			finishCheckpointCP(t, s, c, path+".uninterrupted")
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCPFreshProcessCheckpoint$", "-test.v")
			cmd.Env = append(os.Environ(), "MGPUSIM_CP_RESTORE="+path, "MGPUSIM_CP_ALG="+alg)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("fresh process: %v\n%s", err, output)
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
					t.Fatalf("restored state/order/timing differ: %s", suffix)
				}
			}
		})
	}
}

func TestCPCheckpointRejectsChangedWiring(t *testing.T) {
	_, c := checkpointCP(t, "round-robin", nil)
	launchCheckpointKernels(c)
	var checkpoint bytes.Buffer
	if err := c.SaveCheckpoint(&checkpoint); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Spec){
		func(s *Spec) { s.Driver = "Other.ToGPU" },
		func(s *Spec) { s.DMAEngine = "OtherDMA.ToCP" },
		func(s *Spec) { s.CUs[0].Port = "OtherCU.Top" },
		func(s *Spec) { s.CUs[0], s.CUs[1] = s.CUs[1], s.CUs[0] },
		func(s *Spec) { s.CUs[0].VectorCounts[0] = 512 },
		func(s *Spec) { s.L1VCaches = []messaging.RemotePort{"L1.Control"} },
	} {
		_, other := checkpointCP(t, "round-robin", change)
		if err := other.LoadCheckpoint(bytes.NewReader(checkpoint.Bytes())); err == nil {
			t.Fatal("accepted incompatible wiring/capacity")
		}
	}
}
