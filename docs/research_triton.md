# Phase 2 Research: Triton Kernel Compilation and MGPUSim Integration

## 1. Introduction & Motivation

OpenAI Triton is a Python-based DSL for writing GPU kernels that has
become a critical component of modern ML frameworks. PyTorch 2.0's
`torch.compile` uses Triton as its default backend for generating fused
GPU kernels, and libraries like FlashAttention-2, xFormers, and vLLM
ship Triton kernels for attention, normalization, and activation ops.

Understanding how Triton compiles Python-level kernel code into GPU ISA
— particularly for AMD CDNA3 (MI300 series) — is essential for
evaluating whether Triton-generated kernels can serve as simulation
workloads in MGPUSim.

### 1.1 Why Triton Matters for MGPUSim

| Property              | Hand-written HIP | rocBLAS/hipBLAS    | Triton             |
|-----------------------|-------------------|--------------------|--------------------|
| Source availability   | Fully open        | Closed binary      | Open (Python DSL)  |
| ISA output visibility | Via compiler       | Opaque             | Extractable        |
| Kernel specialization | Manual            | Auto-tuned binary  | Auto-tuned codegen |
| AMD CDNA3 support     | Full              | Full               | Experimental       |
| Simulation potential  | High              | None               | Medium             |

Unlike rocBLAS (analyzed in `docs/research_cublas_hipblas.md`), Triton's
pipeline is fully open-source and its ISA outputs can be inspected.
Unlike hand-written HIP kernels (`workloads/ml_kernels/`), Triton
applies automatic optimizations closer to production behavior.

---

## 2. Triton Architecture Overview

### 2.1 Compilation Pipeline

```
Python DSL (@triton.jit decorated function)
    │
    ▼
Triton IR (MLIR-based, block-level tensor operations)
    │  — type inference, block pointer analysis
    ▼
Triton GPU IR (TTGIR — GPU-specific layout annotations)
    │  — tiling decisions, shared memory allocation, warp scheduling
    ▼
LLVM IR (standard LLVM intermediate representation)
    │  — target-specific lowering
    ▼
AMDGCN Assembly (AMD targets) / PTX (NVIDIA targets)
    │
    ▼
Machine code: HSACO (AMD) / cubin (NVIDIA)
```

Each stage is accessible via environment variables and the Triton
compiler API. Triton operates on **block-level tensor operations**
rather than scalar per-thread code, enabling higher-level optimizations.

### 2.2 Block-Level Programming Model

| Concept          | HIP/CUDA                     | Triton                        |
|------------------|-----------------------------|-------------------------------|
| Unit of work     | Single thread               | Block of threads (program)    |
| Memory access    | Scalar load/store per thread | Tensor load/store per block   |
| Synchronization  | Explicit `__syncthreads()`  | Implicit (compiler-managed)   |
| Shared memory    | Manual `__shared__`         | Auto-allocated by compiler    |
| Tiling           | Manual loop + tile loading  | `tl.load` with block pointers |
| Reduction        | Manual warp shuffle + shmem | `tl.reduce` / `tl.sum`        |

### 2.3 AMD ROCm Backend

Triton's AMD backend targets AMDGPU via LLVM. For CDNA3 (gfx942), it
generates V_FMA_F32/V_MFMA_F32_* (compute), GLOBAL_LOAD/STORE_DWORDX4
(memory), DS_READ/WRITE_B128 (LDS), and S_BARRIER/S_WAITCNT (control).
The AMD backend (`triton-rocm`) is less mature than the NVIDIA backend,
with upstream integration ongoing as of 2024.

---

## 3. Key Triton Operations and ISA Mapping

### 3.1 Memory Operations: tl.load / tl.store

```python
@triton.jit
def vector_add(x_ptr, y_ptr, out_ptr, N, BLOCK_SIZE: tl.constexpr):
    pid = tl.program_id(0)
    offsets = pid * BLOCK_SIZE + tl.arange(0, BLOCK_SIZE)
    mask = offsets < N
    x = tl.load(x_ptr + offsets, mask=mask)
    y = tl.load(y_ptr + offsets, mask=mask)
    tl.store(out_ptr + offsets, x + y, mask=mask)
```

**CDNA3 ISA output:**
```asm
GLOBAL_LOAD_DWORDX4  v[0:3], v[addr], s[desc], offset:0    ; 128-bit load
GLOBAL_LOAD_DWORDX4  v[4:7], v[addr], s[desc], offset:16   ; next 128 bits
S_WAITCNT            vmcnt(0)
GLOBAL_STORE_DWORDX4 v[addr], v[result:result+3], s[desc]
```

