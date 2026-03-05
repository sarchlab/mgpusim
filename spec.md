# Spec

## What do you want to build

Remove the scratchpad abstraction from the emulator to improve emulation performance. The scratchpad prepares operand data for ALU units and collects results, providing a clean interface but adding performance overhead. The alternative is to let the wavefront struct hold emulation register and LDS data, with the emulator ALU directly interfacing with the wavefront to read/write register data via ReadOperand/WriteOperand.

## How do you consider the project is success

1. All vector instruction implementations (VOP1, VOP2, VOP3A, VOP3B, VOPC) in both GCN3 and CDNA3 use ReadOperand/WriteOperand instead of Scratchpad.
2. All scalar instruction implementations (SOP1, SOP2, SOPP, SOPK, SOPC, SMEM) in both GCN3 and CDNA3 use ReadOperand/WriteOperand instead of Scratchpad.
3. Timing vector/scalar Prepare/Commit functions are no-ops.
4. All tests pass. CI checks pass.
5. Benchmarks show emulation performance improvement.
6. Eventually, flat and DS instructions should also be migrated.

## Constraints
- Must not break existing GCN3 or CDNA3 emulation functionality.
- Follow the established ReadOperand/WriteOperand migration pattern.

## Notes
- Scalar instructions (M2) migration is complete and merged.
- Vector instructions (M3) are ~95% complete — only CDNA3 vop3a.go remains.
