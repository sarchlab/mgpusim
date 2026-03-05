# Spec

## What do you want to build

Remove the scratchpad abstraction from the emulator to improve emulation performance, and eliminate remaining heap allocation bottlenecks in the hot path. The scratchpad has been fully removed. The instruction decode cache has been added. Now the focus is on eliminating unnecessary heap allocations in ReadReg, DS read operations, flat address computation, and other hot paths.

## How do you consider the project is success

### Phase 1: Scratchpad Migration ✅
1. ✅ All instruction implementations use ReadOperand/WriteOperand instead of Scratchpad.
2. ✅ All emu Prepare/Commit functions removed (scratchpadpreparer.go deleted).
3. ✅ All tests pass.
4. ✅ Benchmarks show emulation performance improvement (2x for vector instructions, 13.5% end-to-end).

### Phase 2: Scratchpad Removal ✅
5. ✅ The emu `scratchpad.go` file is deleted. Scratchpad type and layout structs moved to timing/wavefront.
6. ✅ The `Scratchpad()` method removed from the `InstEmuState` interface.
7. ✅ The `executeInst` function no longer calls Prepare/Commit.

### Phase 3: Instruction Cache ✅
8. ✅ A decoded instruction cache indexed by PC is implemented in the emu ComputeUnit.

### Phase 4: Heap Allocation Elimination (CURRENT)
9. ReadReg no longer allocates on the heap (currently 64 allocs/op for vector registers).
10. DS read instructions use stack-allocated buffers instead of heap-allocated ones.
11. flatAddr() reads scalar base once outside the lane loop instead of allocating per-lane.
12. ReadOperand padding uses a stack-allocated buffer instead of `make([]byte, 8)`.
13. All tests continue to pass after all changes.
14. Benchmarks show further improvement from allocation elimination.

## Constraints
- Must not break existing GCN3 or CDNA3 emulation functionality.
- Must not break timing simulation.
- GPU programs do not self-modify, so caching decoded instructions by PC is safe.

## Performance Bottleneck Analysis (from Iris)

| Rank | Bottleneck | Status | Est. Savings |
|------|-----------|--------|-------------|
| 1 | Scratchpad clear + Prepare/Commit | ✅ Fixed | ~5,000ns/inst |
| 2 | ReadReg heap allocations | ❌ TODO | ~4,000ns/inst (vector) |
| 3 | DS read allocations | ❌ TODO | ~1,500ns/inst (DS) |
| 4 | Instruction decode cache | ✅ Fixed | ~200-500ns/inst |
| 5 | flatAddr per-lane Operand alloc | ❌ TODO | ~1,200ns/inst (flat) |
| 6 | StorageAccessor.Read alloc | Future | ~500ns/inst (mem ops) |
| 7 | logInst hook check | Future | ~15ns/inst |