A `tl.load` of BLOCK_SIZE=256 FP32 elements generates ~16
GLOBAL_LOAD_DWORDX4 instructions (coalesced 128-bit loads).

### 3.2 Matrix Multiply: tl.dot

`tl.dot` maps directly to hardware matrix cores when available:

```python
@triton.jit
def matmul_kernel(A, B, C, M, N, K,
                  BLOCK_M: tl.constexpr, BLOCK_N: tl.constexpr,
                  BLOCK_K: tl.constexpr):
    pid_m, pid_n = tl.program_id(0), tl.program_id(1)
    acc = tl.zeros((BLOCK_M, BLOCK_N), dtype=tl.float32)
    for k in range(0, K, BLOCK_K):
        a = tl.load(A + ...)   # [BLOCK_M, BLOCK_K] tile
        b = tl.load(B + ...)   # [BLOCK_K, BLOCK_N] tile
        acc += tl.dot(a, b)    # Matrix multiply-accumulate
    tl.store(C + ..., acc)
```

| Triton Operation    | CDNA3 Instruction           | Condition               |
|--------------------|-----------------------------|--------------------------|
| `tl.dot` (FP32)    | V_MFMA_F32_32X32X2_F32     | Matrix cores available   |
| `tl.dot` (FP32)    | V_FMA_F32 (loop)            | Fallback, no matrix cores|
| `tl.dot` (FP16)    | V_MFMA_F32_32X32X8_F16     | FP16 matrix cores        |
| `tl.dot` (BF16)    | V_MFMA_F32_32X32X8_BF16    | BF16 matrix cores        |

Without matrix cores, `tl.dot` falls back to V_FMA_F32 loops — similar
to the tiled GEMM in `workloads/ml_kernels/gemm/hip/gemm.cpp`.

### 3.3 Reduction Operations: tl.reduce / tl.max / tl.sum

Reductions generate warp-level shuffle + shared memory cross-warp patterns:

```asm
; Warp-level reduction via DPP/swizzle
V_MAX_F32            v[max], v[max], v[val]
DS_SWIZZLE_B32       v[tmp], v[max], offset:swizzle(SWAP,1)
V_MAX_F32            v[max], v[max], v[tmp]
; ... log2(warpSize) iterations
; Cross-warp via LDS
DS_WRITE_B32         lds_off, v[max]
S_BARRIER
DS_READ_B32          v[other], lds_off+4
V_MAX_F32            v[max], v[max], v[other]
```

This mirrors the block reductions in `workloads/ml_kernels/softmax/`,
which uses explicit `__shfl_down` and shared memory.

### 3.4 Transcendental and Atomic Operations

| Triton Op       | CDNA3 ISA                          | Notes                  |
|-----------------|------------------------------------|------------------------|
| `tl.exp`        | V_EXP_F32                          | Base-2 exp, scaled     |
| `tl.log`        | V_LOG_F32                          | Base-2 log, scaled     |
| `tl.sin/cos`    | V_SIN_F32 / V_COS_F32             | HW approximation       |
| `tl.sqrt`       | V_SQRT_F32                         | Hardware square root   |
| `tl.sigmoid`    | V_EXP_F32 + V_RCP_F32 + V_ADD_F32 | Multi-instruction seq  |
| `tl.atomic_add` | GLOBAL_ATOMIC_ADD_F32              | Hardware atomic        |

---

## 4. Auto-Tuning Analysis

### 4.1 Tuning Parameters

Triton's `@triton.autotune` searches over kernel configurations:

```python
@triton.autotune(
    configs=[
        triton.Config({'BLOCK_M': 128, 'BLOCK_N': 128, 'BLOCK_K': 32},
                      num_warps=4, num_stages=3),
        triton.Config({'BLOCK_M': 256, 'BLOCK_N': 64,  'BLOCK_K': 32},
                      num_warps=8, num_stages=3),
    ],
    key=['M', 'N', 'K'],
)
```

| Parameter       | Effect on Generated ISA             | Typical Range |
|-----------------|-------------------------------------|---------------|
| `BLOCK_M/N/K`  | Tile dimensions → LDS size, regs    | 32–256        |
| `num_warps`     | Wavefronts per workgroup → occupancy | 2–8          |
| `num_stages`    | Pipeline depth → register usage     | 1–5           |

