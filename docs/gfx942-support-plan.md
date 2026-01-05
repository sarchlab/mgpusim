# GFX942 (MI300/CDNA3) Support Plan for MGPUSim

## Goal

Add GFX942 support to MGPUSim while maintaining GFX803 compatibility using **runtime architecture selection**.

## Requirements

- Full ISA + timing model for GFX942
- Runtime selection based on loaded HSACO
- Both GFX803 and GFX942 must work
- Emulation fails gracefully if architecture mismatch

---

## Strategy: Runtime Architecture Selection

### Directory Structure

```
amd/
├── arch/                          # Architecture configuration
│   ├── arch.go                    # Config type definition
│   ├── gcn3.go                    # GFX803 constants
│   └── cdna3.go                   # GFX942 constants
├── emu/                           # Emulation
│   ├── emu.go                     # ALU interface, shared types
│   ├── gcn3/                      # GCN3 ALU implementation
│   │   ├── alu.go                 # Entry point + dispatcher
│   │   ├── vop1.go
│   │   ├── vop2.go
│   │   ├── vop3a.go
│   │   ├── vop3b.go
│   │   ├── vopc.go
│   │   ├── sop1.go
│   │   ├── sop2.go
│   │   ├── sopc.go
│   │   ├── sopk.go
│   │   ├── flat.go
│   │   └── ds.go
│   └── cdna3/                     # CDNA3 ALU implementation
│       ├── alu.go                 # Entry point + dispatcher
│       ├── vop1.go
│       ├── vop2.go
│       ├── vop3a.go               # Extended opcodes
│       ├── vop3b.go
│       ├── vop3p.go               # NEW: Packed math
│       ├── mfma.go                # NEW: Matrix FMA
│       ├── vopc.go
│       ├── sop1.go
│       ├── sop2.go
│       ├── sopc.go
│       ├── sopk.go
│       ├── flat.go
│       └── ds.go
├── insts/                         # Instruction infrastructure (shared)
├── timing/cu/                     # Timing model
└── samples/runner/timingconfig/
    ├── r9nano/                    # Existing
    └── mi300/                     # NEW
```

### Runtime Selection

The builder code selects the appropriate emulator based on the HSACO code object version:

```go
func selectEmulator(co *insts.KernelCodeObject) (emu.ALU, error) {
    switch co.Version {
    case insts.CodeObjectV2, insts.CodeObjectV3:
        return gcn3.NewALU(storageAccessor), nil
    case insts.CodeObjectV5:
        return cdna3.NewALU(storageAccessor), nil
    default:
        return nil, fmt.Errorf("unsupported code object version: %v", co.Version)
    }
}
```

---

## Implementation Phases

### Phase 1: ALU Interface Abstraction

**Create `amd/emu/emu.go`:**

```go
package emu

// ALU defines the interface for architecture-specific ALU implementations
type ALU interface {
    Run(state InstEmuState)
    SetLDS(lds []byte)
    LDS() []byte
    ArchName() string  // Returns "GCN3" or "CDNA3"
}
```

**Modify existing code to use the interface:**

| File | Change |
|------|--------|
| `emu/computeunit.go` | Accept ALU interface instead of concrete type |
| `timing/cu/*.go` | Accept ALU interface |

### Phase 2: Extract GCN3 to `emu/gcn3/`

Move architecture-specific ALU code:

| Source | Destination |
|--------|-------------|
| `emu/alu.go` (impl) | `emu/gcn3/alu.go` |
| `emu/aluvop1.go` | `emu/gcn3/vop1.go` |
| `emu/aluvop2.go` | `emu/gcn3/vop2.go` |
| `emu/aluvop3a.go` | `emu/gcn3/vop3a.go` |
| `emu/aluvop3b.go` | `emu/gcn3/vop3b.go` |
| `emu/aluvopc.go` | `emu/gcn3/vopc.go` |
| `emu/alusop1.go` | `emu/gcn3/sop1.go` |
| `emu/alusop2.go` | `emu/gcn3/sop2.go` |
| `emu/alusopc.go` | `emu/gcn3/sopc.go` |
| `emu/alusopk.go` | `emu/gcn3/sopk.go` |
| `emu/alu_flat.go` | `emu/gcn3/flat.go` |
| `emu/aluds.go` | `emu/gcn3/ds.go` |

