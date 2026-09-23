# GFX942 (MI300/CDNA3) Support Plan for MGPUSim

## Goal

Add GFX942 support to MGPUSim while maintaining GFX803 compatibility using compile-time architecture selection.

## Requirements

- Full ISA + timing model for GFX942
- Compile-time selection via Go build tags
- Both GFX803 and GFX942 must work

---

## Strategy: Architecture Abstraction with Build Tags

### Directory Structure

```
amd/
├── arch/                          # NEW: Architecture abstraction
│   ├── arch.go                    # Interface definitions
│   ├── gcn3/                      # GFX803 (build tag: gcn3 || !cdna3)
│   │   ├── config.go
│   │   ├── format.go
│   │   ├── decodetable.go
│   │   └── hsaco.go
│   └── cdna3/                     # GFX942 (build tag: cdna3)
│       ├── config.go
│       ├── format.go
│       ├── decodetable.go
│       └── hsaco.go
├── insts/                         # Shared instruction infrastructure
├── emu/                           # Emulation (uses arch.Current)
├── timing/cu/                     # Timing model (uses arch.Current)
└── samples/runner/timingconfig/
    ├── r9nano/                    # Existing
    └── mi300/                     # NEW
```

### Build Commands

```bash
go build -tags gcn3 ./...    # GFX803 (default)
go build -tags cdna3 ./...   # GFX942
```

---

## Implementation Phases

### Phase 1: Architecture Abstraction Layer

**Create `/amd/arch/arch.go`:**

```go
package arch

type Architecture interface {
    Name() string                    // "GCN3" or "CDNA3"
    NumVGPRsPerLane() int           // 256 or 512
    NumSGPRs() int                  // 102 or 106
    WavefrontSize() int             // 64
    NumSIMDsPerCU() int             // 4
    VGPRsPerSIMD() int              // 16384 or 32768
    SGPRsPerCU() int                // 3200
    LDSBytesPerCU() int             // 65536
    FormatTable() map[FormatType]*Format
    DecodeTable() *DecodeTableProvider
}

var Current *Config  // Set by init() in gcn3 or cdna3 package
```

**Modify existing code to use `arch.Current`:**

| File | Change |
|------|--------|
| `emu/wavefront.go:37-38` | `SRegFile = make([]byte, 4*arch.Current.Arch.NumSGPRs())` |
| `timing/cu/cubuilder.go:34-36` | Use `arch.Current.Arch.VGPRsPerSIMD()` etc. |
| `insts/disassembler.go` | Use `arch.Current.Arch.FormatTable()` |

### Phase 2: Extract GCN3 to `arch/gcn3/`

Move architecture-specific code with build tag `//go:build gcn3 || !cdna3`:

| Source | Destination |
|--------|-------------|
| `insts/format.go` (FormatTable) | `arch/gcn3/format.go` |
| `insts/decodetable.go` | `arch/gcn3/decodetable.go` |
| `insts/reg.go` (constants) | `arch/gcn3/reg.go` |
| `insts/hsaco.go` (parsing) | `arch/gcn3/hsaco.go` |

### Phase 3: Create CDNA3 Implementation

Create `arch/cdna3/` with build tag `//go:build cdna3`:

- **New instruction formats:** VOP3P (packed math), MFMA (matrix ops)
- **Extended registers:** 512 VGPRs per lane, 106 SGPRs
- **New decode table:** CDNA3-specific opcodes
- **HSACO v3:** Updated kernel descriptor parsing

### Phase 4: CDNA3 Emulation

Create `emu/cdna3/`:

- `alu_mfma.go` - Matrix fused multiply-add instructions
- `alu_vop3p.go` - Packed vector operations

### Phase 5: MI300 Timing Configuration

Create `samples/runner/timingconfig/mi300/builder.go`:

- 220+ CUs
- HBM3 memory model
- Updated cache hierarchy

---

## Key Files to Modify

| File | Lines | Change |
|------|-------|--------|
| `amd/emu/wavefront.go` | 37-39 | Replace hardcoded `4*102`, `4*64*256` with arch config |
| `amd/timing/cu/cubuilder.go` | 32-38 | Replace hardcoded `3200`, `16384` with arch config |
| `amd/insts/disassembler.go` | 61-71 | Use `arch.Current.Arch.FormatTable()` |
| `amd/insts/format.go` | 44-68 | Extract to `arch/gcn3/format.go` |
| `amd/insts/decodetable.go` | all | Extract to `arch/gcn3/decodetable.go` |

---

## Architecture Comparison

| Parameter | GFX803 (GCN3) | GFX942 (CDNA3) |
|-----------|---------------|----------------|
| VGPRs per lane | 256 | 512 |
| SGPRs | 102 | 106 |
| Wavefront size | 64 | 64 |
| SIMDs per CU | 4 | 4 |
| VGPRs per SIMD | 16384 | 32768 |
| Matrix instructions | None | MFMA |
| Typical CU count | 64 (R9 Nano) | 220+ (MI300X) |

---

## Current Architecture-Specific Code Locations

### ISA Instructions (`amd/insts/`)

- `format.go` - 18 instruction format types with GCN3 encodings
- `decodetable.go` - Hardcoded GCN3 instruction opcode definitions
- `reg.go` - 256 VGPRs (V0-V255), 102 SGPRs (S0-S101)
- `hsaco.go` - HSACO header parsing with GCN3-specific bit positions
- `disassembler.go` - GCN3-specific instruction decoding logic

### Emulator (`amd/emu/`)

- `wavefront.go` - 64-lane wavefronts, register file sizes hardcoded
- `alu*.go` - Instruction execution keyed to GCN3 opcodes
- `scratchpad.go` - Per-format scratchpad layouts

### Timing Model (`amd/timing/cu/`)

- `computeunit.go` - 4 SIMDs, hardcoded pool sizes
- `cubuilder.go` - Default parameters: 16384 VGPRs per SIMD, 3200 SGPRs

### GPU Configuration (`amd/samples/runner/timingconfig/r9nano/`)

- R9 Nano-specific GPU configuration (64 CUs, 16 shader arrays)

---

## Testing Strategy

1. **Phase 1 validation:** All existing GCN3 tests must pass unchanged
2. **Build verification:** Both `-tags gcn3` and `-tags cdna3` compile
3. **CDNA3 tests:** New test suite for GFX942-specific instructions
4. **Benchmark validation:** Run vectoradd and other benchmarks on both

---

## Key Differences: GFX803 vs GFX942

### Instruction Encoding

GFX942 introduces new instruction formats not present in GFX803:

- **VOP3P** - Packed math operations (two FP16 ops per instruction)
- **MFMA** - Matrix Fused Multiply-Add for AI/ML workloads

### Register File

- GFX942 doubles VGPR capacity (512 vs 256 per lane)
- Slightly more SGPRs (106 vs 102)

### Memory Hierarchy

- MI300 uses HBM3 (vs HBM2)
- Larger L2 cache
- Different memory interleaving

### Compute Resources

- MI300X has 220+ CUs (vs 64 for R9 Nano)
- Higher memory bandwidth
- Matrix acceleration units
