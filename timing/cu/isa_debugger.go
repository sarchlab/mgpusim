package cu

import (
	"fmt"
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/timing/wavefront"
	"gitlab.com/akita/util/tracing"
)

// ISADebugger is a logger hook that can dump the wavefront status after each
// instruction execution
type ISADebugger struct {
	akita.LogHookBase
	inflightInst map[string]tracing.Task
}

// NewISADebugger creates a new ISADebugger.
func NewISADebugger(logger *log.Logger) *ISADebugger {
	d := new(ISADebugger)
	d.Logger = logger
	d.inflightInst = make(map[string]tracing.Task)
	return d
}

// Func defines the action that the ISADebugger takes
func (d *ISADebugger) Func(
	ctx akita.HookCtx,
) {
	task, ok := ctx.Item.(tracing.Task)
	if !ok {
		return
	}

	if ctx.Pos == tracing.HookPosTaskStart && task.Kind == "inst" {
		d.inflightInst[task.ID] = task
		return
	}

	if ctx.Pos == tracing.HookPosTaskStep {
		return
	}

	oringinalTask, ok := d.inflightInst[task.ID]
	if !ok {
		return
		// panic("inst is not inflight")
	}
	delete(d.inflightInst, task.ID)

	detail := oringinalTask.Detail.(map[string]interface{})
	cu := ctx.Domain.(*ComputeUnit)
	wf := detail["wf"].(*wavefront.Wavefront)
	inst := detail["inst"].(*wavefront.Inst)

	// For debugging
	if wf.FirstWiFlatID != 0 {
		return
	}

	output := fmt.Sprintf("\n\twg - (%d, %d, %d), wf - %d\n",
		wf.WG.IDX, wf.WG.IDY, wf.WG.IDZ, wf.FirstWiFlatID)
	output += fmt.Sprintf("\tInst: %s\n", inst.String(nil))
	output += fmt.Sprintf("\tPC: 0x%016x\n", wf.PC)
	output += fmt.Sprintf("\tEXEC: 0x%016x\n", wf.EXEC)
	output += fmt.Sprintf("\tSCC: 0x%02x\n", wf.SCC)
	output += fmt.Sprintf("\tVCC: 0x%016x\n", wf.VCC)

	output += d.dumpSRegs(wf, cu)
	output += d.dumpVRegs(wf, cu)

	d.Logger.Print(output)
}

func (d *ISADebugger) dumpSRegs(
	wf *wavefront.Wavefront,
	cu *ComputeUnit,
) string {
	data := make([]byte, 4)
	access := RegisterAccess{}
	access.Data = data
	access.RegCount = 1
	access.WaveOffset = wf.SRegOffset
	output := "\tSGPRs:\n"
	for i := 0; i < int(wf.CodeObject.WFSgprCount); i++ {
		access.Reg = insts.SReg(i)
		cu.SRegFile.Read(access)
		regValue := insts.BytesToUint32(data)
		output += fmt.Sprintf("\t\ts%d: 0x%08x\n", i, regValue)
	}
	return output
}

func (d *ISADebugger) dumpVRegs(
	wf *wavefront.Wavefront,
	cu *ComputeUnit,
) string {
	simdID := wf.SIMDID
	data := make([]byte, 4)
	access := RegisterAccess{}
	access.Data = data
	access.RegCount = 1
	access.WaveOffset = wf.VRegOffset
	output := "\tVGPRs: \n"
	for i := 0; i < int(wf.CodeObject.WIVgprCount); i++ {
		output += fmt.Sprintf("\t\tv%d: ", i)
		access.Reg = insts.VReg(i)
		for laneID := 0; laneID < 64; laneID++ {
			access.LaneID = laneID
			cu.VRegFile[simdID].Read(access)
			regValue := insts.BytesToUint32(data)
			output += fmt.Sprintf("0x%08x ", regValue)
		}
		output += fmt.Sprintf("\n")
	}
	return output
}
