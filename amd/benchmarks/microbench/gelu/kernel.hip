#include <hip/hip_runtime.h>

// GELU activation: y = x * 0.5 * (1 + tanh(sqrt(2/pi) * (x + 0.044715*x^3)))
//
// Uses explicit exp2 + rcp builtins to avoid IEEE-compliant software division
// sequence (v_div_scale/v_div_fmas/v_div_fixup) and packed v_pk_add_f32,
// which can crash or hang the timing simulator.
//
// tanh(z) = (exp(2z) - 1) / (exp(2z) + 1)
//         = (exp(2z) - 1) * rcp(exp(2z) + 1)
// where exp(2z) = exp2(2z * log2(e))
__global__ void gelu_kernel(float* __restrict__ A,
                            float* __restrict__ B,
                            int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        float x = A[gid];
        float x3 = x * x * x;
        float inner = 0.7978845608f * (x + 0.044715f * x3);

        // exp(2*inner) = exp2(2 * inner * log2(e))
        // 2 * log2(e) = 2.885390081
        float e2z = __builtin_amdgcn_exp2f(inner * 2.885390081f);
        float t = (e2z - 1.0f) * __builtin_amdgcn_rcpf(e2z + 1.0f);

        B[gid] = 0.5f * x * (1.0f + t);
    }
}
