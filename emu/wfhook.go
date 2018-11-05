package emu

import (
	"fmt"
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
)

// WfHook is a hook that hooks to a emulator computeunit for each intruction
type WfHook struct {
	akita.LogHookBase
}

// NewWfHook returns a new WfHook that keeps instruction log in logger
func NewWfHook(logger *log.Logger) *WfHook {
	h := new(WfHook)
	h.Logger = logger
	return h
}

// Type of WfHook claims the inst tracer is hooking to the emu.Wavefront type
func (h *WfHook) Type() reflect.Type {
	return reflect.TypeOf((*Wavefront)(nil))
}

// Pos of WfHook returns akita.Any.
func (h *WfHook) Pos() akita.HookPos {
	return akita.AnyHookPos
}

// Func defines the behavior of the tracer when the tracer is invoked.
func (h *WfHook) Func(item interface{}, domain akita.Hookable, info interface{}) {
	wf := item.(*Wavefront)

	// For debugging
	//if wf.FirstWiFlatID != 0 {
	//	return
	//}

	output := fmt.Sprintf("\n\twg - (%d, %d, %d), wf - %d\n",
		wf.WG.IDX, wf.WG.IDY, wf.WG.IDZ, wf.FirstWiFlatID)
	output += fmt.Sprintf("\tInst: %s\n", wf.Inst().String(nil))
	output += fmt.Sprintf("\tPC: 0x%016x\n", wf.PC)
	output += fmt.Sprintf("\tEXEC: 0x%016x\n", wf.Exec)
	output += fmt.Sprintf("\tSCC: 0x%02x\n", wf.SCC)
	output += fmt.Sprintf("\tVCC: 0x%016x\n", wf.VCC)

	output += "\tSGPRs:\n"
	for i := 0; i < int(wf.CodeObject.WFSgprCount); i++ {
		regValue := insts.BytesToUint32(wf.ReadReg(insts.SReg(i), 1, 0))
		output += fmt.Sprintf("\t\ts%d: 0x%08x\n", i, regValue)
	}

	output += "\tVGPRs: \n"
	for i := 0; i < int(wf.CodeObject.WIVgprCount); i++ {
		output += fmt.Sprintf("\t\t%d: ", i)
		for laneID := 0; laneID < 64; laneID++ {
			regValue := insts.BytesToUint32(wf.ReadReg(insts.VReg(i), 1, laneID))
			output += fmt.Sprintf("0x%08x ", regValue)
		}
		output += fmt.Sprintf("\n")
	}

	h.Logger.Print(output)
}
