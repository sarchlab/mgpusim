# ML Kernels Benchmark Suite -- Research Background

This document summarizes the research findings that motivated the design of the
`ml_kernels` benchmark suite. It covers why existing benchmarks are inadequate
for GPU architecture simulation of modern ML workloads, what real-world kernel
profiles look like, and how each benchmark in this suite maps to production
transformer models.

---

## 1. Motivation

### 1.1 The Gap in GPU Benchmark Suites

Traditional GPU benchmark suites were designed for scientific computing and
general-purpose GPU workloads:

| Suite     | Year | Focus                      | ML Relevance |
|-----------|------|----------------------------|--------------|
| Rodinia   | 2009 | Scientific/engineering     | Low          |
| PolyBench | 2012 | Polyhedral loop nests      | Low          |
| Parboil   | 2012 | Throughput computing       | Low          |
| SHOC      | 2010 | Hardware characterization  | Low          |
| Heteromark| 2017 | Heterogeneous computing    | Low          |
| DeepBench | 2017 | DNN primitives (GEMM/Conv) | Medium       |
| MLPerf    | 2018 | End-to-end ML training     | High (but opaque) |

None of these suites provide the **individual kernel-level benchmarks** needed
for GPU architecture simulation research. MLPerf runs complete training
pipelines with vendor libraries, making it impossible to isolate individual
kernel behavior. DeepBench comes closest but relies on cuBLAS/cuDNN, which are
closed-source.

### 1.2 Why Kernel-Level Benchmarks Matter

GPU architecture simulators (e.g., MGPUSim, Accel-Sim, GPGPU-Sim) need:

1. **Open-source kernels** -- closed vendor libraries cannot be simulated
2. **Configurable problem sizes** -- to sweep across architectural design points
3. **Representative compute patterns** -- matching real workload behavior
4. **Self-contained execution** -- no framework dependencies

### 1.3 Transformer Model Dominance

As of 2024, transformer-based models dominate GPU compute cycles:

- **Training**: GPT-2/3/4, Llama-2/3, PaLM, Gemini
- **Inference/Serving**: vLLM, TGI, TensorRT-LLM, Triton Inference Server
- **Fine-tuning**: LoRA, QLoRA, full fine-tuning

The GPU kernel composition of these workloads is well-characterized and
concentrated in a small number of operation types (see Section 3).

---

## 2. cuBLAS/hipBLAS Analysis

### 2.1 Vendor BLAS Libraries are Opaque

NVIDIA's cuBLAS and AMD's hipBLAS/rocBLAS are the standard BLAS implementations
for their respective GPU platforms. They provide highly optimized GEMM routines
that achieve >90% of peak throughput on modern GPUs.

However, these libraries are **unusable for architecture simulation**:

| Property              | cuBLAS / rocBLAS        | This Suite           |
|-----------------------|-------------------------|----------------------|
| Source availability   | Closed-source binary    | Fully open-source    |
| Kernel selection      | Runtime heuristic       | Explicit parameters  |
| Tiling strategy       | Opaque, device-specific | Configurable tile    |
| Shared memory usage   | Unknown                 | Explicit in code     |
| Register pressure     | Tuned per GPU           | Visible in source    |
| Simulation support    | Cannot disassemble      | Direct compilation   |

### 2.2 How cuBLAS Selects Kernels

cuBLAS uses a multi-level kernel selection process:

1. **Problem shape classification**: M, N, K dimensions determine kernel family
2. **Tile size selection**: Chosen from a pre-tuned lookup table per GPU
3. **Split-K decision**: For small-M problems (inference), K-dimension is split
4. **Epilogue fusion**: Bias, activation, scaling fused into GEMM epilogue

For example, on A100 for a typical Llama-2-7B forward pass GEMM (M=4096,
N=4096, K=4096), cuBLAS selects a 256×128 tile with 4-stage pipeline, achieving
~280 TFLOPS (FP16 Tensor Cores) or ~19 TFLOPS (FP32).

### 2.3 Our Approach: Open-Source Tiled GEMM

