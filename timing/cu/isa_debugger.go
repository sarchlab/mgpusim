package cu

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/tebeka/atexit"
	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mgpusim/v2/insts"
	"gitlab.com/akita/mgpusim/v2/timing/wavefront"
	"gitlab.com/akita/util/v2/tracing"
)

// ISADebugger is a hook that hooks to a emulator computeunit for each intruction
type ISADebugger struct {
	sim.LogHookBase

	isFirstEntry  bool
	cu            *ComputeUnit
	executingInst map[string]tracing.Task
	// prevWf *Wavefront
}

// NewISADebugger returns a new ISADebugger that keeps instruction log in logger
func NewISADebugger(logger *log.Logger, cu *ComputeUnit) *ISADebugger {
	h := new(ISADebugger)
	h.Logger = logger
	h.isFirstEntry = true
	h.cu = cu
	h.executingInst = make(map[string]tracing.Task)

	h.Logger.Print("[")
	atexit.Register(func() { h.Logger.Print("\n]") })

	return h
}

// StartTask marks the start of an instruction.
func (h *ISADebugger) StartTask(task tracing.Task) {
	if task.Kind != "inst" {
		return
	}

	// For debugging
	detail := task.Detail.(map[string]interface{})
	wf := detail["wf"].(*wavefront.Wavefront)
	if wf.FirstWiFlatID != 0 {
		return
	}

	h.executingInst[task.ID] = task
}

// StepTask does nothing as of now.
func (h *ISADebugger) StepTask(task tracing.Task) {
	// Do nothing.
}

// EndTask marks the end of an instruction.
func (h *ISADebugger) EndTask(task tracing.Task) {
	originalTask, found := h.executingInst[task.ID]

	if !found {
		return
	}

	detail := originalTask.Detail.(map[string]interface{})
	wf := detail["wf"].(*wavefront.Wavefront)
	inst := detail["inst"].(*wavefront.Inst).Inst

	if wf.WG.IDX == 75 && wf.WG.IDY == 1 {
		h.logWholeWf(inst, wf)
	}

	delete(h.executingInst, task.ID)
}

func (h *ISADebugger) logWholeWf(
	inst *insts.Inst,
	wf *wavefront.Wavefront,
) {
	output := ""
	if h.isFirstEntry {
		h.isFirstEntry = false
	} else {
		output += ","
	}

	output += fmt.Sprintf("{")
	output += fmt.Sprintf(`"wg":[%d,%d,%d],"wf":%d,`,
		wf.WG.IDX, wf.WG.IDY, wf.WG.IDZ, wf.FirstWiFlatID)
	output += fmt.Sprintf(`"Inst":"%s",`, inst.String(nil))
	output += fmt.Sprintf(`"PCLo":%d,`, wf.PC&0xffffffff)
	output += fmt.Sprintf(`"PCHi":%d,`, wf.PC>>32)
	output += fmt.Sprintf(`"EXECLo":%d,`, wf.EXEC&0xffffffff)
	output += fmt.Sprintf(`"EXECHi":%d,`, wf.EXEC>>32)
	output += fmt.Sprintf(`"VCCLo":%d,`, wf.VCC&0xffffffff)
	output += fmt.Sprintf(`"VCCHi":%d,`, wf.VCC>>32)
	output += fmt.Sprintf(`"SCC":%d,`, wf.SCC)

	output += fmt.Sprintf(`"SGPRs":[`)
	for i := 0; i < int(wf.CodeObject.WFSgprCount); i++ {
		if i > 0 {
			output += ","
		}

		registerFile := h.cu.SRegFile
		regRead := RegisterAccess{}
		regRead.Reg = insts.SReg(i)
		regRead.RegCount = 1
		regRead.WaveOffset = wf.SRegOffset
		regRead.Data = make([]byte, 4)
		registerFile.Read(regRead)

		regValue := binary.LittleEndian.Uint32(regRead.Data)
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

			registerFile := h.cu.VRegFile[wf.SIMDID]
			regRead := RegisterAccess{}
			regRead.Reg = insts.VReg(i)
			regRead.RegCount = 1
			regRead.LaneID = laneID
			regRead.WaveOffset = wf.VRegOffset
			regRead.Data = make([]byte, 4)
			registerFile.Read(regRead)

			regValue := binary.LittleEndian.Uint32(regRead.Data)
			output += fmt.Sprintf("%d", regValue)
		}

		output += "]"
	}
	output += "]"

	output += `,"LDS":""`

	// output += fmt.Sprintf(`"%s"`, base64.StdEncoding.EncodeToString())

	output += fmt.Sprintf("}")

	h.Logger.Print(output)
}