### 4.2 Impact on Generated Code

| Config                        | LDS    | VGPRs | Occupancy | Instructions |
|-------------------------------|--------|-------|-----------|-------------|
| BLOCK=64, warps=2, stages=1   | 8 KB   | ~40   | High      | ~200        |
| BLOCK=128, warps=4, stages=3  | 48 KB  | ~96   | Medium    | ~800        |
| BLOCK=256, warps=8, stages=4  | 128 KB | ~160  | Low       | ~2000       |

The same Triton kernel generates vastly different instruction streams
per configuration. The compiler makes **different decisions per GPU
architecture** — gfx942 output differs from gfx90a even for identical
Python source. For simulation, configurations must be pinned explicitly.

---

## 5. Kernel Extraction Methodology

### 5.1 Environment Variables

```bash
TRITON_CACHE_DIR=/tmp/triton_cache python my_kernel.py

# Cache produces per-kernel:
#   <hash>.ttir    — Triton IR (MLIR)
#   <hash>.ttgir   — Triton GPU IR (with layout)
#   <hash>.llir    — LLVM IR
#   <hash>.amdgcn  — AMDGCN assembly (AMD)
#   <hash>.hsaco   — Final binary (AMD)
```

### 5.2 Programmatic Extraction

```python
compiled = triton.compile(
    my_kernel,
    signature="*fp32, i32",
    constants={"BLOCK": 256},
    target=("hip", "gfx942"),
)
amdgcn_asm = compiled.asm["amdgcn"]
hsaco_binary = compiled.asm["hsaco"]
```

### 5.3 Example: Triton Matmul → CDNA3 Instruction Breakdown

For BLOCK_M=128, BLOCK_N=128, BLOCK_K=32 targeting gfx942:

```asm
; Main K-loop structure:
.Lloop:
    GLOBAL_LOAD_DWORDX4  v[4:7], v[a_addr], s[a_desc]   ; Load A tile
    GLOBAL_LOAD_DWORDX4  v[8:11], v[b_addr], s[b_desc]  ; Load B tile
    S_WAITCNT            vmcnt(0)
    DS_WRITE_B128        v[lds_a_off], v[4:7]            ; A → LDS
    DS_WRITE_B128        v[lds_b_off], v[8:11]           ; B → LDS
    S_BARRIER
    DS_READ_B128         v[a_frag:a_frag+3], v[lds_a_rd]
    DS_READ_B128         v[b_frag:b_frag+3], v[lds_b_rd]
    V_MFMA_F32_32X32X2_F32  a[0:15], v[a_frag], v[b_frag], a[0:15]
    S_BARRIER
    S_CBRANCH_SCC1       .Lloop
```

| Category             | Count  | Percentage |
|----------------------|--------|------------|
| Global load/store    | ~80    | 12%        |
| LDS read/write       | ~120   | 18%        |
| MFMA (matrix core)   | ~256   | 38%        |
| Address computation  | ~100   | 15%        |
| Control + sync       | ~50    | 8%         |
| Scalar ops           | ~60    | 9%         |

---

## 6. MGPUSim Integration Feasibility

### 6.1 Current Simulator Capabilities

| Instruction Category | MGPUSim Status | Triton Usage      |
|---------------------|----------------|-------------------|
| V_FMA_F32           | ✅ Working      | Fallback GEMM     |
| V_ADD/MUL/MAX_F32   | ✅ Working      | Elementwise ops   |
| V_EXP/LOG/SIN_F32   | ✅ Working      | Transcendentals   |
| GLOBAL_LOAD/STORE   | ✅ Working      | Memory access     |
| DS_READ/WRITE       | ⚠️ Partial     | Shared memory     |
| S_BARRIER           | ✅ Working      | Synchronization   |
| V_MFMA_F32_*        | ❌ Missing      | Matrix cores      |
| AGPR read/write     | ❌ Missing      | MFMA output regs  |
| DS_SWIZZLE          | ❌ Missing      | Warp reductions   |

### 6.2 Three Approaches

**A: Simulate Triton HSACO directly** — Requires MFMA, AGPR, DPP
support plus a Triton runtime shim. **Not feasible** without major
simulator extensions.

**B: Use Triton as reference for HIP kernels** — Compare Triton AMDGCN
output against hand-written HIP kernel assembly to validate equivalent
patterns. Use Triton's auto-tuned tile sizes to parameterize HIP
benchmarks. **Highly feasible**, directly actionable.

