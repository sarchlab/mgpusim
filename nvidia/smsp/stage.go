package smsp

import (
	"bufio"
	"fmt"
	"os"
	"strings"

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
			s("Execute", 1, UnitNone),
			stWB(),
		},
	}
}

func appendToFileIfNotExists(filePath, line string) {
	// Open the file (create if not exists)
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.WithField("error", err).Error("Failed to open unknownopcode.log")
		return
	}
	defer file.Close()

	// Check if the line already exists
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == line {
			return // Line already exists, skip
		}
	}

	// Append the line to the file
	_, err = file.WriteString(line + "\n")
	if err != nil {
		log.WithField("error", err).Error("Failed to write to unknownopcode.log")
	}
}

// SimilarityThreshold is the minimum normalized similarity (0..1) required
// to consider an unseen opcode "similar" enough to an existing entry.
// Editable at runtime/tests.
var SimilarityThreshold = 0.60

// getPipelineStages returns a pipeline template for opcode. If there is no
// exact match in PipelineTable, it searches for the most similar existing
// opcode and returns that template if similarity >= SimilarityThreshold.
// Otherwise it falls back to defaultStages(opcode).
func getPipelineStages(opcode string) InstructionPipelineTemplate {
	uc := strings.ToUpper(opcode)
	if pipeline, exists := PipelineTable[uc]; exists {
		return pipeline
	}

	// Find best match by normalized Levenshtein similarity.
	bestSim := 0.0
	var bestKey string
	for k := range PipelineTable {
		sim := normalizedSimilarity(uc, strings.ToUpper(k))
		if sim > bestSim {
			bestSim = sim
			bestKey = k
		}
	}

	if bestSim >= SimilarityThreshold {
		log.WithFields(log.Fields{
			"opcode":        opcode,
			"best_match":    bestKey,
			"similarity":    bestSim,
			"threshold":     SimilarityThreshold,
			"using_default": false,
		}).Info("getPipelineStages: using similar pipeline template")
		unknownLog := fmt.Sprintf("%s -> %s", opcode, bestKey)
		appendToFileIfNotExists("unknownopcode_getPipelineStages.log", unknownLog)
		return PipelineTable[bestKey]
	}

	unknownLog := fmt.Sprintf("%s", opcode)
	appendToFileIfNotExists("unknownopcode_getPipelineStages.log", unknownLog)
	// Nothing similar enough; fall back to default behaviour.
	return defaultStages(opcode)
}

// =======================
// Pipeline Table (H100 PCIe model)
// =======================

