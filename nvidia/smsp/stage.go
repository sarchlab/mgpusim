package smsp

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

// ========================================
// Stage + Unit Metadata for H100 Simulation
// ========================================

// Stage names used in all instruction pipelines
//   "Decode", "Issue", "Execute", "MemoryPipe", "BranchResolve", "Writeback"

// Execution unit types used for resource conflict checking
type ExecUnitKind int

const (
	UnitNone ExecUnitKind = iota
	UnitInt
	UnitFP32
	UnitFP64
	UnitTensor
	UnitLdSt
	UnitSpecial
)

// String returns a human-readable name for ExecUnitKind.
func (u ExecUnitKind) String() string {
	switch u {
	case UnitNone:
		return "UnitNone"
	case UnitInt:
		return "UnitInt"
	case UnitFP32:
		return "UnitFP32"
	case UnitFP64:
		return "UnitFP64"
	case UnitTensor:
		return "UnitTensor"
	case UnitLdSt:
		return "UnitLdSt"
	case UnitSpecial:
		return "UnitSpecial"
	default:
		return fmt.Sprintf("ExecUnitKind(%d)", int(u))
	}
}

// =====================
// Stage & Pipeline Types
// =====================

type StageDef struct {
	Name   string
	Cycles int
	Unit   ExecUnitKind
}

type InstructionPipelineTemplate struct {
	Opcode string
	Stages []StageDef
}

// ======================
// Helper Stage Builders
// ======================

func s(name string, cycles int, unit ExecUnitKind) StageDef {
	return StageDef{Name: name, Cycles: cycles, Unit: unit}
}

func stDecode() StageDef { return s("Decode", 1, UnitNone) }
func stIssue() StageDef  { return s("Issue", 1, UnitNone) }
func stWB() StageDef     { return s("Writeback", 1, UnitNone) }

// =======================
// Default Fallback Entry
// =======================
func defaultStages(op string) InstructionPipelineTemplate {
	log.WithField("opcode", op).Warn("PipelineTable missing entry for: Unknown opcode")
	return InstructionPipelineTemplate{
		Opcode: op,
		Stages: []StageDef{
			stDecode(),
			stIssue(),
			s("Execute", 4, UnitNone),
			stWB(),
		},
	}
}

// =======================
// Pipeline Table (H100 PCIe model)
// =======================

var PipelineTable = map[string]InstructionPipelineTemplate{
	// --- Control Flow ---
	"BRA":  {Opcode: "BRA", Stages: []StageDef{stDecode(), stIssue(), s("BranchResolve", 2, UnitNone), stWB()}},
	"EXIT": {Opcode: "EXIT", Stages: []StageDef{stDecode(), stIssue(), s("BranchResolve", 1, UnitNone), stWB()}},

	// --- Type Conversion ---
	"F2I.FTZ.U32.TRUNC.NTZ": {Opcode: "F2I...", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32), stWB()}},
	"I2F.U32.RP":            {Opcode: "I2F...", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt), stWB()}},

	// --- FP32 ---
	"FADD": {Opcode: "FADD", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32), stWB()}},
	"FFMA": {Opcode: "FFMA", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32), stWB()}},
	"FMUL": {Opcode: "FMUL", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32), stWB()}},

	// --- Tensor / Half ---
	"HFMA2.MMA": {Opcode: "HFMA2.MMA", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 8, UnitTensor), stWB()}},

	// --- INT ALU ---
	"IADD3":   {Opcode: "IADD3", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},
	"IADD3.X": {Opcode: "IADD3.X", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},
	"UIADD3":  {Opcode: "UIADD3", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},
	"VIADD":   {Opcode: "VIADD", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},

	"IMAD":          {Opcode: "IMAD", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},
	"IMAD.HI.U32":   {Opcode: "IMAD.HI.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},
	"IMAD.IADD":     {Opcode: "IMAD.IADD", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},
	"IMAD.MOV":      {Opcode: "IMAD.MOV", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},
	"IMAD.MOV.U32":  {Opcode: "IMAD.MOV.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},
	"IMAD.U32":      {Opcode: "IMAD.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},
	"IMAD.WIDE":     {Opcode: "IMAD.WIDE", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt), stWB()}},
	"IMAD.WIDE.U32": {Opcode: "IMAD.WIDE.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt), stWB()}},
	"UIMAD.WIDE":    {Opcode: "UIMAD.WIDE", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt), stWB()}},
	"LEA":           {Opcode: "LEA", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},
	"LEA.HI.X":      {Opcode: "LEA.HI.X", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},
	"LEA.HI.X.SX32": {Opcode: "LEA.HI.X.SX32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},

	// --- Predicate / Compare ---
	"ISETP.GE.AND":        {Opcode: "ISETP.GE.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.GE.U32.AND":    {Opcode: "ISETP.GE.U32.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.GT.AND":        {Opcode: "ISETP.GT.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.GT.U32.AND.EX": {Opcode: "ISETP.GT.U32.AND.EX", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.LT.U32.AND":    {Opcode: "ISETP.LT.U32.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.NE.AND":        {Opcode: "ISETP.NE.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.NE.OR":         {Opcode: "ISETP.NE.OR", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.NE.U32.AND":    {Opcode: "ISETP.NE.U32.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},

	// --- Load / Store ---
	"LDC":     {Opcode: "LDC", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},
	"ULDC":    {Opcode: "ULDC", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},
	"ULDC.64": {Opcode: "ULDC.64", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},
	"LDG.E":   {Opcode: "LDG.E", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},
	"STG.E":   {Opcode: "STG.E", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},

	// --- Logic / Bit ---
	"LOP3.LUT":  {Opcode: "LOP3.LUT", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},
	"PLOP3.LUT": {Opcode: "PLOP3.LUT", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},

	// --- Move & Special ---
	"MOV":      {Opcode: "MOV", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 1, UnitNone), stWB()}},
	"UMOV":     {Opcode: "UMOV", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 1, UnitNone), stWB()}},
	"S2R":      {Opcode: "S2R", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"S2UR":     {Opcode: "S2UR", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"MUFU.RCP": {Opcode: "MUFU.RCP", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 12, UnitSpecial), stWB()}},

	// --- Shift / Bitfield ---
	"SHF.L.U32":    {Opcode: "SHF.L.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"SHF.R.S32.HI": {Opcode: "SHF.R.S32.HI", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"SHF.R.S64":    {Opcode: "SHF.R.S64", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitSpecial), stWB()}},
	"SHF.R.U32.HI": {Opcode: "SHF.R.U32.HI", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
}
