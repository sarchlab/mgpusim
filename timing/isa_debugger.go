package timing

import (
	"fmt"
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
)

// ISADebugger is a logger hook that can dump the wavefront status after each
// instruction execution
type ISADebugger struct {
	core.LogHookBase
}

// NewISADebugger creates a new ISADebugger.
func NewISADebugger(logger *log.Logger) *ISADebugger {
	d := new(ISADebugger)
	d.Logger = logger
	return d
}

// Type of WfHook claims the inst tracer is hooking to the emu.Wavefront type
func (d *ISADebugger) Type() reflect.Type {
	return reflect.TypeOf((*Wavefront)(nil))
}

// Pos of WfHook returns core.Any.
func (d *ISADebugger) Pos() core.HookPos {
	return core.Any
}

// The action that the ISADebugger takes
func (d *ISADebugger) Func(
	item interface{},
	domain core.Hookable,
	info interface{},
) {
	instInfo := info.(*InstHookInfo)

	if instInfo.Stage != "Completed" {
		return
	}

	cu := domain.(*ComputeUnit)
	wf := item.(*Wavefront)

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

	sRegFileStorage := cu.SRegFile.Storage()
	sRegOffset := wf.SRegOffset
	output += "\tSGPRs:\n"
	for i := 0; i < int(wf.CodeObject.WFSgprCount); i++ {
		regBytes, _ := sRegFileStorage.Read(uint64(sRegOffset+4*i), 4)
		regValue := insts.BytesToUint32(regBytes)
		output += fmt.Sprintf("\t\ts%d: 0x%08x\n", i, regValue)
	}

	simdID := wf.SIMDID
	vRegFileStorage := cu.VRegFile[simdID].Storage()
	vRegOffset := wf.VRegOffset
	output += "\tVGPRs: \n"
	for i := 0; i < int(wf.CodeObject.WIVgprCount); i++ {
		output += fmt.Sprintf("\t\t%d: ", i)
		for laneID := 0; laneID < 64; laneID++ {
			regBytes, _ := vRegFileStorage.Read(
				uint64(vRegOffset+laneID*1024+4*i), 4)
			regValue := insts.BytesToUint32(regBytes)
			output += fmt.Sprintf("0x%08x ", regValue)
		}
		output += fmt.Sprintf("\n")
	}

	d.Logger.Print(output)
}
