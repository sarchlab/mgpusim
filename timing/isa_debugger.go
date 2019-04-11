package timing

import (
	"fmt"
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/timing/wavefront"
)

// ISADebugger is a logger hook that can dump the wavefront status after each
// instruction execution
type ISADebugger struct {
	akita.LogHookBase
}

// NewISADebugger creates a new ISADebugger.
func NewISADebugger(logger *log.Logger) *ISADebugger {
	d := new(ISADebugger)
	d.Logger = logger
	return d
}

// Func defines the action that the ISADebugger takes
func (d *ISADebugger) Func(
	item interface{},
	domain akita.Hookable,
	info interface{},
) {
	instInfo := info.(*wavefront.InstHookInfo)

	if instInfo.Stage != "Completed" {
		return
	}

	cu := domain.(*ComputeUnit)
	wf := item.(*wavefront.Wavefront)

	// For debugging
	if wf.FirstWiFlatID != 0 {
		return
	}

	output := fmt.Sprintf("\n\twg - (%d, %d, %d), wf - %d\n",
		wf.WG.IDX, wf.WG.IDY, wf.WG.IDZ, wf.FirstWiFlatID)
	output += fmt.Sprintf("\tInst: %s\n", instInfo.Inst.String(nil))
	output += fmt.Sprintf("\tPC: 0x%016x\n", wf.PC)
	output += fmt.Sprintf("\tEXEC: 0x%016x\n", wf.EXEC)
	output += fmt.Sprintf("\tSCC: 0x%02x\n", wf.SCC)
	output += fmt.Sprintf("\tVCC: 0x%016x\n", wf.VCC)

	//sRegFileStorage := cu.SRegFile.Storage()
	data := make([]byte, 4)
	access := RegisterAccess{}
	access.Data = data
	access.RegCount = 1
	access.WaveOffset = wf.SRegOffset
	output += "\tSGPRs:\n"
	for i := 0; i < int(wf.CodeObject.WFSgprCount); i++ {
		access.Reg = insts.SReg(i)
		cu.SRegFile.Read(access)
		regValue := insts.BytesToUint32(data)
		output += fmt.Sprintf("\t\ts%d: 0x%08x\n", i, regValue)
	}

	simdID := wf.SIMDID
	access.WaveOffset = wf.VRegOffset
	output += "\tVGPRs: \n"
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

	d.Logger.Print(output)
}
