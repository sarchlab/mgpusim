# MMU Page-Not-Found Investigation

## Problem

MMU page-not-found panics crash benchmarks at larger problem sizes when
running in **timing mode**. The MMU's `finalizePageWalk()` cannot find pages
for the requested virtual addresses and panics. Emulation mode works correctly
at all sizes.

## Root Cause

The panics are caused by **corrupted 64-bit virtual addresses** reaching the
MMU during timing simulation. The upper 32 bits of the address are set to
`0xFFFFFFFF` when they should be `0x00000000`.

In GCN3/CDNA ISA, FLAT memory addresses are 64-bit values composed of two
consecutive VGPRs: `v[N]` (low 32 bits) and `v[N+1]` (high 32 bits). The
corruption manifests as the high VGPR containing `0xFFFFFFFF` instead of
`0x00000000`, producing addresses like `0xFFFFFFFF_0000E000`.

The likely causes are:

1. **VRegOffset conflicts** — When multiple wavefronts share the same SIMD
   register file, incorrect VRegOffset computation could cause one wavefront's
   registers to be read from another wavefront's space.
2. **Carry flag (VCC) propagation errors** — The `v_addc_co_u32` instruction
   uses VCC as carry input. If VCC is not properly stored/restored between
   wavefronts sharing the same CU, the carry propagation produces
   `0xFFFFFFFF` in the high 32 bits (from `-1 + 0 + carry`).
3. **Scratchpad Prepare/Commit mismatch** — The VOP instructions that compute
   the address may write results incorrectly through the timing
   ScratchpadPreparer, corrupting the high 32-bit VGPR.

## Evidence

### Emu mode works; timing mode fails

```bash
# Emu mode — passes at any size:
go run ./amd/samples/vectoradd -width=12289 -height=1 -verify   # PASS

# Timing mode — small sizes pass:
go run ./amd/samples/vectoradd -width=12288 -height=1 -timing   # PASS

# Timing mode — larger sizes panic:
go run ./amd/samples/vectoradd -width=12289 -height=1 -timing   # PANIC
```

### Corrupted addresses in MMU

Instrumented MMU logging shows:

```
MMU PAGE NOT FOUND: PID=1, VAddr=0xffffffff0000e000, DeviceID=1,
  Src=GPU[1].L2TLB.BottomPort
```

The lower 32 bits (`0x0000E000`) are valid; the upper 32 bits are corrupted.

### Vector memory path origin

The corrupted addresses originate from:

```
GPU[1].SA[0].L1VROB[0].BottomPort → L1V Address Translator → TLB → MMU
```

This is the **vector memory path** (FLAT load/store instructions), not
instruction fetch.

### Size threshold

Larger problem sizes trigger the bug because:

- More workgroups are dispatched, increasing the chance of wavefront register
  file conflicts.
- Non-multiple-of-64 grid sizes create partial workgroups with fewer active
  lanes.

## Affected Benchmarks

| Benchmark   | Failing threshold            |
|-------------|------------------------------|
| vectoradd   | width ≥ 32768                |
| relu        | width ≥ 16384                |
| stencil2d   | ≥ 768×768                    |

## Secondary Bug Found: storageAccessor.Write

**File**: `amd/emu/storageaccessor.go`, line 80

When a `Write` spans multiple pages, the code used the original base `vAddr`
instead of `currVAddr` for page table lookups on subsequent pages. This caused
silent data corruption — the data was written to the wrong physical address.

**Fix**: Changed `a.pageTable.Find(pid, vAddr)` to
`a.pageTable.Find(pid, currVAddr)`, matching the correct pattern used in the
`Read` method (line 38). Fixed in this commit.

## Key Files for Future Investigation

| File | Relevance |
|------|-----------|
| `amd/timing/cu/scratchpadpreparer.go` | FLAT address preparation from VGPRs |
| `amd/timing/cu/vectormemoryunit.go` | How FLAT loads generate memory transactions |
| `amd/timing/cu/defaultcoalescer.go` | How addresses from scratchpad become ReadReqs |
| `amd/timing/cu/regfileaccessor.go` | Timing-mode register read/write delegation |
| `amd/timing/cu/wfdispatcher.go` | Wavefront initialization (PC, EXEC, registers) |
| `amd/timing/cu/registerfile.go` | SimpleRegisterFile with shared wave offsets |
| `amd/timing/cu/computeunit.go` | How wavefronts share SIMDs and register file space |
| `amd/samples/runner/timingconfig/builder.go` | MMU builder configuration |

## Status

- **storageAccessor.Write bug**: Fixed.
- **MMU page-not-found root cause**: Identified but **not fully fixed**. The
  address corruption originates in the timing CU's register file or
  scratchpad management. Fixing it requires deeper debugging of the timing
  CU's VRegOffset assignment, VCC carry propagation, and scratchpad
  prepare/commit flow.
