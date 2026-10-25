package cu

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

func TestCheckpointPreservesBarrierAndLDSIdentity(t *testing.T) {
	original := checkpointCU(t)
	grid := kernels.NewGridBuilder(original.comp)
	grid.SetKernel(kernels.KernelLaunchInfo{
		CodeObject: &insts.KernelCodeObject{KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{}},
		Packet: &kernels.HsaKernelDispatchPacket{GridSizeX: 128, GridSizeY: 1, GridSizeZ: 1,
			WorkgroupSizeX: 128, WorkgroupSizeY: 1, WorkgroupSizeZ: 1},
	})
	raw := grid.NextWG()
	wg := wavefront.NewWorkGroup(raw, messaging.Msg{ID: original.comp.NewID(), Payload: protocol.MapWGReq{}})
	wg.LDS = []byte{11, 13, 17, 19}
	for _, base := range raw.Wavefronts {
		wf := wavefront.NewWavefront(base)
		wf.WG = wg
		wf.State = wavefront.WfAtBarrier
		wg.Wfs = append(wg.Wfs, wf)
		original.WfPools[0].AddWf(wf)
	}
	original.workGroups[wg.UID] = wg
	scheduler := original.Scheduler.(*SchedulerImpl)
	scheduler.barrierBuffer = wg.Wfs[:1]
	scheduler.cyclesNoProgress = 27
	scheduler.isPaused = true
	original.LDSUnit.(*LDSUnit).toExec = wg.Wfs[1]
	original.LDSUnit.(*LDSUnit).cycleLeft = 3
	var data bytes.Buffer
	if err := (&cuCheckpoint{original}).SaveCheckpoint(&data); err != nil {
		t.Fatal(err)
	}
	restored := checkpointCU(t)
	if err := (&cuCheckpoint{restored}).LoadCheckpoint(&data); err != nil {
		t.Fatal(err)
	}
	restoredWG := restored.workGroups[wg.UID]
	if restoredWG == wg || !bytes.Equal(restoredWG.LDS, wg.LDS) {
		t.Fatal("LDS not restored independently")
	}
	rs := restored.Scheduler.(*SchedulerImpl)
	if rs.barrierBuffer[0] != restoredWG.Wfs[0] || restored.WfPools[0].wfs[0] != restoredWG.Wfs[0] {
		t.Fatal("barrier and pool do not share the restored wavefront")
	}
	lds := restored.LDSUnit.(*LDSUnit)
	if lds.toExec != restoredWG.Wfs[1] || lds.cycleLeft != 3 || rs.cyclesNoProgress != 27 || !rs.isPaused {
		t.Fatal("pipeline progress was not restored")
	}
	restoredWG.LDS[0] = 23
	if wg.LDS[0] != 11 {
		t.Fatal("restored LDS still aliases source")
	}
}

func checkpointCU(t *testing.T) *ComputeUnit {
	t.Helper()
	simulation := sim.MakeBuilder().Build()
	t.Cleanup(func() {
		if err := simulation.Terminate(); err != nil {
			t.Error(err)
		}
	})
	return MakeBuilder().WithSimulation(simulation).Build("CU").Resources.Runtime
}
