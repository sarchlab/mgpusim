package emu

import (
	"encoding/base64"
	"fmt"
	"log"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v3/insts"
	"github.com/tebeka/atexit"
)

// ISADebugger is a hook that hooks to a emulator computeunit for each intruction
type ISADebugger struct {
	sim.LogHookBase

	isFirstEntry bool
	// prevWf *Wavefront
}

// NewISADebugger returns a new ISADebugger that keeps instruction log in logger
func NewISADebugger(logger *log.Logger) *ISADebugger {
	h := new(ISADebugger)
	h.Logger = logger
	h.isFirstEntry = true

	h.Logger.Print("[")
	atexit.Register(func() { h.Logger.Print("\n]") })

	return h
}

// Func defines the behavior of the tracer when the tracer is invoked.
func (h *ISADebugger) Func(ctx sim.HookCtx) {
	wf, ok := ctx.Item.(*Wavefront)
	if !ok {
		return
	}

	if wf.WG.IDX == 75 && wf.WG.IDY == 1 {
		h.logWholeWf(wf)
	}

	// For debugging
	// if wf.FirstWiFlatID != 0 {
	// 	return
	// }

	// if h.prevWf == nil || h.prevWf.FirstWiFlatID != wf.FirstWiFlatID {
	// 	h.logWholeWf(wf)
	// } else {
	// 	h.logDiffWf(wf)
	// }

	// h.stubWf(wf)
}

func (h *ISADebugger) logWholeWf(wf *Wavefront) {
	output := ""
	if h.isFirstEntry {
		h.isFirstEntry = false
	} else {
		output += ","
	}

	output += fmt.Sprintf("{")
	output += fmt.Sprintf(`"wg":[%d,%d,%d],"wf":%d,`,
		wf.WG.IDX, wf.WG.IDY, wf.WG.IDZ, wf.FirstWiFlatID)
	output += fmt.Sprintf(`"Inst":"%s",`, wf.Inst().String(nil))
	output += fmt.Sprintf(`"PCLo":%d,`, wf.PC&0xffffffff)
	output += fmt.Sprintf(`"PCHi":%d,`, wf.PC>>32)
	output += fmt.Sprintf(`"EXECLo":%d,`, wf.Exec&0xffffffff)
	output += fmt.Sprintf(`"EXECHi":%d,`, wf.Exec>>32)
	output += fmt.Sprintf(`"VCCLo":%d,`, wf.VCC&0xffffffff)
	output += fmt.Sprintf(`"VCCHi":%d,`, wf.VCC>>32)
	output += fmt.Sprintf(`"SCC":%d,`, wf.SCC)

	output += fmt.Sprintf(`"SGPRs":[`)
	for i := 0; i < int(wf.CodeObject.WFSgprCount); i++ {
		if i > 0 {
			output += ","
		}
		regValue := insts.BytesToUint32(wf.ReadReg(insts.SReg(i), 1, 0))
		output += fmt.Sprintf("%d", regValue)
	}
	output += "]"

	output += `,"VGPRs":[`
	for i := 0; i < int(wf.CodeObject.WIVgprCount); i++ {
		if i > 0 {
			output += ","
		}
		output += "["

		for laneID := 0; laneID < 64; laneID++ {
			if laneID > 0 {
				output += ","
			}

			regValue := insts.BytesToUint32(
				wf.ReadReg(insts.VReg(i), 1, laneID))
			output += fmt.Sprintf("%d", regValue)
		}

		output += "]"
	}
	output += "]"

	output += `,"LDS":`
	output += fmt.Sprintf(`"%s"`, base64.StdEncoding.EncodeToString(wf.LDS))

	output += fmt.Sprintf("}")

	h.Logger.Print(output)
}