Instead of using vendor libraries, we implement a tiled GEMM with configurable:

- **Tile dimensions** (TILE_M, TILE_N, TILE_K): control shared memory footprint
- **Block size**: threads per block, affecting occupancy
- **Matrix dimensions** (M, N, K): matching real model layer shapes

This gives architecture researchers full visibility into memory access patterns,
shared memory bank conflicts, register usage, and instruction mix -- all critical
for simulation accuracy.

### 2.4 Key GEMM Shapes from Real Models

| Model        | Layer          | M      | N      | K      | GFLOPS (FP32) |
|--------------|----------------|--------|--------|--------|----------------|
| GPT-2 Small  | QKV projection | 768    | 2304   | 768    | 2.72           |
| GPT-2 Small  | FFN up         | 768    | 3072   | 768    | 3.62           |
| GPT-2 Small  | FFN down       | 3072   | 768    | 768    | 3.62           |
| Llama-2-7B   | QKV projection | 4096   | 4096   | 4096   | 137.4          |
| Llama-2-7B   | Gate proj      | 4096   | 11008  | 4096   | 369.1          |
| Llama-2-7B   | Up proj        | 4096   | 11008  | 4096   | 369.1          |
| Llama-2-7B   | Down proj      | 11008  | 4096   | 4096   | 369.1          |
| Llama-2-70B  | QKV projection | 8192   | 8192   | 8192   | 1099.5         |
| Llama-2-70B  | Gate proj      | 8192   | 28672  | 8192   | 3848.3         |
| Llama-2-70B  | Down proj      | 28672  | 8192   | 8192   | 3848.3         |

These shapes are directly encoded in the `gemm/params.json` scaling parameters.

---

## 3. Triton & ML Framework Kernel Analysis

### 3.1 Triton Compilation Pipeline

OpenAI Triton is a Python-based language for writing GPU kernels. Its
compilation pipeline is:

```
Triton Python DSL
    → Triton IR (MLIR-based)
    → LLVM IR
    → PTX (NVIDIA) / AMDGPU IR (AMD)
    → Machine code (cubin / hsaco)
```

Triton kernels can theoretically be extracted at the PTX/AMDGPU IR level for
simulation. However, in practice:

- Triton generates **highly specialized code** per problem size
- The generated PTX changes with every Triton version
- Auto-tuning selects different tile sizes per hardware target

For simulation research, **hand-written kernels with explicit parameters** are
more reproducible and inspectable than Triton-generated code.

### 3.2 PyTorch Training Kernel Profile

Profiling transformer training with PyTorch (using `torch.profiler`) on an A100
GPU reveals the following kernel time breakdown:

| Kernel Category       | % GPU Time | Example Kernels                        |
|-----------------------|------------|----------------------------------------|
| GEMM (forward)        | 30-40%     | ampere_sgemm, cublas_gemm              |
| GEMM (backward)       | 25-35%     | cublas_gemm (dW, dX)                   |
| Attention (softmax)   | 5-10%      | softmax_warp_forward                   |
| Attention (score)     | 5-8%       | fused_attention_kernel (Flash)         |
| LayerNorm             | 3-8%       | layer_norm_forward_kernel              |
| Elementwise (activ.)  | 3-6%       | gelu_kernel, silu_kernel               |
| Dropout / Mask        | 2-4%       | fused_dropout_kernel                   |
| Optimizer (Adam)      | 2-5%       | multi_tensor_adam                      |
| AllReduce (comm)      | 5-15%      | ncclKernel (excluded from this suite)  |
| Other                 | 2-5%       | embedding, positional encoding         |

**Key insight**: GEMM dominates at 55-75% of total GPU time. The remaining
25-45% is split among memory-bound operations (attention, layernorm, softmax,
activations) that are individually small but collectively significant.

### 3.3 vLLM Serving Kernel Profile

LLM inference serving (vLLM on A100, Llama-2-7B, batch=32) shows a different
distribution due to the autoregressive decode phase:

