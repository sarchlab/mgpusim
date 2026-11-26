// Device source from GPU Benchmarks code_fetch_latency.hip (sarchlab/gpu_benchmarks#198, f50fb0f).
#include <hip/hip_runtime.h>

/* Straight-line bodies of exactly N dependent FMAs, by doubling. */
#define FETCH_OPS_1     x = fmaf(x, a, b);
#define FETCH_OPS_2     FETCH_OPS_1 FETCH_OPS_1
#define FETCH_OPS_4     FETCH_OPS_2 FETCH_OPS_2
#define FETCH_OPS_8     FETCH_OPS_4 FETCH_OPS_4
#define FETCH_OPS_16    FETCH_OPS_8 FETCH_OPS_8
#define FETCH_OPS_32    FETCH_OPS_16 FETCH_OPS_16
#define FETCH_OPS_64    FETCH_OPS_32 FETCH_OPS_32
#define FETCH_OPS_128   FETCH_OPS_64 FETCH_OPS_64
#define FETCH_OPS_256   FETCH_OPS_128 FETCH_OPS_128
#define FETCH_OPS_512   FETCH_OPS_256 FETCH_OPS_256
#define FETCH_OPS_1024  FETCH_OPS_512 FETCH_OPS_512
#define FETCH_OPS_2048  FETCH_OPS_1024 FETCH_OPS_1024
#define FETCH_OPS_4096  FETCH_OPS_2048 FETCH_OPS_2048
#define FETCH_OPS_8192  FETCH_OPS_4096 FETCH_OPS_4096
#define FETCH_OPS_16384 FETCH_OPS_8192 FETCH_OPS_8192
#define FETCH_OPS_32768 FETCH_OPS_16384 FETCH_OPS_16384
#define FETCH_OPS_65536 FETCH_OPS_32768 FETCH_OPS_32768

/* Every compiled body size: each power of two from MIN_BODY_OPS to
 * MAX_BODY_OPS. Kernels and their dispatch are generated from this one list. */
#define FETCH_FOR_EACH_SIZE(X) \
    X(16) X(32) X(64) X(128) X(256) X(512) X(1024) X(2048) X(4096) X(8192) \
    X(16384) X(32768) X(65536)

/* Chain seed of a block's thread: 1 + thread/1024, exact in float for
 * every launchable thread index, so device and host start from identical bits
 * whether or not a compiler contracts the multiply-add. */
#define FETCH_SEED(thread) (1.0f + static_cast<float>(thread) * 0.0009765625f)
/* The single store that makes the chain observable. */
#define FETCH_STORE(value) \
    out[static_cast<size_t>(blockIdx.x) * blockDim.x + threadIdx.x] = (value)

/**
 * fetch_kernel_<body_ops> — each thread runs reps * body_ops dependent FMAs.
 *
 * The loop body is body_ops straight-line FMAs; `unroll 1` keeps the compiler
 * from unrolling the loop, which would change the code footprint. Check the
 * compiled code before using a result (see README).
 *
 * @param out   Device array with one element per launched thread.
 * @param a, b  Chain coefficients (runtime values).
 * @param reps  Loop iterations (`loops`); FMAs per thread = reps * body_ops.
 */
#define FETCH_DEFINE_KERNEL(N)                                                 \
    extern "C" __global__ void fetch_kernel_##N(float *out, float a, float b, int reps)   \
    {                                                                          \
        float x = FETCH_SEED(threadIdx.x);                                     \
        _Pragma("unroll 1")                                                    \
        for (int r = 0; r < reps; ++r) { FETCH_OPS_##N }                       \
        FETCH_STORE(x);                                                        \
    }

FETCH_FOR_EACH_SIZE(FETCH_DEFINE_KERNEL)
