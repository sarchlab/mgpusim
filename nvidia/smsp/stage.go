package smsp

// =====================
// Stage + Unit Metadata
// =====================

// Possible Stage Name:
//   "Fetch", "Decode", "Issue", "Scoreboard", "Execute", "Writeback", "MemoryPipe", "BranchResolve"

// Possible Unit:
//   UnitNone       - No execution unit / bookkeeping stage
//   UnitInt        - Integer ALU pipeline
//   UnitFP32       - FP32 ALU pipeline
//   UnitFP64       - FP64 ALU pipeline
//   UnitTensor     - Tensor Core pipeline
//   UnitLdSt       - Load/Store pipeline
//   UnitSpecial    - Special function unit (MUFU, shift, permute, predicate ops)

// ================
// Types
// ================
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

type StageDef struct {
	Name      string
	Latency   int
	Unit      ExecUnitKind
	UnitsUsed int
}

type InstructionPipelineTemplate struct {
	Opcode string
	Stages []StageDef
}

// =======================
// Helper Stage Shortcuts
// =======================
func s(name string, cycles int, unit ExecUnitKind, u int) StageDef {
	return StageDef{Name: name, Latency: cycles, Unit: unit, UnitsUsed: u}
}
func stDecode() StageDef { return s("Decode", 1, UnitNone, 0) }
func stIssue() StageDef  { return s("Issue", 1, UnitNone, 0) }
func stWB() StageDef     { return s("Writeback", 1, UnitNone, 0) }

// =======================
// Default Fallback Entry
// =======================
func defaultStages(op string) InstructionPipelineTemplate {
	println("[WARN] PipelineTable missing entry for: ", op)
	return InstructionPipelineTemplate{
		Opcode: op,
		Stages: []StageDef{
			stDecode(),
			stIssue(),
			s("Execute", 4, UnitNone, 0),
			stWB(),
		},
	}
}

// =======================
// Pipeline Table
// =======================
var PipelineTable = map[string]InstructionPipelineTemplate{
	// --- Control Flow ---
	"BRA":  {Opcode: "BRA", Stages: []StageDef{stDecode(), stIssue(), s("BranchResolve", 2, UnitNone, 0), stWB()}},
	"EXIT": {Opcode: "EXIT", Stages: []StageDef{stDecode(), stIssue(), s("BranchResolve", 1, UnitNone, 0), stWB()}},

	// --- Type Conversion ---
	"F2I.FTZ.U32.TRUNC.NTZ": {Opcode: "F2I...", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32, 1), stWB()}},
	"I2F.U32.RP":            {Opcode: "I2F...", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt, 1), stWB()}},

	// --- FP32 ---
	"FADD": {Opcode: "FADD", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32, 1), stWB()}},
	"FFMA": {Opcode: "FFMA", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32, 1), stWB()}},
	"FMUL": {Opcode: "FMUL", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32, 1), stWB()}},

	// --- Tensor / Half ---
	"HFMA2.MMA": {Opcode: "HFMA2.MMA", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 8, UnitTensor, 4), stWB()}},

	// --- INT ALU ---
	"IADD3":   {Opcode: "IADD3", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt, 1), stWB()}},
	"IADD3.X": {Opcode: "IADD3.X", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt, 1), stWB()}},
	"UIADD3":  {Opcode: "UIADD3", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt, 1), stWB()}},
	"VIADD":   {Opcode: "VIADD", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt, 1), stWB()}},

	// --- INT MUL / MAD ---
	"IMAD":          {Opcode: "IMAD", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt, 1), stWB()}},
	"IMAD.HI.U32":   {Opcode: "IMAD.HI.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt, 1), stWB()}},
	"IMAD.IADD":     {Opcode: "IMAD.IADD", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt, 1), stWB()}},
	"IMAD.MOV":      {Opcode: "IMAD.MOV", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt, 1), stWB()}},
	"IMAD.MOV.U32":  {Opcode: "IMAD.MOV.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt, 1), stWB()}},
	"IMAD.U32":      {Opcode: "IMAD.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt, 1), stWB()}},
	"IMAD.WIDE":     {Opcode: "IMAD.WIDE", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt, 2), stWB()}},
	"IMAD.WIDE.U32": {Opcode: "IMAD.WIDE.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt, 2), stWB()}},
	"UIMAD.WIDE":    {Opcode: "UIMAD.WIDE", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt, 2), stWB()}},

	// --- Predicate / Compare ---
	"ISETP.GE.AND":        {Opcode: "ISETP.GE.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},
	"ISETP.GE.U32.AND":    {Opcode: "ISETP.GE.U32.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},
	"ISETP.GT.AND":        {Opcode: "ISETP.GT.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},
	"ISETP.GT.U32.AND.EX": {Opcode: "ISETP.GT.U32.AND.EX", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},
	"ISETP.LT.U32.AND":    {Opcode: "ISETP.LT.U32.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},
	"ISETP.NE.AND":        {Opcode: "ISETP.NE.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},
	"ISETP.NE.OR":         {Opcode: "ISETP.NE.OR", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},
	"ISETP.NE.U32.AND":    {Opcode: "ISETP.NE.U32.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},

	// --- Address Calc ---
	"LEA":           {Opcode: "LEA", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt, 1), stWB()}},
	"LEA.HI.X":      {Opcode: "LEA.HI.X", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt, 1), stWB()}},
	"LEA.HI.X.SX32": {Opcode: "LEA.HI.X.SX32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt, 1), stWB()}},

	// --- Load / Store ---
	"LDC":     {Opcode: "LDC", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 20, UnitLdSt, 1), stWB()}},
	"ULDC":    {Opcode: "ULDC", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 20, UnitLdSt, 1), stWB()}},
	"ULDC.64": {Opcode: "ULDC.64", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 20, UnitLdSt, 1), stWB()}},

	"LDG.E": {Opcode: "LDG.E", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 40, UnitLdSt, 1), stWB()}},
	"STG.E": {Opcode: "STG.E", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 30, UnitLdSt, 1), stWB()}},

	// --- Logic / Bit ---
	"LOP3.LUT":  {Opcode: "LOP3.LUT", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt, 1), stWB()}},
	"PLOP3.LUT": {Opcode: "PLOP3.LUT", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},

	// --- Move & Special ---
	"MOV":      {Opcode: "MOV", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 1, UnitNone, 0), stWB()}},
	"UMOV":     {Opcode: "UMOV", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 1, UnitNone, 0), stWB()}},
	"S2R":      {Opcode: "S2R", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},
	"S2UR":     {Opcode: "S2UR", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},
	"MUFU.RCP": {Opcode: "MUFU.RCP", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 12, UnitSpecial, 1), stWB()}},

	// --- Shift / Bitfield ---
	"SHF.L.U32":    {Opcode: "SHF.L.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},
	"SHF.R.S32.HI": {Opcode: "SHF.R.S32.HI", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},
	"SHF.R.S64":    {Opcode: "SHF.R.S64", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitSpecial, 1), stWB()}},
	"SHF.R.U32.HI": {Opcode: "SHF.R.U32.HI", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial, 1), stWB()}},
}
