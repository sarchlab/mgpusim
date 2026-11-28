package loopkernels

import (
	"fmt"
	"math"
	"slices"
)

// The embedded code objects. A Kernel names the one that holds it.
const (
	InstructionCostObject      = "instruction_cost_kernels.hsaco"
	InstructionCostOtherObject = "instruction_cost_other_kernels.hsaco"
	LoopWaitObject             = "loop_wait_kernels.hsaco"
)

// Kernel is one launchable kernel of an embedded code object and the 64-bit
// value every thread must store.
type Kernel struct {
	Name     string
	Object   string
	Expected uint64
}

// InstructionCostPairs are the instruction_cost bodies that alternate two VALU
// instructions, starting with the first (IC_FOR_EACH_PAIR).
var InstructionCostPairs = []string{"v_mul_lo_u32_v_add_u32", "v_mov_b64_v_fma_f64"}

// InstructionCostScalars are the instruction_cost bodies of one scalar or
// branch instruction (IC_FOR_EACH_SCALAR). Each has one kernel per
// ops_per_iter, whatever the dependency.
var InstructionCostScalars = []string{"s_add_u32", "s_cbranch_scc0", "s_branch"}

// InstructionCostDependencies are the values of the dependency parameter.
var InstructionCostDependencies = []string{"chained", "independent"}

// InstructionCostOpsPerIter are the instruction_cost loop-body sizes that
// have a kernel; 0 is ic_empty.
var InstructionCostOpsPerIter = []int{0, 1, 2, 4, 8, 16}

// LoopWaitLayouts are the loop_wait layouts (LW_FOR_EACH_LAYOUT).
var LoopWaitLayouts = []string{
	"head_offset_8",
	"head_offset_20",
	"head_offset_28",
	"head_offset_52",
	"add_at_head",
	"valu_before_branch",
	"compiled",
	"sgpr_at_head",
	"sgpr_in_middle",
}

// LoopWaitOpsPerIter are the loop_wait loop-body sizes that have a kernel.
var LoopWaitOpsPerIter = []int{2, 4, 8, 16}

// InstructionCostInstructions are every value of the instruction_cost
// instruction parameter.
func InstructionCostInstructions() []string {
	return slices.Concat(VALUInstructions, InstructionCostPairs, InstructionCostScalars)
}

// InstructionCostKernel selects the instruction_cost kernel and its result as
// instruction_cost.hip does: zero copies is ic_empty for every instruction
// and dependency; a scalar or branch body is ic_<instruction>_k<K> for both
// dependencies; any other body is ic_<instruction>_<dependency>_k<K>. Only
// the bodies of one VALU instruction and ic_empty are in the first code
// object.
func InstructionCostKernel(
	instruction, dependency string, opsPerIter, iterations int,
) (Kernel, error) {
	if !slices.Contains(InstructionCostInstructions(), instruction) {
		return Kernel{}, fmt.Errorf("instruction_cost has no instruction %q; "+
			"supported instructions are %v", instruction, InstructionCostInstructions())
	}
	if !slices.Contains(InstructionCostDependencies, dependency) {
		return Kernel{}, fmt.Errorf("instruction_cost has no dependency %q; "+
			"supported dependencies are %v", dependency, InstructionCostDependencies)
	}
	if !slices.Contains(InstructionCostOpsPerIter, opsPerIter) {
		return Kernel{}, fmt.Errorf("instruction_cost has no kernel for ops_per_iter %d; "+
			"supported values are %v", opsPerIter, InstructionCostOpsPerIter)
	}

	if opsPerIter == 0 {
		// ic_empty stores the untouched X of the v_mov_b64 operands.
		return Kernel{"ic_empty", InstructionCostObject, Reference("v_mov_b64", 0)}, nil
	}

	k := Kernel{
		Name:     fmt.Sprintf("ic_%s_%s_k%d", instruction, dependency, opsPerIter),
		Object:   InstructionCostOtherObject,
		Expected: instructionCostResult(instruction, dependency == "chained", opsPerIter, iterations),
	}
	switch {
	case slices.Contains(VALUInstructions, instruction):
		k.Object = InstructionCostObject
	case slices.Contains(InstructionCostScalars, instruction):
		k.Name = fmt.Sprintf("ic_%s_k%d", instruction, opsPerIter)
	}
	return k, nil
}

