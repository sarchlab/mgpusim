# Phase 2 Research: cuBLAS/hipBLAS Benchmarking in MGPUSim

## 1. Introduction & Motivation

BLAS (Basic Linear Algebra Subprograms) libraries are the computational
backbone of modern ML workloads. NVIDIA's cuBLAS and AMD's hipBLAS/rocBLAS
provide highly optimized GPU implementations of matrix operations — most
critically GEMM (General Matrix Multiply) — that consume 55–75% of GPU time
in transformer training and 40–60% in inference serving.

Understanding how these vendor-optimized BLAS routines map to GPU hardware
is essential for GPU architecture simulation research. However, cuBLAS and
rocBLAS are closed-source binaries that cannot be directly disassembled or
simulated. This document analyzes the technical feasibility of benchmarking
BLAS-equivalent operations in MGPUSim and recommends concrete approaches
for Phase 2 implementation.

### 1.1 Current Coverage in MGPUSim

MGPUSim already includes several BLAS-adjacent benchmarks:

| Benchmark | BLAS Equivalent | Location | Status |
|-----------|----------------|----------|--------|
| matrixmultiplication | SGEMM (naive) | amd/samples/matrixmultiplication | Working (timing + emulation) |
| conv2d | Im2Col + GEMM | amd/samples/conv2d | Emulation only (timing crashes) |
| ml_kernels/gemm | Tiled GEMM (FP32) | workloads/ml_kernels/gemm | HIP source, not in sim |
| im2col | Im2Col transform | amd/samples/im2col | Working |

The existing `matrixmultiplication` benchmark achieves 22.36% MAPE against
MI300a hardware data (see accuracy_analysis.md Section 3.6). This provides
a baseline for evaluating more sophisticated GEMM implementations.

### 1.2 Scope

This analysis covers:
- GEMM operations: SGEMM, DGEMM, HGEMM, and batched variants
- TRSM/TRSV: Triangular solve operations
- SYMM/SYRK: Symmetric matrix operations
- Mapping these to GPU ISA instructions on CDNA3
- Feasibility of simulation in MGPUSim without modifying the simulator

## 2. hipBLAS/rocBLAS Architecture Overview

### 2.1 Software Stack

```
Application (PyTorch, TensorFlow, etc.)
    │
    ▼
hipBLAS API (device-agnostic interface)
    │
    ▼
rocBLAS (AMD's optimized BLAS backend)
    │
    ├── Tensile (code generation framework for GEMM kernels)
    │   ├── Problem descriptor → kernel selection
    │   ├── YAML kernel configs → assembly kernels
    │   └── Auto-tuned per-GPU-architecture lookup tables
    │
    └── Pre-compiled kernel libraries (.co / .hsaco files)
        └── CDNA3 (gfx942) optimized kernels
```

rocBLAS delegates GEMM kernel generation to **Tensile**, AMD's open-source
code generation framework. Tensile generates GCN/CDNA assembly kernels
optimized for specific problem shapes and GPU architectures.

### 2.2 Tensile Kernel Generation