| Kernel Category         | % GPU Time | Notes                              |
|-------------------------|------------|------------------------------------|
| GEMM (prefill)          | 25-35%     | Large batch, compute-bound         |
| GEMM (decode)           | 15-25%     | Small batch (M=1-32), memory-bound |
| PagedAttention          | 15-30%     | Custom kernel for KV cache paging  |
| RMSNorm / LayerNorm     | 3-5%       | Fused with residual connection     |
| RoPE (rotary embed)     | 2-5%       | Elementwise, complex arithmetic    |
| SwiGLU activation       | 2-4%       | Fused SiLU * linear, elementwise   |
| Softmax                 | 2-4%       | Online softmax in attention        |
| Sampling / Top-k        | 1-3%       | Token selection                    |
| KV cache management     | 2-5%       | Memory copies, page table ops     |

**Key insight**: In serving, the decode phase makes GEMM less dominant and
elevates the importance of attention, RoPE, and activation kernels.

### 3.4 Kernel Arithmetic Intensity Analysis

The arithmetic intensity (FLOP/byte) determines whether a kernel is compute-
bound or memory-bound on a given GPU:

| Kernel         | FLOP/element | Bytes/element | AI (FLOP/byte) | Bound     |
|----------------|--------------|---------------|-----------------|-----------|
| GEMM           | 2*K          | 4 (amortized) | K/2             | Compute   |
| Softmax        | ~5           | 8-12          | 0.4-0.6         | Memory    |
| LayerNorm      | ~10          | 12            | 0.8             | Memory    |
| RMSNorm        | ~6           | 8             | 0.75            | Memory    |
| RoPE           | ~8           | 8             | 1.0             | Memory    |
| SwiGLU         | ~4           | 12            | 0.33            | Memory    |
| GELU           | ~12          | 8             | 1.5             | Memory    |
| Attention      | 2*seq_len    | 4 (amortized) | seq_len/2       | Varies    |

On an A100 (peak 19.5 TFLOPS FP32, 2039 GB/s HBM bandwidth), the
compute-memory balance point is at AI ≈ 9.6 FLOP/byte. Only GEMM (for K > 19)
and attention (for seq_len > 19) are compute-bound; all other kernels are
memory-bandwidth-limited.

---

## 4. Benchmark Design Decisions

### 4.1 The Seven Kernels

This suite contains 7 benchmarks covering the most critical ML operations:

| # | Benchmark      | Category        | Pattern            | Bound    |
|---|----------------|-----------------|--------------------|-----------| 
| 1 | `gemm`         | Linear algebra  | Tiled matrix mult  | Compute  |
| 2 | `softmax`      | Attention       | Row-wise reduction | Memory   |
| 3 | `layernorm`    | Normalization   | Row-wise reduction | Memory   |
| 4 | `attention`    | Attention       | Batched GEMM+Soft  | Varies   |
| 5 | `rope`         | Positional enc. | Elementwise trig   | Memory   |
| 6 | `activation`   | FFN activation  | Elementwise math   | Memory   |
| 7 | `fused_swiglu` | FFN activation  | Fused elementwise  | Memory   |

### 4.2 Design Principles

1. **Self-contained**: Each benchmark compiles and runs independently with
   zero external dependencies (no cuBLAS, no PyTorch, no Python).

2. **Parameterized**: All kernels accept command-line parameters for problem
   size, block size, and kernel-specific configuration. This enables automated
   sweeps across the design space.

3. **Deterministic initialization**: All data is generated with `srand(42)`,
   ensuring reproducible results across runs.

4. **Real model shapes**: Parameter values in `params.json` are derived from
   actual model configurations (GPT-2, Llama-2-7B, Llama-2-70B).

5. **CSV output**: All benchmarks use the common benchmark harness
   (`bench_common_cuda.h` / `bench_common_hip.h`) to produce standardized
   CSV output for automated analysis.

6. **Dual platform**: Every benchmark has both CUDA and HIP implementations,
   enabling cross-platform GPU architecture research.

### 4.3 Why These 7 and Not Others?

**Included**: Operations that appear in the top-10 GPU kernel time consumers
across both training and inference workloads.

**Excluded** (with justification):

