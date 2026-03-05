# Project Specification

## What do you want to build?

Improve the emulation performance of MGPUSim by removing the scratchpad intermediary layer from the emulator's instruction execution pipeline.

Currently, the emulator uses a "scratchpad" — a flat byte buffer that acts as an intermediary between the wavefront's register files and the ALU. For every instruction:
1. `ScratchpadPreparer.Prepare()` copies operand data from the wavefront's registers into the scratchpad
2. `ALU.Run()` reads inputs from and writes outputs to the scratchpad (via typed layout structs like `VOP2Layout`)
3. `ScratchpadPreparer.Commit()` copies results from the scratchpad back into the wavefront's registers

The alternative design is to let the wavefront struct hold emulation register and LDS data directly, and have the ALU read/write register data through the wavefront interface — eliminating the copy-in/copy-out overhead entirely.

## How do you consider the project is successful?

1. **Scratchpad removed**: The `Scratchpad` type, scratchpad layout structs (`SOP1Layout`, `VOP2Layout`, etc.), and `ScratchpadPreparer` are no longer used in the emulation path. The ALU directly interfaces with the wavefront to read/write register data.

2. **Performance improvement**: Emulation benchmarks run faster than before the change. The overhead of clearing ~4KB, copying operands in, and copying results out on every instruction is eliminated.

3. **No regressions**: All 22 CDNA3 benchmarks and all GCN3 benchmarks continue to pass byte-level verification (`-verify` flag). All CI checks pass.

4. **Clean implementation**: Both GCN3 (`amd/emu/`) and CDNA3 (`amd/emu/cdna3/`) ALU implementations are updated. Code passes lint, unit tests, and integration tests.

5. **Benchmarked**: Before/after performance measurements demonstrate the improvement.

## Constraints

- Must maintain backward compatibility with both GCN3 and CDNA3 architectures
- Must pass all CI checks (lint, unit tests, integration tests, emulation tests)
- Must have byte-level correct emulation (not approximate)
- The timing simulation path (`amd/timing/`) may also reference scratchpad — handle or preserve as needed
- Incremental approach: refactor one instruction format at a time, verify, then proceed

## Resources

- Existing emulator code: `amd/emu/` (GCN3) and `amd/emu/cdna3/` (CDNA3)
- Scratchpad: `amd/emu/scratchpad.go`, `amd/emu/scratchpadpreparer.go`
- Wavefront: `amd/emu/wavefront.go`
- 22 CDNA3 benchmarks + GCN3 benchmarks for regression testing
- CI with 30+ checks

## Notes

- The scratchpad provides a clean interface but adds significant overhead. Every instruction requires: (1) clearing 4096 bytes, (2) copying operands from register files to scratchpad, (3) ALU execution, (4) copying results back. Steps 1, 2, and 4 are pure overhead.
- The wavefront already holds `SRegFile`, `VRegFile`, `LDS`, `VCC`, `EXEC`, `SCC`, `PC`, `M0` — all the data the ALU needs.
- The `InstEmuState` interface currently exposes `Scratchpad()` — this interface will need to change or be replaced.
- The timing simulation (`amd/timing/`) may use the scratchpad differently — investigate before removing.
