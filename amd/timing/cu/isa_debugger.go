package cu

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/tebeka/atexit"
)

// ISADebugger is a tracer that logs the wavefront state after each
// instruction completes. Attach it to a timing compute-unit component with
// tracing.CollectTrace; obtain the ComputeUnit middleware with
// MiddlewareOf(comp).
type ISADebugger struct {
	Logger *log.Logger

	isFirstEntry  bool
	cu            *ComputeUnit
	executingInst map[uint64]tasks.Task
}

// NewISADebugger returns a new ISADebugger that keeps instruction log in
// logger
func NewISADebugger(logger *log.Logger, cu *ComputeUnit) *ISADebugger {
	h := new(ISADebugger)
	h.Logger = logger
	h.isFirstEntry = true
	h.cu = cu
	h.executingInst = make(map[uint64]tasks.Task)

	h.Logger.Print("[")
	atexit.Register(func() { h.Logger.Print("\n]") })

	return h
}

// StartTask marks the start of an instruction.
func (h *ISADebugger) TaskStarted(task tasks.Task) {
	if task.Kind != "inst" {
		return
	}

	// For debugging
	detail := task.Detail.(map[string]interface{})
	wf := h.cu.wavefrontByID(detail["wf"].(uint64))
	if wf.raw().FirstWiFlatID != 0 {
		return
	}

	h.executingInst[task.ID] = task
}

// AddTaskTag does nothing as of now.
func (h *ISADebugger) TagAdded(tag tasks.Tag) {
	// Do nothing.
}

// AddMilestone does nothing as of now.
func (h *ISADebugger) MilestoneAdded(milestone tasks.Milestone) {
	// Do nothing.
}

// EndTask marks the end of an instruction.
func (h *ISADebugger) TaskEnded(task tasks.Task) {
	originalTask, found := h.executingInst[task.ID]

	if !found {
		return
	}

	detail := originalTask.Detail.(map[string]interface{})
	wf := h.cu.wavefrontByID(detail["wf"].(uint64))
	inst := h.cu.instruction(detail["inst"].(uint64))

	if wf.group().raw().IDX == 75 && wf.group().raw().IDY == 1 {
		h.logWholeWf(inst, wf)
	}

	delete(h.executingInst, task.ID)
}

func (h *ISADebugger) logWholeWf(
	inst *insts.Inst,
	wf *Wavefront,
) {
	output := ""
	if h.isFirstEntry {
		h.isFirstEntry = false
	} else {
		output += ","
	}

	output += "{"
	output += fmt.Sprintf(`"wg":[%d,%d,%d],"wf":%d,`,
		wf.group().raw().IDX, wf.group().raw().IDY, wf.group().raw().IDZ, wf.raw().FirstWiFlatID)
	output += fmt.Sprintf(`"Inst":"%s",`, insts.NewInstPrinter(nil).Print(inst))
	output += fmt.Sprintf(`"PCLo":%d,`, wf.PC()&0xffffffff)
	output += fmt.Sprintf(`"PCHi":%d,`, wf.PC()>>32)
	output += fmt.Sprintf(`"EXECLo":%d,`, wf.EXEC()&0xffffffff)
	output += fmt.Sprintf(`"EXECHi":%d,`, wf.EXEC()>>32)
	output += fmt.Sprintf(`"VCCLo":%d,`, wf.VCC()&0xffffffff)
	output += fmt.Sprintf(`"VCCHi":%d,`, wf.VCC()>>32)
	output += fmt.Sprintf(`"SCC":%d,`, wf.SCC())

	output += `"SGPRs":[`
	co := &wf.codeObject().Meta
	for i := 0; i < int(co.WFSgprCount); i++ {
		if i > 0 {
			output += ","
		}

		regValue := h.getSRegValue(wf, i)
		output += fmt.Sprintf("%d", regValue)
	}
	output += "]"

	output += `,"VGPRs":[`
	for i := 0; i < int(co.WIVgprCount); i++ {
		if i > 0 {
			output += ","
		}
		output += "["

		for laneID := 0; laneID < 64; laneID++ {
			if laneID > 0 {
				output += ","
			}

			regValue := h.getVRegValue(wf, i, laneID)
			output += fmt.Sprintf("%d", regValue)
		}

		output += "]"
	}
	output += "]"

	output += `,"LDS":""`

	output += "}"

	h.Logger.Print(output)
}

func (h *ISADebugger) getVRegValue(
	wf *Wavefront,
	regIndex, laneID int,
) uint32 {
	registerFile := h.cu.VRegFile[wf.execution().SIMDID]
	regRead := RegisterAccess{}
	regRead.Reg = insts.VReg(regIndex)
	regRead.RegCount = 1
	regRead.LaneID = laneID
	regRead.WaveOffset = wf.execution().VRegOffset
	regRead.Data = make([]byte, 4)
	registerFile.Read(regRead)

	regValue := binary.LittleEndian.Uint32(regRead.Data)
	return regValue
}

func (h *ISADebugger) getSRegValue(
	wf *Wavefront,
	regIndex int,
) uint32 {
	registerFile := h.cu.SRegFile
	regRead := RegisterAccess{}
	regRead.Reg = insts.SReg(regIndex)
	regRead.RegCount = 1
	regRead.WaveOffset = wf.execution().SRegOffset
	regRead.Data = make([]byte, 4)
	registerFile.Read(regRead)

	regValue := binary.LittleEndian.Uint32(regRead.Data)
	return regValue
}