| Operation       | Reason for Exclusion                                    |
|-----------------|----------------------------------------------------------|
| Convolution     | Not used in transformer models (CNNs are separate)       |
| BatchNorm       | Replaced by LayerNorm/RMSNorm in transformers            |
| Embedding       | Simple table lookup, not computationally interesting      |
| Optimizer (Adam)| Trivial elementwise, similar pattern to activation        |
| Dropout         | Random mask + multiply, covered by elementwise patterns   |
| AllReduce       | Communication kernel, outside single-GPU simulation scope |
| Quantization    | Emerging area, to be added in future iteration            |

### 4.4 Kernel Fusion Considerations

Modern ML frameworks aggressively fuse kernels to reduce memory bandwidth
pressure. Our `fused_swiglu` benchmark represents this trend:

- **Unfused**: `SiLU(gate)` → write to memory → read back → multiply with `up`
  - 4 memory transactions: read gate, write silu_gate, read silu_gate, read up
  - Plus 1 write for output = 5 × N × 4 bytes = 20N bytes total

- **Fused**: Read gate + up → SiLU(gate) * up → write output
  - 3 memory transactions: read gate, read up, write output
  - = 3 × N × 4 bytes = 12N bytes total
  - **1.67× bandwidth reduction** from fusion

This is representative of how PyTorch's `torch.compile` and Triton fuse
elementwise operations in production.

---

## 5. Model Shape Reference

### 5.1 Architecture Parameters

| Parameter            | GPT-2 Small | GPT-2 Medium | Llama-2-7B | Llama-2-13B | Llama-2-70B |
|----------------------|-------------|--------------|------------|-------------|-------------|
| Hidden dim (d_model) | 768         | 1024         | 4096       | 5120        | 8192        |
| Num heads            | 12          | 16           | 32         | 40          | 64          |
| Head dim (d_k)       | 64          | 64           | 128        | 128         | 128         |
| Num KV heads         | 12          | 16           | 32         | 40          | 8 (GQA)     |
| FFN intermediate     | 3072        | 4096         | 11008      | 13824       | 28672       |
| Num layers           | 12          | 24           | 32         | 40          | 80          |
| Max sequence length   | 1024        | 1024         | 4096       | 4096        | 4096        |
| Vocab size           | 50257       | 50257        | 32000      | 32000       | 32000       |
| Total parameters     | 124M        | 355M         | 6.7B       | 13B         | 70B         |
| Activation function  | GELU        | GELU         | SiLU/SwiGLU| SiLU/SwiGLU | SiLU/SwiGLU |
| Normalization        | LayerNorm   | LayerNorm    | RMSNorm    | RMSNorm     | RMSNorm     |
| Positional encoding  | Learned     | Learned      | RoPE       | RoPE        | RoPE        |

### 5.2 Mapping Benchmarks to Model Shapes

#### GEMM Benchmark

The GEMM benchmark sweeps M × N × K dimensions. Representative shapes:

| Model       | Operation     | M     | N     | K     | FLOPs (per call) |
|-------------|---------------|-------|-------|-------|------------------|
| GPT-2-S     | QKV proj      | 768   | 2304  | 768   | 2.72G            |
| GPT-2-S     | Out proj      | 768   | 768   | 768   | 0.91G            |
| GPT-2-S     | FFN up        | 768   | 3072  | 768   | 3.62G            |
| GPT-2-S     | FFN down      | 3072  | 768   | 768   | 3.62G            |
| Llama-7B    | Q proj        | 4096  | 4096  | 4096  | 137.4G           |
| Llama-7B    | K proj (GQA)  | 4096  | 4096  | 4096  | 137.4G           |
| Llama-7B    | Gate proj     | 4096  | 11008 | 4096  | 369.1G           |
| Llama-7B    | Up proj       | 4096  | 11008 | 4096  | 369.1G           |
| Llama-7B    | Down proj     | 11008 | 4096  | 4096  | 369.1G           |
| Llama-70B   | Q proj        | 8192  | 8192  | 8192  | 1099.5G          |
| Llama-70B   | Gate proj     | 8192  | 28672 | 8192  | 3848.3G          |