Tensile (https://github.com/ROCm/Tensile) is the key differentiator.
Unlike NVIDIA's closed cuBLAS, AMD's Tensile is partially open:

- **Kernel logic files** (.yaml) define tiling parameters, vector widths,
  loop unrolling, and memory access patterns
- **Generated assembly** is CDNA ISA (gfx940/gfx942 for MI300)
- **Lookup tables** map (M, N, K, dtype, transA, transB) → kernel variant

However, the generated assembly is highly specialized:
- Register-level optimizations specific to CDNA3 matrix core units
- Hardcoded tile sizes (e.g., 256×128×32 for large FP32 GEMM)
- Software pipelining with explicit LDS (Local Data Share) management
- Use of MFMA (Matrix Fused Multiply-Add) instructions on CDNA3

### 2.3 Key hipBLAS APIs

| API | Operation | Complexity | Use in ML |
|-----|-----------|------------|-----------|
| hipblasSgemm | C = α·A·B + β·C (FP32) | O(MNK) | Training (FP32 mode) |
| hipblasHgemm | C = α·A·B + β·C (FP16) | O(MNK) | Inference, mixed precision |
| hipblasGemmEx | Extended precision GEMM | O(MNK) | BF16, INT8, mixed types |
| hipblasSgemmBatched | Batched FP32 GEMM | O(B·MNK) | Multi-head attention |
| hipblasSgemmStridedBatched | Strided batched GEMM | O(B·MNK) | Attention (contiguous) |
| hipblasStrsm | Triangular solve | O(MN²) | Cholesky, LU decomposition |
| hipblasSsyrk | Symmetric rank-k update | O(MN²) | Covariance computation |

For transformer workloads, **GEMM** (Sgemm, Hgemm, GemmEx) accounts for
~95% of all BLAS calls. Batched GEMM handles multi-head attention score
computation and value projection.

## 3. GEMM Implementation Analysis

### 3.1 Tiling Hierarchy

Production GEMM implementations use a 3-level tiling strategy:

```
Global Memory (HBM)
    │
    ▼  ── Thread Block Tile (e.g., 256×128) ──
    │     Loaded cooperatively by all threads in a block
    │     Stored in LDS (Local Data Share / Shared Memory)
    │
    ▼  ── Warp Tile (e.g., 64×32) ──
    │     Subset of block tile processed by one warp (64 threads on CDNA3)
    │     May use MFMA instructions for matrix core acceleration
    │
    ▼  ── Thread Tile (e.g., 8×8) ──
          Computed by a single thread using register-resident accumulators
          Uses V_FMA_F32 or MFMA instructions
```

### 3.2 Memory Access Patterns

A well-optimized GEMM kernel exhibits these memory behaviors:

**Global → LDS (Shared Memory) loads:**
- Coalesced loads: 128-byte transactions (32 threads × 4 bytes each)
- Double-buffering: Prefetch next tile while computing current tile
- On CDNA3: Uses FLAT_LOAD_DWORDX4 for 16-byte vector loads

**LDS → Register loads:**
- Bank-conflict-free access patterns (64 banks on CDNA3 LDS)
- DS_READ_B128 for 16-byte loads from LDS
- Interleaved across warp lanes to maximize throughput

**Register computation:**
- CDNA3 MFMA instructions: V_MFMA_F32_16X16X4_F32, V_MFMA_F32_32X32X2_F32
- Alternative: V_FMA_F32 for non-matrix-core GEMM
- Accumulation in VGPR (Vector General Purpose Registers)

**Register → Global stores:**
- FLAT_STORE_DWORDX4 for coalesced writes
- Optional epilogue fusion: bias add, activation, scaling

### 3.3 CDNA3 Matrix Core Instructions

The MI300a's CDNA3 architecture provides Matrix Fused Multiply-Add (MFMA)
instructions that accelerate GEMM:

| Instruction | Input Type | Output Type | Matrix Shape | FLOPs/cycle |
|---|---|---|---|---|
| V_MFMA_F32_32X32X2_F32 | FP32 | FP32 | 32×32×2 | 4096 |
| V_MFMA_F32_16X16X4_F32 | FP32 | FP32 | 16×16×4 | 2048 |
| V_MFMA_F32_32X32X8_F16 | FP16 | FP32 | 32×32×8 | 16384 |
| V_MFMA_F32_16X16X16_F16 | FP16 | FP32 | 16×16×16 | 8192 |
| V_MFMA_F32_32X32X8_BF16 | BF16 | FP32 | 32×32×8 | 16384 |
| V_MFMA_I32_32X32X16_I8 | INT8 | INT32 | 32×32×16 | 32768 |

These instructions operate on data distributed across a wavefront (64 threads).
Each thread contributes input elements and receives output elements of the
matrix product. The key simulation challenge is modeling the internal pipeline
latency and data routing of these instructions.

### 3.4 Representative GEMM Shapes for ML

From the RESEARCH.md analysis, critical GEMM shapes include:

| Model | Operation | M | N | K | Compute (GFLOPS) |
|-------|-----------|---|---|---|-------------------|
| GPT-2-Small | QKV projection | 768 | 2304 | 768 | 2.72 |
| GPT-2-Small | FFN up | 768 | 3072 | 768 | 3.62 |
| Llama-2-7B | QKV projection | 4096 | 4096 | 4096 | 137.4 |
| Llama-2-7B | Gate proj | 4096 | 11008 | 4096 | 369.1 |
| Llama-2-70B | QKV projection | 8192 | 8192 | 8192 | 1099.5 |
| Llama-2-70B | Gate proj | 8192 | 28672 | 8192 | 3848.3 |

The challenge: Llama-2-70B GEMM shapes require ~4 GB of data for a single
operation — well beyond what MGPUSim can currently simulate in reasonable time.

## 4. Mapping to GPU ISA: Instruction-Level Analysis

### 4.1 Naive GEMM (Current MGPUSim matrixmultiplication)

The existing `amd/samples/matrixmultiplication` uses a simple tiled approach.
When compiled for CDNA3, it generates:

```
; Inner loop (K-dimension)
GLOBAL_LOAD_DWORD    v[a], s[base_A], offset_A    ; Load A[i][k]
GLOBAL_LOAD_DWORD    v[b], s[base_B], offset_B    ; Load B[k][j]
S_WAITCNT            vmcnt(0)                      ; Wait for loads
V_FMA_F32            v[acc], v[a], v[b], v[acc]    ; acc += a * b
; (loop overhead: S_ADD_U32, S_CMP, S_CBRANCH)
```

This approach is inefficient because:
1. Each iteration does 1 FMA with 2 global memory loads (arithmetic intensity ~0.25 FLOP/byte)
2. No shared memory tiling — redundant global loads across threads
3. No data reuse within registers

### 4.2 Tiled GEMM (ml_kernels/gemm target)

A properly tiled GEMM generates:

```
; Tile load phase (cooperative across block)
DS_WRITE_B128        lds_A, v[a_vec]               ; Store A tile to LDS
DS_WRITE_B128        lds_B, v[b_vec]               ; Store B tile to LDS
S_BARRIER                                           ; Sync threads

; Compute phase (per-thread)
DS_READ_B128         v[a_lds], lds_A_offset        ; Load from LDS
DS_READ_B128         v[b_lds], lds_B_offset        ; Load from LDS
V_FMA_F32            v[c0], v[a0], v[b0], v[c0]    ; Thread-tile compute
V_FMA_F32            v[c1], v[a0], v[b1], v[c1]    ; (unrolled)
V_FMA_F32            v[c2], v[a1], v[b0], v[c2]
V_FMA_F32            v[c3], v[a1], v[b1], v[c3]
; (many more FMA instructions for full thread tile)
```

Key differences:
- Arithmetic intensity rises to ~K/2 FLOP/byte (compute-bound for K > 20)
- LDS provides ~10× bandwidth vs global memory
- Thread-tile allows register-level data reuse

### 4.3 MFMA GEMM (Tensile-style)

The highest performance GEMM uses MFMA matrix core instructions:

```
; Load A and B tiles into registers (from LDS, pre-loaded from global)
DS_READ_B128         v[a_frag:a_frag+3], lds_a_off
DS_READ_B128         v[b_frag:b_frag+3], lds_b_off

; Matrix multiply accumulate
V_MFMA_F32_32X32X2_F32  a[acc:acc+15], v[a_frag], v[b_frag], a[acc:acc+15]
; Single instruction computes 32×32×2 = 2048 FMAs
; Uses accumulation registers (AGPRs) on CDNA3
```

This achieves peak hardware throughput but requires:
- Specific data layout in registers matching MFMA input format
- AGPR (Accumulation GPU Registers) support in the simulator
- Correct modeling of MFMA instruction latency and throughput

## 5. MGPUSim Simulation Feasibility

### 5.1 What Works Today

MGPUSim can already simulate GEMM-like workloads:

| Feature | Status | Notes |
|---------|--------|-------|
| FP32 V_FMA_F32 | ✅ Working | Core of current matrixmultiplication benchmark |
| Global memory loads | ✅ Working | FLAT_LOAD_DWORD, GLOBAL_LOAD_DWORD |
| LDS operations | ⚠️ Partial | DS_READ/DS_WRITE work; some complex modes untested |
| Barriers (S_BARRIER) | ✅ Working | Used in simpleconvolution, conv2d |
| Multiple CUs | ✅ Working | MI300a config supports full CU count |
| Large matrix sizes | ⚠️ Slow | Simulation time grows as O(N³) for GEMM |

### 5.2 What Requires Extension

| Feature | Status | Effort | Notes |
|---------|--------|--------|-------|
| MFMA instructions | ❌ Missing | High | V_MFMA_F32_* not implemented in CDNA3 emulator |
| AGPR (accumulation registers) | ❌ Missing | Medium | MFMA outputs go to AGPRs, not VGPRs |
| FP16 compute | ⚠️ Partial | Medium | V_PK_FMA_F16 exists; V_MFMA_F32_*_F16 missing |
| BF16 compute | ❌ Missing | Medium | No BF16 instruction support |
| INT8 compute | ❌ Missing | Medium | No INT8 MFMA support |
| DS_READ_B128 | ⚠️ Untested | Low | 128-bit LDS reads may not be fully tested |
| Software pipelining | ✅ N/A | — | Handled by compiler; sim just executes instructions |

### 5.3 Simulation Without MFMA

The critical insight: **we do NOT need MFMA instructions to benchmark GEMM
behavior for architecture research.** The key parameters we care about are:

1. **Memory access patterns** (cache hit rates, bandwidth utilization)
2. **Compute-to-memory ratio** (arithmetic intensity)
3. **Occupancy** (register/LDS pressure)
4. **Scaling behavior** (how performance changes with problem size)

All of these can be captured with V_FMA_F32-based tiled GEMM, which MGPUSim
already supports. The absolute throughput numbers will differ from MFMA-based
production kernels, but the **relative accuracy** (sim vs real scaling trends)
is what matters for architecture studies.

## 6. Recommended Approach for Phase 2

### 6.1 Strategy: Open-Source Tiled GEMM (Not rocBLAS)

We recommend implementing benchmarkable GEMM kernels directly in HIP
source code, **not** attempting to simulate rocBLAS/cuBLAS binaries.

**Rationale:**
1. rocBLAS kernels use MFMA instructions that MGPUSim doesn't support
2. Tensile-generated assembly is too specialized to port
3. Open-source tiled GEMM gives full visibility into behavior
4. The ml_kernels/gemm implementation already exists — it needs to be
   compiled for CDNA3 and integrated into MGPUSim's sample framework

### 6.2 Implementation Plan

**Step 1: Port ml_kernels/gemm to MGPUSim sample** (1–2 cycles)
- Copy the tiled GEMM kernel from `workloads/ml_kernels/gemm/` into
  `amd/samples/gemm/` following the existing benchmark pattern
- Parameterize: TILE_M, TILE_N, TILE_K, M, N, K as command-line args
- Add to benchmark.yml for automated CI runs

**Step 2: Validate against matrixmultiplication** (1 cycle)
- Run both naive and tiled GEMM at overlapping sizes
- Compare sim vs real accuracy — tiled should be closer to hardware

**Step 3: Add batched GEMM** (1–2 cycles)
- Implement strided batched GEMM for multi-head attention
- Parameters: B (batch), M, N, K, strideA, strideB, strideC
- Validate against hardware measurement data (kernel_timings_20260317-075319-odyssey.csv) if batched data becomes available

**Step 4: Sweep ML-relevant shapes** (1 cycle)
- Run GPT-2-Small shapes (768–3072 dimensions) — feasible in sim time
- Run Llama-2-7B shapes (4096+ dimensions) — may require long sim runs
- Compare scaling trends with hardware measurements

### 6.3 TRSM/TRSV Considerations

Triangular solves are less critical for ML workloads (<1% of GPU time in
transformers) but important for scientific computing. Implementation approach:

- Port a simple block-based TRSM using shared memory
- Key instruction pattern: V_FMA_F32 with dependency chains
- Simulation feasibility is high (same instructions as GEMM, smaller matrices)
- Lower priority than GEMM variants

## 7. Key Technical Challenges

### 7.1 Simulation Time

The O(N³) compute complexity of GEMM makes large problem sizes impractical:
- GPT-2-Small shapes (768×2304×768): ~5 minutes simulation → feasible
- Llama-2-7B shapes (4096×11008×4096): hours → impractical for sweeps
- Solution: Focus on smaller representative shapes that demonstrate
  scaling behavior; extrapolate for larger sizes

### 7.2 LDS Bank Conflicts

Tiled GEMM performance depends heavily on LDS bank conflict avoidance.
MGPUSim's LDS model must accurately represent:
- 64-bank LDS structure on CDNA3 (32 banks per sub-partition)
- Bank conflict detection and serialization
- Padding strategies to avoid conflicts

Current status: LDS banking is modeled in MGPUSim but accuracy for
conflict-heavy access patterns is untested.

### 7.3 Memory Coalescing Accuracy

GEMM kernels rely on coalesced memory access for bandwidth efficiency.
The simulator must accurately model:
- 128-byte cache line boundaries
- Warp-level coalescing rules for CDNA3
- L1/L2 cache behavior under strided access patterns

The existing matrixmultiplication benchmark at 22% MAPE suggests the
memory subsystem model has room for improvement.

### 7.4 Conv2d as GEMM

The conv2d benchmark implements convolution via Im2Col + GEMM, which is
the standard cuDNN/MIOpen approach. The timing crash in conv2d (MMU page
walk panic at Repeat kernel) may actually reveal an underlying issue
relevant to GEMM benchmarking — if the MMU cannot handle the memory
allocation pattern of Im2Col's unfolded matrix, similar large GEMM
operations may also trigger this bug.

## 8. References

1. AMD rocBLAS documentation: https://rocm.docs.amd.com/projects/rocBLAS/
2. AMD Tensile: https://github.com/ROCm/Tensile
3. AMD Composable Kernel: https://github.com/ROCm/composable_kernel
4. NVIDIA CUTLASS: https://github.com/NVIDIA/cutlass
5. CDNA3 ISA Reference: docs/amd-instinct-mi300-cdna3-instruction-set-architecture.pdf
6. MGPUSim matrixmultiplication accuracy: accuracy_analysis.md Section 3.6
7. ML kernel shapes: workloads/ml_kernels/RESEARCH.md Section 2
8. Goto & Van de Geijn, "Anatomy of High-Performance Matrix Multiplication"
   ACM TOMS, 2008.
9. Kwasniewski et al., "Red-Blue Pebbling Revisited: Near-Optimal Parallel
   Matrix-Matrix Multiplication" SC'19.