var PipelineTable = map[string]InstructionPipelineTemplate{
	// --- Control Flow ---
	"BRA":           {Opcode: "BRA", Stages: []StageDef{stDecode(), stIssue(), s("BranchResolve", 2, UnitNone), stWB()}},
	"EXIT":          {Opcode: "EXIT", Stages: []StageDef{stDecode(), stIssue(), s("BranchResolve", 1, UnitNone), stWB()}},
	"RET.REL.NODEC": {Opcode: "RET.REL.NODEC", Stages: []StageDef{stDecode(), stIssue(), s("BranchResolve", 2, UnitNone), stWB()}}, // added

	// --- Synchronization / Barriers ---
	"BAR.SYNC.DEFER_BLOCKING": {Opcode: "BAR.SYNC.DEFER_BLOCKING", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 8, UnitSpecial), stWB()}}, // added
	"BSSY":                    {Opcode: "BSSY", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},                    // added
	"BSYNC":                   {Opcode: "BSYNC", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 8, UnitSpecial), stWB()}},                   // added

	// --- Type Conversion ---
	"F2I.FTZ.U32.TRUNC.NTZ": {Opcode: "F2I...", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32), stWB()}},
	"I2F.U32.RP":            {Opcode: "I2F...", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt), stWB()}},
	"I2F.RP":                {Opcode: "I2F.RP", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt), stWB()}},       // added
	"I2FP.F32.S32":          {Opcode: "I2FP.F32.S32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt), stWB()}}, // added

	// --- FP32 Arithmetic ---
	"FADD":     {Opcode: "FADD", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32), stWB()}},
	"FADD.FTZ": {Opcode: "FADD.FTZ", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32), stWB()}}, // added
	"FFMA":     {Opcode: "FFMA", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32), stWB()}},
	"FFMA.SAT": {Opcode: "FFMA.SAT", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32), stWB()}}, // added
	"FFMA.RM":  {Opcode: "FFMA.RM", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32), stWB()}},  // added
	"FMUL":     {Opcode: "FMUL", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32), stWB()}},
	"FMUL.FTZ": {Opcode: "FMUL.FTZ", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32), stWB()}}, // added
	"FMUL.D2":  {Opcode: "FMUL.D2", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitFP32), stWB()}},  // added

	// --- FP32 Predicates & Special ---
	"FSETP.GEU.AND":     {Opcode: "FSETP.GEU.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitFP32), stWB()}},     // added
	"FSETP.GTU.FTZ.AND": {Opcode: "FSETP.GTU.FTZ.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitFP32), stWB()}}, // added
	"FSETP.NEU.AND":     {Opcode: "FSETP.NEU.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitFP32), stWB()}},     // added
	"FSETP.NEU.FTZ.AND": {Opcode: "FSETP.NEU.FTZ.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitFP32), stWB()}}, // added
	"FSEL":              {Opcode: "FSEL", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitFP32), stWB()}},              // added
	"FCHK":              {Opcode: "FCHK", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitFP32), stWB()}},              // added

	// --- FP64 Arithmetic ---
	"DFMA": {Opcode: "DFMA", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 8, UnitFP64), stWB()}}, // added
	"DMUL": {Opcode: "DMUL", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 8, UnitFP64), stWB()}}, // added

	// --- FP64 Predicates ---
	"DSETP.NEU.AND": {Opcode: "DSETP.NEU.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitFP64), stWB()}}, // added

	// --- Tensor / Half ---
	"HFMA2.MMA": {Opcode: "HFMA2.MMA", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 8, UnitTensor), stWB()}},

	// --- INT ALU ---
	"IADD3":    {Opcode: "IADD3", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},
	"IADD3.X":  {Opcode: "IADD3.X", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},
	"UIADD3":   {Opcode: "UIADD3", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},
	"UIADD3.X": {Opcode: "UIADD3.X", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}}, // added
	"VIADD":    {Opcode: "VIADD", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},
	"IABS":     {Opcode: "IABS", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}}, // added

	// --- INT Multiply-Add ---
	"IMAD":           {Opcode: "IMAD", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},
	"IMAD.HI.U32":    {Opcode: "IMAD.HI.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},
	"IMAD.IADD":      {Opcode: "IMAD.IADD", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},
	"IMAD.MOV":       {Opcode: "IMAD.MOV", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},
	"IMAD.MOV.U32":   {Opcode: "IMAD.MOV.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},
	"IMAD.U32":       {Opcode: "IMAD.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},
	"IMAD.WIDE":      {Opcode: "IMAD.WIDE", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt), stWB()}},
	"IMAD.WIDE.U32":  {Opcode: "IMAD.WIDE.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt), stWB()}},
	"IMAD.X":         {Opcode: "IMAD.X", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},       // added
	"IMAD.SHL.U32":   {Opcode: "IMAD.SHL.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}}, // added
	"UIMAD":          {Opcode: "UIMAD", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},        // added
	"UIMAD.WIDE":     {Opcode: "UIMAD.WIDE", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt), stWB()}},
	"UIMAD.WIDE.U32": {Opcode: "UIMAD.WIDE.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 4, UnitInt), stWB()}}, // added

	// --- LEA (Load Effective Address) ---
	"LEA":           {Opcode: "LEA", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},
	"LEA.HI":        {Opcode: "LEA.HI", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}}, // added
	"LEA.HI.X":      {Opcode: "LEA.HI.X", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},
	"LEA.HI.X.SX32": {Opcode: "LEA.HI.X.SX32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},
	"ULEA":          {Opcode: "ULEA", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},    // added
	"ULEA.HI":       {Opcode: "ULEA.HI", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}}, // added

	// --- Vector INT Operations ---
	"VIADDMNMX":     {Opcode: "VIADDMNMX", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},     // added
	"VIADDMNMX.U32": {Opcode: "VIADDMNMX.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}}, // added
	"VIMNMX":        {Opcode: "VIMNMX", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},        // added
	"VIMNMX.U32":    {Opcode: "VIMNMX.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},    // added
	"VIMNMX3":       {Opcode: "VIMNMX3", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitInt), stWB()}},       // added

	// --- Integer Predicates / Compare ---
	"ISETP.EQ.OR":         {Opcode: "ISETP.EQ.OR", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}}, // added
	"ISETP.GE.AND":        {Opcode: "ISETP.GE.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.GE.OR":         {Opcode: "ISETP.GE.OR", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}}, // added
	"ISETP.GE.U32.AND":    {Opcode: "ISETP.GE.U32.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.GE.U32.AND.EX": {Opcode: "ISETP.GE.U32.AND.EX", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}}, // added
	"ISETP.GT.AND":        {Opcode: "ISETP.GT.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.GT.AND.EX":     {Opcode: "ISETP.GT.AND.EX", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},  // added
	"ISETP.GT.U32.AND":    {Opcode: "ISETP.GT.U32.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}}, // added
	"ISETP.GT.U32.AND.EX": {Opcode: "ISETP.GT.U32.AND.EX", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.GT.U32.OR":     {Opcode: "ISETP.GT.U32.OR", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},  // added
	"ISETP.LE.AND":        {Opcode: "ISETP.LE.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},     // added
	"ISETP.LE.OR":         {Opcode: "ISETP.LE.OR", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},      // added
	"ISETP.LE.U32.AND":    {Opcode: "ISETP.LE.U32.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}}, // added
	"ISETP.LT.OR":         {Opcode: "ISETP.LT.OR", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},      // added
	"ISETP.LT.U32.AND":    {Opcode: "ISETP.LT.U32.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.NE.AND":        {Opcode: "ISETP.NE.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.NE.OR":         {Opcode: "ISETP.NE.OR", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"ISETP.NE.U32.AND":    {Opcode: "ISETP.NE.U32.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"UISETP.GE.AND":       {Opcode: "UISETP.GE.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}}, // added
	"UISETP.GT.AND":       {Opcode: "UISETP.GT.AND", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}}, // added

	// --- Load / Store (Constant Cache) ---
	"LDC":     {Opcode: "LDC", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},
	"LDC.64":  {Opcode: "LDC.64", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}}, // added
	"ULDC":    {Opcode: "ULDC", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},
	"ULDC.64": {Opcode: "ULDC.64", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},

	// --- Load / Store (Global) ---
	"LDG.E":             {Opcode: "LDG.E", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},
	"LDG.E.64.CONSTANT": {Opcode: "LDG.E.64.CONSTANT", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}}, // added
	"LDG.E.128":         {Opcode: "LDG.E.128", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},         // added
	"LDG.E.CONSTANT":    {Opcode: "LDG.E.CONSTANT", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},    // added
	"LDG.E.U8":          {Opcode: "LDG.E.U8", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},          // added
	"LDG.E.U8.CONSTANT": {Opcode: "LDG.E.U8.CONSTANT", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}}, // added
	"STG.E":             {Opcode: "STG.E", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},
	"STG.E.64":          {Opcode: "STG.E.64", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},  // added
	"STG.E.128":         {Opcode: "STG.E.128", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}}, // added
	"STG.E.U8":          {Opcode: "STG.E.U8", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},  // added

	// --- Load / Store (Shared) ---
	"LDS":     {Opcode: "LDS", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},     // added
	"LDS.64":  {Opcode: "LDS.64", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},  // added
	"LDS.128": {Opcode: "LDS.128", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}}, // added
	"STS":     {Opcode: "STS", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},     // added
	"STS.64":  {Opcode: "STS.64", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},  // added
	"STS.128": {Opcode: "STS.128", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}}, // added

	// --- Load / Store (Local) ---
	"LDL":    {Opcode: "LDL", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}},    // added
	"STL.64": {Opcode: "STL.64", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 1, UnitLdSt), stWB()}}, // added

	// --- Atomic / Reduction ---
	"REDG.E.ADD.F32.FTZ.RN.STRONG.GPU": {Opcode: "REDG.E.ADD.F32.FTZ.RN.STRONG.GPU", Stages: []StageDef{stDecode(), stIssue(), s("MemoryPipe", 4, UnitLdSt), stWB()}}, // added

	// --- Logic / Bit ---
	"LOP3.LUT":  {Opcode: "LOP3.LUT", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}},
	"ULOP3.LUT": {Opcode: "ULOP3.LUT", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}}, // added
	"PLOP3.LUT": {Opcode: "PLOP3.LUT", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},

	// --- Move & Special ---
	"MOV":  {Opcode: "MOV", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 1, UnitNone), stWB()}},
	"UMOV": {Opcode: "UMOV", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 1, UnitNone), stWB()}},
	"SEL":  {Opcode: "SEL", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 1, UnitInt), stWB()}},  // added
	"PRMT": {Opcode: "PRMT", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitInt), stWB()}}, // added

	// --- Special Register Access ---
	"S2R":  {Opcode: "S2R", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"S2UR": {Opcode: "S2UR", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"R2UR": {Opcode: "R2UR", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}}, // added
	"CS2R": {Opcode: "CS2R", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}}, // added

	// --- Multi-Function Special Unit (MUFU) ---
	"MUFU.RCP": {Opcode: "MUFU.RCP", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 12, UnitSpecial), stWB()}},
	"MUFU.RSQ": {Opcode: "MUFU.RSQ", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 12, UnitSpecial), stWB()}}, // added
	"MUFU.EX2": {Opcode: "MUFU.EX2", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 12, UnitSpecial), stWB()}}, // added

	// --- Shift / Bitfield ---
	"SHF.L.U32":     {Opcode: "SHF.L.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"SHF.L.U64.HI":  {Opcode: "SHF.L.U64.HI", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitSpecial), stWB()}}, // added
	"SHF.R.S32.HI":  {Opcode: "SHF.R.S32.HI", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"SHF.R.S64":     {Opcode: "SHF.R.S64", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitSpecial), stWB()}},
	"SHF.R.U32.HI":  {Opcode: "SHF.R.U32.HI", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},
	"USHF.L.U32":    {Opcode: "USHF.L.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}},    // added
	"USHF.L.U64.HI": {Opcode: "USHF.L.U64.HI", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitSpecial), stWB()}}, // added
	"USHF.R.S32.HI": {Opcode: "USHF.R.S32.HI", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}}, // added
	"USHF.R.U32.HI": {Opcode: "USHF.R.U32.HI", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 2, UnitSpecial), stWB()}}, // added

	// --- Bit Scan / Count ---
	"FLO.U32":  {Opcode: "FLO.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitSpecial), stWB()}},  // added
	"UFLO.U32": {Opcode: "UFLO.U32", Stages: []StageDef{stDecode(), stIssue(), s("Execute", 3, UnitSpecial), stWB()}}, // added
}

// levenshteinDistance returns the Levenshtein edit distance between a and b.
func levenshteinDistance(a, b string) int {
	la := len(a)
	lb := len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	cur := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			deletion := prev[j] + 1
			insertion := cur[j-1] + 1
			subst := prev[j-1] + cost
			min := deletion
			if insertion < min {
				min = insertion
			}
			if subst < min {
				min = subst
			}
			cur[j] = min
		}
		prev, cur = cur, prev
	}
	return prev[lb]
}

// normalizedSimilarity computes a 0..1 similarity score based on
// normalized Levenshtein distance: 1 - dist / maxLen.
func normalizedSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	la := len(a)
	lb := len(b)
	maxLen := la
	if lb > maxLen {
		maxLen = lb
	}
	if maxLen == 0 {
		return 1.0
	}
	dist := levenshteinDistance(a, b)
	return 1.0 - float64(dist)/float64(maxLen)
}