With batched inference (batch=B, seq_len=S), the effective M = B × S.

#### Softmax Benchmark

Softmax operates on attention scores of shape [batch, heads, seq_len, seq_len]:

| Model    | Batch | Seq Len | Heads | Elements per row | Total elements |
|----------|-------|---------|-------|------------------|----------------|
| GPT-2-S  | 1     | 512     | 12    | 512              | 3.1M           |
| GPT-2-S  | 8     | 1024    | 12    | 1024             | 100.7M         |
| Llama-7B | 1     | 2048    | 32    | 2048             | 134.2M         |
| Llama-7B | 4     | 4096    | 32    | 4096             | 2147.5M        |
| Llama-70B| 1     | 4096    | 64    | 4096             | 1073.7M        |

#### LayerNorm / RMSNorm Benchmark

Normalization operates on hidden-dimension vectors:

| Model    | Batch×Seq  | Hidden Dim | Total elements |
|----------|------------|------------|----------------|
| GPT-2-S  | 8×1024     | 768        | 6.3M           |
| Llama-7B | 4×2048     | 4096       | 33.6M          |
| Llama-7B | 32×4096    | 4096       | 536.9M         |
| Llama-70B| 4×4096     | 8192       | 134.2M         |

#### Attention Benchmark

Multi-head attention with shape [batch, heads, seq_len, head_dim]:

| Model    | Batch | Heads | Seq Len | Head Dim | Score FLOPs |
|----------|-------|-------|---------|----------|-------------|
| GPT-2-S  | 1     | 12    | 1024    | 64       | 1.6G        |
| Llama-7B | 1     | 32    | 2048    | 128      | 34.4G       |
| Llama-7B | 4     | 32    | 4096    | 128      | 549.8G      |
| Llama-70B| 1     | 64    | 4096    | 128      | 274.9G      |

#### RoPE Benchmark

Rotary positional embeddings operate on Q/K tensors:

| Model    | Batch×Seq  | Heads | Head Dim | Total elements |
|----------|------------|-------|----------|----------------|
| Llama-7B | 4×2048     | 32    | 128      | 33.6M          |
| Llama-7B | 32×4096    | 32    | 128      | 536.9M         |
| Llama-70B| 4×4096     | 64+8  | 128      | 150.0M         |

#### Activation Benchmark

Elementwise activations on FFN hidden states:

| Model    | Batch×Seq  | FFN Dim | Total elements |
|----------|------------|---------|----------------|
| GPT-2-S  | 8×1024     | 3072    | 25.2M          |
| Llama-7B | 4×2048     | 11008   | 90.2M          |
| Llama-7B | 32×4096    | 11008   | 1443.0M        |
| Llama-70B| 4×4096     | 28672   | 469.8M         |

#### Fused SwiGLU Benchmark

SwiGLU operates on the same shapes as the activation benchmark, but reads
two inputs (gate and up projections) and writes one output:

| Model    | Batch×Seq  | FFN Dim | Total elements | Bytes (3×N×4) |
|----------|------------|---------|----------------|---------------|
| GPT-2-S  | 8×1024     | 3072    | 25.2M          | 302.0 MB      |
| Llama-7B | 1×1024     | 11008   | 11.3M          | 135.2 MB      |
| Llama-7B | 4×1024     | 11008   | 45.1M          | 540.7 MB      |
| Llama-7B | 4×2048     | 11008   | 90.2M          | 1081.3 MB     |
| Llama-7B | 32×4096    | 11008   | 1443.0M        | 17316.0 MB    |
| Llama-70B| 4×4096     | 28672   | 469.8M         | 5637.1 MB     |

---

## 6. GPU Architecture Considerations

### 6.1 Memory Hierarchy Impact

Different kernels stress different levels of the GPU memory hierarchy:

