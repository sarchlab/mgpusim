package smsp

import (
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
	"sync"
)

// ========================================
// Stage + Unit Metadata for H100 Simulation
// ========================================

// Stage names used in all instruction pipelines
//   "Decode", "Issue", "Execute", "MemoryPipeRead", "MemoryPipeWrite", "BranchResolve", "Writeback"

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

func stDecode() StageDef { return s("Decode", 0, UnitNone) }
func stIssue() StageDef  { return s("Issue", 1, UnitNone) }
func stWB() StageDef     { return s("Writeback", 0, UnitNone) }

// =======================
// Default Fallback Entry
// =======================
func defaultStages(op string) InstructionPipelineTemplate {
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

// SimilarityThreshold is the minimum normalized similarity (0..1) required
// to consider an unseen opcode "similar" enough to an existing entry.
// Editable at runtime/tests.
var SimilarityThreshold = 0.60

var (
	warnedOpcodesMu sync.Mutex
	warnedOpcodes   = map[string]bool{}
)

// warnOncef prints a warning about an opcode the first time it is seen.
func warnOncef(opcode, format string, args ...any) {
	warnedOpcodesMu.Lock()
	defer warnedOpcodesMu.Unlock()

	if warnedOpcodes[opcode] {
		return
	}

	warnedOpcodes[opcode] = true

	log.Printf(format, args...)
}

// resolvedTemplates caches the result of the similarity search for opcodes
// that are not in PipelineTable.
var resolvedTemplates sync.Map

// getPipelineStages returns a pipeline template for opcode. If there is no
// exact match in PipelineTable, it searches for the most similar existing
// opcode and returns that template if similarity >= SimilarityThreshold.
// Otherwise it falls back to defaultStages(opcode).
func getPipelineStages(opcode string) InstructionPipelineTemplate {
	uc := strings.ToUpper(opcode)
	if pipeline, exists := PipelineTable[uc]; exists {
		return pipeline
	}

	if tpl, found := resolvedTemplates.Load(opcode); found {
		return tpl.(InstructionPipelineTemplate)
	}

	tpl := findSimilarPipelineStages(opcode, uc)
	resolvedTemplates.Store(opcode, tpl)

	return tpl
}

func findSimilarPipelineStages(
	opcode, uc string,
) InstructionPipelineTemplate {
	// Find best match by normalized Levenshtein similarity. Keys are visited
	// in sorted order so that ties are broken deterministically.
	bestSim := 0.0

	var bestKey string

	for _, k := range slices.Sorted(maps.Keys(PipelineTable)) {
		sim := normalizedSimilarity(uc, strings.ToUpper(k))
		if sim > bestSim {
			bestSim = sim
			bestKey = k
		}
	}

	if bestSim >= SimilarityThreshold {
		warnOncef(opcode,
			"opcode %s is not in the pipeline table, using %s (similarity %.2f)",
			opcode, bestKey, bestSim)

		return PipelineTable[bestKey]
	}

	warnOncef(opcode,
		"opcode %s is not in the pipeline table, using the default pipeline",
		opcode)

	return defaultStages(opcode)
}

// =======================
// Pipeline Table (H100 PCIe model)
// =======================

func alu(op string, unit ExecUnitKind, cycles int) InstructionPipelineTemplate {
	return InstructionPipelineTemplate{Opcode: op, Stages: []StageDef{
		stDecode(), s("Issue", 1, unit), s("Execute", cycles, unit), stWB(),
	}}
}

func issueOnly(op string, unit ExecUnitKind, cycles int) InstructionPipelineTemplate {
	return InstructionPipelineTemplate{Opcode: op, Stages: []StageDef{
		stDecode(), s("Issue", cycles, unit), stWB(),
	}}
}

// Memory stages take two steps: sending the request and receiving the
// response.
func memRead(op string) InstructionPipelineTemplate {
	return InstructionPipelineTemplate{Opcode: op, Stages: []StageDef{
		stDecode(), s("MemoryPipeRead", 2, UnitLdSt), stWB(),
	}}
}

func memWrite(op string) InstructionPipelineTemplate {
	return InstructionPipelineTemplate{Opcode: op, Stages: []StageDef{
		stDecode(), s("MemoryPipeWrite", 2, UnitLdSt), stWB(),
	}}
}

func branch(op string, cycles int) InstructionPipelineTemplate {
	return InstructionPipelineTemplate{Opcode: op, Stages: []StageDef{
		stDecode(), s("BranchResolve", cycles, UnitNone), stWB(),
	}}
}

var PipelineTable = map[string]InstructionPipelineTemplate{
	// --- Control Flow ---
	"BRA":           branch("BRA", 2),
	"EXIT":          branch("EXIT", 1),
	"RET.REL.NODEC": branch("RET.REL.NODEC", 2),

	// --- Synchronization / Barriers ---
	"BAR.SYNC.DEFER_BLOCKING": alu("BAR.SYNC.DEFER_BLOCKING", UnitSpecial, 7),
	"BSSY":                    alu("BSSY", UnitSpecial, 1),
	"BSYNC":                   alu("BSYNC", UnitSpecial, 7),

	// --- Type Conversion ---
	"F2I.FTZ.U32.TRUNC.NTZ": alu("F2I.FTZ.U32.TRUNC.NTZ", UnitFP32, 3),
	"I2F.U32.RP":            alu("I2F.U32.RP", UnitInt, 4),
	"I2F.RP":                alu("I2F.RP", UnitInt, 3),
	"I2FP.F32.S32":          alu("I2FP.F32.S32", UnitInt, 3),

	// --- FP32 Arithmetic ---
	"FADD":     alu("FADD", UnitFP32, 3),
	"FADD.FTZ": alu("FADD.FTZ", UnitFP32, 3),
	"FFMA":     alu("FFMA", UnitFP32, 3),
	"FFMA.SAT": alu("FFMA.SAT", UnitFP32, 3),
	"FFMA.RM":  alu("FFMA.RM", UnitFP32, 3),
	"FMUL":     alu("FMUL", UnitFP32, 3),
	"FMUL.FTZ": alu("FMUL.FTZ", UnitFP32, 3),
	"FMUL.D2":  alu("FMUL.D2", UnitFP32, 3),

	// --- FP32 Predicates & Special ---
	"FSETP.GEU.AND":     alu("FSETP.GEU.AND", UnitFP32, 1),
	"FSETP.GTU.FTZ.AND": alu("FSETP.GTU.FTZ.AND", UnitFP32, 1),
	"FSETP.NEU.AND":     alu("FSETP.NEU.AND", UnitFP32, 1),
	"FSETP.NEU.FTZ.AND": alu("FSETP.NEU.FTZ.AND", UnitFP32, 1),
	"FSEL":              alu("FSEL", UnitFP32, 1),
	"FCHK":              alu("FCHK", UnitFP32, 1),

	// --- FP64 Arithmetic ---
	"DFMA": alu("DFMA", UnitFP64, 7),
	"DMUL": alu("DMUL", UnitFP64, 7),

	// --- FP64 Predicates ---
	"DSETP.NEU.AND": alu("DSETP.NEU.AND", UnitFP64, 1),

	// --- Tensor / Half ---
	"HFMA2.MMA": alu("HFMA2.MMA", UnitTensor, 7),

	// --- INT ALU ---
	"IADD3":    alu("IADD3", UnitInt, 1),
	"IADD3.X":  alu("IADD3.X", UnitInt, 1),
	"UIADD3":   alu("UIADD3", UnitInt, 1),
	"UIADD3.X": alu("UIADD3.X", UnitInt, 1),
	"VIADD":    alu("VIADD", UnitInt, 1),
	"IABS":     alu("IABS", UnitInt, 1),

	// --- INT Multiply-Add ---
	"IMAD":           alu("IMAD", UnitInt, 2),
	"IMAD.HI.U32":    alu("IMAD.HI.U32", UnitInt, 2),
	"IMAD.IADD":      alu("IMAD.IADD", UnitInt, 2),
	"IMAD.MOV":       alu("IMAD.MOV", UnitInt, 1),
	"IMAD.MOV.U32":   alu("IMAD.MOV.U32", UnitInt, 1),
	"IMAD.U32":       alu("IMAD.U32", UnitInt, 2),
	"IMAD.WIDE":      alu("IMAD.WIDE", UnitInt, 3),
	"IMAD.WIDE.U32":  alu("IMAD.WIDE.U32", UnitInt, 3),
	"IMAD.X":         alu("IMAD.X", UnitInt, 2),
	"IMAD.SHL.U32":   alu("IMAD.SHL.U32", UnitInt, 2),
	"UIMAD":          alu("UIMAD", UnitInt, 2),
	"UIMAD.WIDE":     alu("UIMAD.WIDE", UnitInt, 3),
	"UIMAD.WIDE.U32": alu("UIMAD.WIDE.U32", UnitInt, 3),

	// --- LEA (Load Effective Address) ---
	"LEA":           alu("LEA", UnitInt, 2),
	"LEA.HI":        alu("LEA.HI", UnitInt, 2),
	"LEA.HI.X":      alu("LEA.HI.X", UnitInt, 2),
	"LEA.HI.X.SX32": alu("LEA.HI.X.SX32", UnitInt, 2),
	"ULEA":          alu("ULEA", UnitInt, 2),
	"ULEA.HI":       alu("ULEA.HI", UnitInt, 2),

	// --- Vector INT Operations ---
	"VIADDMNMX":     alu("VIADDMNMX", UnitInt, 2),
	"VIADDMNMX.U32": alu("VIADDMNMX.U32", UnitInt, 2),
	"VIMNMX":        alu("VIMNMX", UnitInt, 1),
	"VIMNMX.U32":    alu("VIMNMX.U32", UnitInt, 1),
	"VIMNMX3":       alu("VIMNMX3", UnitInt, 2),

	// --- Integer Predicates / Compare ---
	"ISETP.EQ.OR":         alu("ISETP.EQ.OR", UnitSpecial, 1),
	"ISETP.GE.AND":        alu("ISETP.GE.AND", UnitSpecial, 1),
	"ISETP.GE.OR":         alu("ISETP.GE.OR", UnitSpecial, 1),
	"ISETP.GE.U32.AND":    alu("ISETP.GE.U32.AND", UnitSpecial, 1),
	"ISETP.GE.U32.AND.EX": alu("ISETP.GE.U32.AND.EX", UnitSpecial, 1),
	"ISETP.GT.AND":        alu("ISETP.GT.AND", UnitSpecial, 1),
	"ISETP.GT.AND.EX":     alu("ISETP.GT.AND.EX", UnitSpecial, 1),
	"ISETP.GT.U32.AND":    alu("ISETP.GT.U32.AND", UnitSpecial, 1),
	"ISETP.GT.U32.AND.EX": alu("ISETP.GT.U32.AND.EX", UnitSpecial, 1),
	"ISETP.GT.U32.OR":     alu("ISETP.GT.U32.OR", UnitSpecial, 1),
	"ISETP.LE.AND":        alu("ISETP.LE.AND", UnitSpecial, 1),
	"ISETP.LE.OR":         alu("ISETP.LE.OR", UnitSpecial, 1),
	"ISETP.LE.U32.AND":    alu("ISETP.LE.U32.AND", UnitSpecial, 1),
	"ISETP.LT.OR":         alu("ISETP.LT.OR", UnitSpecial, 1),
	"ISETP.LT.U32.AND":    alu("ISETP.LT.U32.AND", UnitSpecial, 1),
	"ISETP.NE.AND":        alu("ISETP.NE.AND", UnitSpecial, 1),
	"ISETP.NE.OR":         alu("ISETP.NE.OR", UnitSpecial, 1),
	"ISETP.NE.U32.AND":    alu("ISETP.NE.U32.AND", UnitSpecial, 1),
	"UISETP.GE.AND":       alu("UISETP.GE.AND", UnitSpecial, 1),
	"UISETP.GT.AND":       alu("UISETP.GT.AND", UnitSpecial, 1),

	// --- Load / Store (Constant Cache) ---
	"LDC":     alu("LDC", UnitSpecial, 1),
	"LDC.64":  alu("LDC.64", UnitSpecial, 1),
	"ULDC":    alu("ULDC", UnitSpecial, 1),
	"ULDC.64": alu("ULDC.64", UnitSpecial, 1),

	// --- Load / Store (Global) ---
	"LDG.E":             memRead("LDG.E"),
	"LDG.E.64.CONSTANT": memRead("LDG.E.64.CONSTANT"),
	"LDG.E.128":         memRead("LDG.E.128"),
	"LDG.E.CONSTANT":    memRead("LDG.E.CONSTANT"),
	"LDG.E.U8":          memRead("LDG.E.U8"),
	"LDG.E.U8.CONSTANT": memRead("LDG.E.U8.CONSTANT"),
	"LDG.E.STRONG.SYS":  memRead("LDG.E.STRONG.SYS"),
	"STG.E":             memWrite("STG.E"),
	"STG.E.64":          memWrite("STG.E.64"),
	"STG.E.128":         memWrite("STG.E.128"),
	"STG.E.U8":          memWrite("STG.E.U8"),

	// --- Load / Store (Shared) ---
	"LDS":     memRead("LDS"),
	"LDS.64":  memRead("LDS.64"),
	"LDS.128": memRead("LDS.128"),
	"STS":     memWrite("STS"),
	"STS.64":  memWrite("STS.64"),
	"STS.128": memWrite("STS.128"),

	// --- Load / Store (Local) ---
	"LDL":    memRead("LDL"),
	"STL.64": memWrite("STL.64"),

	// --- Atomic / Reduction ---
	"REDG.E.ADD.F32.FTZ.RN.STRONG.GPU": alu("REDG.E.ADD.F32.FTZ.RN.STRONG.GPU", UnitSpecial, 1),

	// --- Logic / Bit ---
	"LOP3.LUT":  alu("LOP3.LUT", UnitInt, 1),
	"ULOP3.LUT": alu("ULOP3.LUT", UnitInt, 1),
	"PLOP3.LUT": alu("PLOP3.LUT", UnitSpecial, 1),

	// --- Move & Special ---
	"MOV":  issueOnly("MOV", UnitNone, 1),
	"UMOV": issueOnly("UMOV", UnitNone, 1),
	"SEL":  issueOnly("SEL", UnitInt, 1),
	"PRMT": alu("PRMT", UnitInt, 1),

	// --- Special Register Access ---
	"S2R":  alu("S2R", UnitSpecial, 1),
	"S2UR": alu("S2UR", UnitSpecial, 1),
	"R2UR": alu("R2UR", UnitSpecial, 1),
	"CS2R": alu("CS2R", UnitSpecial, 1),

	// --- Multi-Function Special Unit (MUFU) ---
	"MUFU.RCP": alu("MUFU.RCP", UnitSpecial, 11),
	"MUFU.RSQ": alu("MUFU.RSQ", UnitSpecial, 11),
	"MUFU.EX2": alu("MUFU.EX2", UnitSpecial, 11),

	// --- Shift / Bitfield ---
	"SHF.L.U32":     alu("SHF.L.U32", UnitSpecial, 1),
	"SHF.L.U64.HI":  alu("SHF.L.U64.HI", UnitSpecial, 2),
	"SHF.R.S32.HI":  alu("SHF.R.S32.HI", UnitSpecial, 1),
	"SHF.R.S64":     alu("SHF.R.S64", UnitSpecial, 2),
	"SHF.R.U32.HI":  alu("SHF.R.U32.HI", UnitSpecial, 1),
	"USHF.L.U32":    alu("USHF.L.U32", UnitSpecial, 1),
	"USHF.L.U64.HI": alu("USHF.L.U64.HI", UnitSpecial, 2),
	"USHF.R.S32.HI": alu("USHF.R.S32.HI", UnitSpecial, 1),
	"USHF.R.U32.HI": alu("USHF.R.U32.HI", UnitSpecial, 1),

	// --- Bit Scan / Count ---
	"FLO.U32":  alu("FLO.U32", UnitSpecial, 2),
	"UFLO.U32": alu("UFLO.U32", UnitSpecial, 2),
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