**C: Compile Triton without MFMA** — Target older arch (gfx908) to
force V_FMA_F32 fallback. Simulable but not representative of
production behavior. **Partially feasible** for memory validation.

### 6.3 HIP Kernel Equivalents Already Available

| Triton Kernel Type | HIP Equivalent in ml_kernels/       |
|-------------------|--------------------------------------|
| Triton matmul     | `gemm/hip/gemm.cpp` (tiled GEMM)    |
| Triton softmax    | `softmax/hip/softmax.cpp`            |
| Triton layernorm  | `layernorm/hip/layernorm.cpp`        |
| Triton attention  | `attention/hip/attention.cpp`        |
| Triton fused act  | `fused_swiglu/hip/fused_swiglu.cpp`  |
| Triton RoPE       | `rope/hip/rope.cpp`                  |

---

## 7. Performance Modeling Considerations

### 7.1 Triton vs Hand-Written HIP: Performance Delta

| Operation    | Triton % of rocBLAS | Hand-written HIP % of rocBLAS |
|-------------|---------------------|-------------------------------|
| GEMM (large) | 80–95%             | 40–65% (no MFMA)             |
| Softmax      | 90–100%            | 75–90%                        |
| LayerNorm    | 85–95%             | 70–85%                        |
| Fused activ. | 95–100%            | 85–95%                        |

The gap is largest for GEMM (MFMA vs V_FMA_F32). For memory-bound
kernels (softmax, layernorm, activations), the gap is smaller because
performance is limited by memory bandwidth, not compute throughput.

### 7.2 What Triton Analysis Adds

Even without direct simulation, Triton output provides:
1. **Optimal tile sizes** per architecture → parameterize HIP benchmarks
2. **Instruction mix reference** → validate HIP kernel distributions
3. **LDS allocation patterns** → verify shared memory sizing
4. **Register pressure data** → compare VGPR usage

---

## 8. Recommended Approach for Phase 2

### 8.1 Strategy

1. **Primary**: Use hand-written HIP kernels from `workloads/ml_kernels/`
   as MGPUSim benchmarks, compiled to CDNA3 ISA
2. **Validation**: Compare Triton AMDGCN assembly with HIP kernel
   assembly to confirm equivalent algorithmic patterns
3. **Parameterization**: Adopt Triton's auto-tuned tile sizes as defaults

### 8.2 Concrete Steps

| Step | Action                                              | Priority | Effort   |
|------|-----------------------------------------------------|----------|----------|
| 1    | Port `ml_kernels/gemm` → `amd/samples/gemm`        | High     | 1–2 cyc  |
| 2    | Port `ml_kernels/softmax` → `amd/samples/softmax`  | High     | 1 cycle  |
| 3    | Extract Triton matmul `.amdgcn` for gfx942          | Medium   | 1 cycle  |
| 4    | Compare instruction mix: Triton vs HIP gemm.cpp     | Medium   | 1 cycle  |
| 5    | Adopt Triton tile sizes for HIP benchmark defaults   | Low      | 0.5 cyc  |

### 8.3 Long-Term: MFMA Support

If MFMA is added to MGPUSim, direct Triton HSACO simulation becomes
feasible. Requirements: V_MFMA_F32_32X32X2_F32, AGPR register file,
V_ACCVGPR_READ/WRITE, and DS_SWIZZLE_B32. This would also enable
rocBLAS/Tensile kernel simulation.

---

## 9. References

1. Tillet et al., "Triton: An Intermediate Language and Compiler for
   Tiled Neural Network Computations," MLSys 2019.
   https://github.com/triton-lang/triton
2. AMD ROCm Triton fork: https://github.com/ROCm/triton
3. CDNA3 ISA: `docs/amd-instinct-mi300-cdna3-instruction-set-architecture.pdf`
4. Triton compiler guide: https://triton-lang.org/main/programming-guide/
5. ML kernel suite: `workloads/ml_kernels/RESEARCH.md`
6. cuBLAS/hipBLAS analysis: `docs/research_cublas_hipblas.md`
7. MGPUSim matmul accuracy: 22.36% MAPE (accuracy_analysis.md)
8. Flash Attention 2 (Triton): https://github.com/Dao-AILab/flash-attention
9. PyTorch TorchInductor + Triton: https://pytorch.org/docs/stable/torch.compiler.html