| Kernel         | L1/Shared Memory | L2 Cache    | HBM Bandwidth |
|----------------|------------------|-------------|----------------|
| GEMM           | Heavy (tiling)   | Moderate    | Moderate       |
| Softmax        | Light            | Heavy       | Heavy          |
| LayerNorm      | Moderate (reduce)| Moderate    | Heavy          |
| Attention      | Heavy (tiling)   | Heavy       | Heavy          |
| RoPE           | None             | Light       | Heavy          |
| Activation     | None             | Light       | Heavy          |
| Fused SwiGLU   | None             | Light       | Heavy          |

### 6.2 Occupancy and Parallelism

Each kernel has different occupancy characteristics:

| Kernel         | Registers/thread | Shared mem | Typical occupancy |
|----------------|------------------|------------|-------------------|
| GEMM (tiled)   | 32-64           | 8-48 KB    | 25-50%            |
| Softmax        | 12-20           | 0-4 KB     | 75-100%           |
| LayerNorm      | 16-24           | 2-8 KB     | 50-75%            |
| Attention      | 24-48           | 8-32 KB    | 25-50%            |
| RoPE           | 12-16           | 0 KB       | 100%              |
| Activation     | 8-12            | 0 KB       | 100%              |
| Fused SwiGLU   | 10-14           | 0 KB       | 100%              |

### 6.3 Instruction Mix

The instruction mix varies significantly across kernels, which is important
for architecture simulation of different functional units:

| Kernel         | FMA    | Transcendental | Load/Store | Control |
|----------------|--------|----------------|------------|---------|
| GEMM           | ~80%   | 0%             | ~15%       | ~5%     |
| Softmax        | ~20%   | ~30% (exp)     | ~40%       | ~10%    |
| LayerNorm      | ~40%   | ~10% (rsqrt)   | ~40%       | ~10%    |
| Attention      | ~60%   | ~10% (exp)     | ~25%       | ~5%     |
| RoPE           | ~30%   | ~30% (sin/cos) | ~30%       | ~10%    |
| Activation     | ~20%   | ~40% (exp/tanh)| ~30%       | ~10%    |
| Fused SwiGLU   | ~25%   | ~30% (exp)     | ~35%       | ~10%    |

---

## 7. Benchmark Execution and Metrics

### 7.1 Common Benchmark Harness

All benchmarks use the shared harness in `workloads/common/`:
- `bench_common_cuda.h` for CUDA
- `bench_common_hip.h` for HIP

The harness provides:
- `parseIterations(argc, argv)` -- parse `--iterations` flag
- `parseIntParam(argc, argv, name, default)` -- parse integer parameters
- `runBenchmark(name, problemSize, iterations, kernel_lambda)` -- warm-up +
  timed execution with GPU synchronization
- `printCSVHeader()` / `printCSVRow(result)` -- standardized CSV output

### 7.2 Reported Metrics

Each benchmark reports:
- **Kernel time** (avg, min, max in milliseconds)
- **Bandwidth** (GB/s) -- bytes transferred / time
- **Throughput** (GFLOPS or Gops/s) -- operations / time

### 7.3 Parameter Sweeping

Each benchmark includes a `params.json` file defining:
- **Scaling parameter**: The primary dimension swept to vary problem size
- **Non-scaling parameters**: Secondary knobs (block_size, tile dimensions,
  algorithm variants)

Automated sweep scripts can read `params.json` to generate exhaustive
combinations for architecture design space exploration.

---

## 8. Related Work and References

### 8.1 Benchmark Suites

- **DeepBench** (Baidu Research, 2017): GEMM, Conv, RNN shapes extracted from
  real DNN models. Relies on vendor BLAS libraries for execution. Motivated our
  GEMM shape selection but not our implementation approach.
  https://github.com/baidu-research/DeepBench

- **MLPerf Training v4.0** (MLCommons, 2024): End-to-end training benchmarks
  for ResNet, BERT, GPT-3, Llama-2, Stable Diffusion. Measures time-to-accuracy
  on full models. Too coarse-grained for architecture simulation.
  https://mlcommons.org/benchmarks/training/

- **MLPerf Inference v4.0** (MLCommons, 2024): Inference benchmarks including
  LLM serving (GPT-J, Llama-2-70B). Focuses on throughput and latency SLAs.
  https://mlcommons.org/benchmarks/inference/

