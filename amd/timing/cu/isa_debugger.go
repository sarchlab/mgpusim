package cu

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
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
	executingInst map[uint64]tracing.TaskStart
}

// NewISADebugger returns a new ISADebugger that keeps instruction log in
// logger
func NewISADebugger(logger *log.Logger, cu *ComputeUnit) *ISADebugger {
	h := new(ISADebugger)
	h.Logger = logger
	h.isFirstEntry = true
	h.cu = cu
	h.executingInst = make(map[uint64]tracing.TaskStart)

	h.Logger.Print("[")
	atexit.Register(func() { h.Logger.Print("\n]") })

	return h
}

// StartTask marks the start of an instruction.
func (h *ISADebugger) StartTask(task tracing.TaskStart) {
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

// AddTaskTag does nothing as of now.
func (h *ISADebugger) AddTaskTag(tag tracing.TaskTag) {
	// Do nothing.
}

// AddMilestone does nothing as of now.
func (h *ISADebugger) AddMilestone(milestone tracing.Milestone) {
	// Do nothing.
}

// EndTask marks the end of an instruction.
func (h *ISADebugger) EndTask(task tracing.TaskEnd) {
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

	output += "{"
	output += fmt.Sprintf(`"wg":[%d,%d,%d],"wf":%d,`,
		wf.WG.IDX, wf.WG.IDY, wf.WG.IDZ, wf.FirstWiFlatID)
	output += fmt.Sprintf(`"Inst":"%s",`, insts.NewInstPrinter(nil).Print(inst))
	output += fmt.Sprintf(`"PCLo":%d,`, wf.PC()&0xffffffff)
	output += fmt.Sprintf(`"PCHi":%d,`, wf.PC()>>32)
	output += fmt.Sprintf(`"EXECLo":%d,`, wf.EXEC()&0xffffffff)
	output += fmt.Sprintf(`"EXECHi":%d,`, wf.EXEC()>>32)
	output += fmt.Sprintf(`"VCCLo":%d,`, wf.VCC()&0xffffffff)
	output += fmt.Sprintf(`"VCCHi":%d,`, wf.VCC()>>32)
	output += fmt.Sprintf(`"SCC":%d,`, wf.SCC())

	output += `"SGPRs":[`
	for i := 0; i < int(wf.CodeObject.WFSgprCount); i++ {
		if i > 0 {
			output += ","
		}

		regValue := h.getSRegValue(wf, i)
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
	wf *wavefront.Wavefront,
	regIndex, laneID int,
) uint32 {
	registerFile := h.cu.VRegFile[wf.SIMDID]
	regRead := RegisterAccess{}
	regRead.Reg = insts.VReg(regIndex)
	regRead.RegCount = 1
	regRead.LaneID = laneID
	regRead.WaveOffset = wf.VRegOffset
	regRead.Data = make([]byte, 4)
	registerFile.Read(regRead)

	regValue := binary.LittleEndian.Uint32(regRead.Data)
	return regValue
}

func (h *ISADebugger) getSRegValue(
	wf *wavefront.Wavefront,
	regIndex int,
) uint32 {
	registerFile := h.cu.SRegFile
	regRead := RegisterAccess{}
	regRead.Reg = insts.SReg(regIndex)
	regRead.RegCount = 1
	regRead.WaveOffset = wf.SRegOffset
	regRead.Data = make([]byte, 4)
	registerFile.Read(regRead)

	regValue := binary.LittleEndian.Uint32(regRead.Data)
	return regValue
}