// instructionCostResult is the value a kernel of opsPerIter > 0 instructions
// stores per thread after iterations iterations, a port of expected_result in
// instruction_cost.hip.
func instructionCostResult(instruction string, chained bool, opsPerIter, iterations int) uint64 {
	executed := uint64(opsPerIter) * uint64(iterations)
	// A pair body runs its second instruction opsPerIter/2 times an
	// iteration, each time right after the first.
	seconds := uint64(opsPerIter/2) * uint64(iterations)

	switch instruction {
	case "v_mul_lo_u32_v_add_u32":
		// Independent, the stored register is copy 0: one multiply.
		if !chained {
			return Reference("v_mul_lo_u32", 1)
		}
		// One copy is a multiply alone.
		if opsPerIter == 1 {
			return Reference("v_mul_lo_u32", executed)
		}
		// X = X * M then X = X + A, over and over.
		x := u32X0
		for range seconds {
			x = x*u32M + u32A
		}
		return uint64(x)
	case "v_mov_b64_v_fma_f64":
		// X starts as the FMA's, and a move leaves it alone, so only the
		// FMAs count. Independent, copy 0 is a move of the initial X.
		if !chained {
			return Reference("v_fma_f64", 0)
		}
		return Reference("v_fma_f64", seconds)
	case "s_add_u32":
		// The 32-bit counter the body incremented, zero-extended.
		return uint64(uint32(executed))
	case "s_cbranch_scc0", "s_branch":
		// A branch leaves X as in ic_empty.
		return Reference("v_mov_b64", 0)
	}

	// One VALU instruction. Chained, X saw every instruction the loop
	// executed. Independent, the stored register holds one application.
	if chained {
		return Reference(instruction, executed)
	}
	return Reference(instruction, 1)
}

// LoopWaitKernel selects lw_<layout>_<instruction>_k<K> and its result as
// loop_wait.hip does. A layout moves instructions around but every copy is
// X = op(X, ...), so X saw every VALU instruction the loop executed.
func LoopWaitKernel(layout, instruction string, opsPerIter, iterations int) (Kernel, error) {
	if !slices.Contains(LoopWaitLayouts, layout) {
		return Kernel{}, fmt.Errorf("loop_wait has no layout %q; supported layouts are %v",
			layout, LoopWaitLayouts)
	}
	if !slices.Contains(VALUInstructions, instruction) {
		return Kernel{}, fmt.Errorf("loop_wait has no instruction %q; "+
			"supported instructions are %v", instruction, VALUInstructions)
	}
	if !slices.Contains(LoopWaitOpsPerIter, opsPerIter) {
		return Kernel{}, fmt.Errorf("loop_wait has no kernel for ops_per_iter %d; "+
			"supported values are %v", opsPerIter, LoopWaitOpsPerIter)
	}
	return Kernel{
		Name:     fmt.Sprintf("lw_%s_%s_k%d", layout, instruction, opsPerIter),
		Object:   LoopWaitObject,
		Expected: Reference(instruction, uint64(opsPerIter)*uint64(iterations)),
	}, nil
}

// maxThreadsPerBlock is the workgroup size the kernels declare in their
// metadata.
const maxThreadsPerBlock = 1024

// CheckLaunch rejects a launch that gfx942_loop_host.h rejects, or that does
// not fit the dispatch packet: the loop tests at the bottom, so a kernel runs
// at least one iteration; the work-group is at most 1024; the grid is a
// uint32.
func CheckLaunch(iterations, numBlocks, threadsPerBlock int) error {
	if iterations < 1 || iterations > math.MaxInt32 {
		return fmt.Errorf("iterations must be 1 to %d; got %d", math.MaxInt32, iterations)
	}
	if threadsPerBlock < 1 || threadsPerBlock > maxThreadsPerBlock {
		return fmt.Errorf("threads per block must be 1 to %d; got %d",
			maxThreadsPerBlock, threadsPerBlock)
	}
	if numBlocks < 1 || uint64(numBlocks) > math.MaxUint32/uint64(threadsPerBlock) {
		return fmt.Errorf("the grid must have 1 block or more and at most %d threads; "+
			"got %d blocks of %d threads", uint32(math.MaxUint32), numBlocks, threadsPerBlock)
	}
	return nil
}