### Phase 3: Create CDNA3 Implementation

Create `emu/cdna3/` package:

- Copy GCN3 implementations as base
- Add missing opcodes (v_bfe_u32, etc.)
- Add CDNA3-specific formats:
  - `vop3p.go` - Packed vector operations
  - `mfma.go` - Matrix fused multiply-add instructions

### Phase 4: Architecture Configuration

Create `amd/arch/` for shared constants:

```go
// arch/arch.go
package arch

type Config struct {
    Name            string
    NumVGPRsPerLane int
    NumSGPRs        int
    WavefrontSize   int
    NumSIMDsPerCU   int
    VGPRsPerSIMD    int
    SGPRsPerCU      int
    LDSBytesPerCU   int
}

// arch/gcn3.go
var GCN3 = &Config{
    Name:            "GCN3",
    NumVGPRsPerLane: 256,
    NumSGPRs:        102,
    WavefrontSize:   64,
    NumSIMDsPerCU:   4,
    VGPRsPerSIMD:    16384,
    SGPRsPerCU:      3200,
    LDSBytesPerCU:   65536,
}

// arch/cdna3.go
var CDNA3 = &Config{
    Name:            "CDNA3",
    NumVGPRsPerLane: 512,
    NumSGPRs:        106,
    WavefrontSize:   64,
    NumSIMDsPerCU:   4,
    VGPRsPerSIMD:    32768,
    SGPRsPerCU:      3200,
    LDSBytesPerCU:   65536,
}
```

### Phase 5: Builder Integration

Update builder/driver code to:
1. Detect HSACO architecture from code object version
2. Select appropriate ALU implementation
3. Validate architecture compatibility
4. Fail with clear error if mismatch

### Phase 6: MI300 Timing Configuration

Create `samples/runner/timingconfig/mi300/builder.go`:

- 220+ CUs
- HBM3 memory model
- Updated cache hierarchy

---

## Key Files to Modify

| File | Lines | Change |
|------|-------|--------|
| `amd/emu/emu.go` | NEW | ALU interface definition |
| `amd/emu/gcn3/alu.go` | NEW | GCN3 ALU entry point |
| `amd/emu/cdna3/alu.go` | NEW | CDNA3 ALU entry point |
| `amd/emu/wavefront.go` | 37-39 | Use `arch.Config` for register sizes |
| `amd/timing/cu/cubuilder.go` | 32-38 | Use `arch.Config` for pool sizes |
| `amd/driver/` | various | Add emulator selection logic |

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
- `decodetable.go` - Instruction opcode definitions (shared, includes CDNA3 markers)
- `reg.go` - Register definitions
- `hsaco.go` - HSACO header parsing (V2/V3 and V5 support)
- `disassembler.go` - Instruction decoding logic

### Emulator (`amd/emu/`)

- `wavefront.go` - 64-lane wavefronts, register file sizes
- `alu*.go` - Instruction execution (to be split by architecture)
- `scratchpad.go` - Per-format scratchpad layouts

### Timing Model (`amd/timing/cu/`)

- `computeunit.go` - 4 SIMDs
- `cubuilder.go` - Default parameters for pool sizes

### GPU Configuration (`amd/samples/runner/timingconfig/r9nano/`)

- R9 Nano-specific GPU configuration (64 CUs, 16 shader arrays)

---

## Testing Strategy

1. **Phase 1 validation:** All existing GCN3 tests must pass unchanged
2. **Runtime selection:** Verify builder selects correct emulator based on HSACO
3. **Error handling:** Test architecture mismatch produces clear error
4. **CDNA3 tests:** New test suite for GFX942-specific instructions
5. **Benchmark validation:** Run vectoradd and other benchmarks on both architectures

---

## Key Differences: GFX803 vs GFX942

### Instruction Encoding

GFX942 introduces new instruction formats not present in GFX803:

- **VOP3P** - Packed math operations (two FP16 ops per instruction)
- **MFMA** - Matrix Fused Multiply-Add for AI/ML workloads

### Code Object Format

- **V2/V3** (GCN3): 256-byte header per kernel in `.text` section
- **V5** (GFX9+/CDNA3): 64-byte kernel descriptor in `.rodata` section

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