### 8.2 GPU Kernel Libraries

- **CUTLASS** (NVIDIA, 2017-present): Template-based GEMM library exposing
  tile sizes, warp arrangements, and pipeline stages. Our tiled GEMM design
  follows CUTLASS algorithmic patterns (global→shared→register tiling) but
  with simplified implementations suitable for simulation.
  https://github.com/NVIDIA/cutlass

- **Composable Kernel** (AMD, 2022-present): AMD's equivalent to CUTLASS for
  ROCm. Provides template-based GEMM and attention kernels. Informed our HIP
  implementation patterns.
  https://github.com/ROCm/composable_kernel

- **Flash Attention** (Tri Dao et al., 2022): Memory-efficient attention using
  online softmax and tiling to avoid materializing the full attention matrix.
  Our attention benchmark implements the tiled attention pattern from Flash
  Attention v1.
  https://github.com/Dao-AILab/flash-attention

- **Flash Attention 2** (Tri Dao, 2023): Improved parallelism (splitting across
  sequence length dimension) and reduced non-matmul FLOPs. Achieves 50-73% of
  A100 theoretical peak.

### 8.3 LLM Serving Systems

- **vLLM** (Kwon et al., 2023): PagedAttention for efficient KV cache
  management during LLM serving. Introduced the concept of paged memory for
  attention KV caches, analogous to OS virtual memory.
  https://github.com/vllm-project/vllm

- **TensorRT-LLM** (NVIDIA, 2023-present): Optimized inference engine using
  custom CUDA kernels for attention, GEMM, and normalization. Closed-source
  kernels prevent use in simulation.

### 8.4 GPU Architecture Simulators

- **GPGPU-Sim** (Bakhoda et al., 2009): Cycle-level GPU simulator. Requires
  PTX input, making open-source CUDA kernels essential.

- **Accel-Sim** (Khairy et al., 2020): Trace-driven GPU simulation framework
  built on GPGPU-Sim. Can trace and replay real GPU kernels.

- **MGPUSim** (Sun et al., 2019): Go-based GPU simulator supporting GCN3/RDNA
  ISA. Requires GCN/RDNA binaries compiled from HIP source.

### 8.5 Key Papers

- Vaswani et al., "Attention Is All You Need" (NeurIPS 2017): Introduced the
  transformer architecture with multi-head attention and FFN layers.

- Shazeer, "GLU Variants Improve Transformer" (2020): Proposed SwiGLU and other
  gated linear unit variants, showing consistent improvements over standard
  FFN. SwiGLU adopted by Llama-2, PaLM, and other modern LLMs.

- Touvron et al., "Llama 2: Open Foundation and Fine-Tuned Chat Models" (2023):
  Described the Llama-2 architecture including SwiGLU activation, RoPE, RMSNorm,
  and grouped-query attention (GQA).

- Su et al., "RoFormer: Enhanced Transformer with Rotary Position Embedding"
  (2021): Introduced RoPE, now standard in Llama, Mistral, and other models.

- Zhang & Sennrich, "Root Mean Square Layer Normalization" (2019): Proposed
  RMSNorm as a simpler alternative to LayerNorm, adopted by Llama-2.

---

## 9. Future Work

Potential extensions to this benchmark suite:

1. **FP16/BF16 kernels**: Half-precision GEMM and attention leveraging Tensor
   Cores (NVIDIA) or Matrix Cores (AMD).

2. **Quantized kernels**: INT8/INT4 GEMM for quantized inference (GPTQ, AWQ).

3. **Grouped-Query Attention (GQA)**: Llama-2-70B uses 8 KV heads with 64 Q
   heads, requiring specialized attention kernels.

4. **Speculative decoding kernels**: Draft model verification and token
   acceptance/rejection logic.

5. **MoE routing**: Mixture-of-Experts top-k gating and permutation kernels
   (Mixtral, Switch Transformer).

6. **Multi-GPU communication**: NCCL/RCCL AllReduce and AllGather patterns for
   tensor/pipeline parallelism.
