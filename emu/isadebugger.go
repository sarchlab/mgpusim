package emu

import (
	"encoding/base64"
	"fmt"
	"log"

	"github.com/tebeka/atexit"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/insts"
)

// ISADebugger is a hook that hooks to a emulator computeunit for each intruction
type ISADebugger struct {
	akita.LogHookBase

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
func (h *ISADebugger) Func(ctx akita.HookCtx) {
	wf, ok := ctx.Item.(*Wavefront)
	if !ok {
		return
	}

	// For debugging
	// if wf.FirstWiFlatID != 0 {
	// 	return
	// }

	h.logWholeWf(wf)
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

	// output += fmt.Sprintf("\tPC: 0x%016x\n", wf.PC)
	// output += fmt.Sprintf("\tEXEC: 0x%016x\n", wf.Exec)
	// output += fmt.Sprintf("\tSCC: 0x%02x\n", wf.SCC)
	// output += fmt.Sprintf("\tVCC: 0x%016x\n", wf.VCC)

	// output += "\tSGPRs:\n"
	// for i := 0; i < int(wf.CodeObject.WFSgprCount); i++ {
	// 	regValue := insts.BytesToUint32(wf.ReadReg(insts.SReg(i), 1, 0))
	// 	output += fmt.Sprintf("\t\ts%d: 0x%08x\n", i, regValue)
	// }

	// output += "\tVGPRs: \n"
	// for i := 0; i < int(wf.CodeObject.WIVgprCount); i++ {
	// 	output += fmt.Sprintf("\t\tv%d: ", i)
	// 	for laneID := 0; laneID < 64; laneID++ {
	// 		regValue := insts.BytesToUint32(wf.ReadReg(insts.VReg(i), 1, laneID))
	// 		output += fmt.Sprintf("0x%08x ", regValue)
	// 	}
	// 	output += fmt.Sprintf("\n")
	// }

	// if wf.WG.Packet.GroupSegmentSize > 0 {
	// 	output += "\tLDS: \n"
	// 	for i := uint32(0); i < wf.WG.Packet.GroupSegmentSize; i += 32 {
	// 		output += "\t\t"

	// 		for j := 3; j >= 0; j-- {
	// 			startAddr := i + uint32(j*4)
	// 			endAddr := i + uint32((j+1)*4)

	// 			d := uint32(0)
	// 			if endAddr <= wf.WG.Packet.GroupSegmentSize {
	// 				d = binary.LittleEndian.Uint32(wf.LDS[startAddr:endAddr])
	// 			}
	// 			output += fmt.Sprintf("%08x ", d)
	// 		}

	// 		output += fmt.Sprintf("\t\t0x%08x\n", i)
	// 	}
	// }

	h.Logger.Print(output)
}
