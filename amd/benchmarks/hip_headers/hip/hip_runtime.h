/**
 * Minimal HIP runtime header for standalone kernel compilation with clang.
 *
 * For use with:
 *   clang -target amdgcn-amd-amdhsa -mcpu=gfx942 -x c++ -nostdinc
 *         -D__AMDGCN__=1 -I<this-dir> ...
 *
 * This avoids clang-offload-bundler which is not available on Fedora aarch64.
 */
#ifndef HIP_RUNTIME_H
#define HIP_RUNTIME_H

/* Basic type definitions - use built-in types since -nostdinc may be used */
#ifndef NULL
#  define NULL ((void*)0)
#endif

/* Define basic integer types manually to avoid stdint.h dependency */
typedef signed char        int8_t;
typedef unsigned char      uint8_t;
typedef signed short       int16_t;
typedef unsigned short     uint16_t;
typedef signed int         int32_t;
typedef unsigned int       uint32_t;
typedef signed long long   int64_t;
typedef unsigned long long uint64_t;

typedef unsigned long size_t;

/* --- HIP function qualifiers ---
 * With -target amdgcn-amd-amdhsa -x c++:
 *   - __global__ must be __attribute__((amdgpu_kernel)) for kernel entry points
 *   - __device__ is just for regular device functions (no special attribute needed)
 *   - __shared__ maps to __attribute__((address_space(3)))
 *   - __constant__ maps to __attribute__((address_space(4)))
 */
#ifndef __global__
#  if defined(__AMDGCN__)
#    define __global__    extern "C" __attribute__((amdgpu_kernel))
#  else
#    define __global__    /* host: kernel functions are no-ops */
#  endif
#endif

#ifndef __device__
#  if defined(__AMDGCN__)
#    define __device__    /* device functions are normal in amdgcn target */
#  else
#    define __device__    /* host: empty */
#  endif
#endif

#ifndef __host__
#  define __host__        /* always empty */
#endif

#ifndef __forceinline__
#  define __forceinline__ __attribute__((always_inline)) inline
#endif

#ifndef __shared__
#  if defined(__AMDGCN__)
/* Shared memory must be static+uninitialized in AMDGCN target.
 * loader_uninitialized suppresses default zero-initialization for LDS. */
#    define __shared__    static __attribute__((address_space(3))) __attribute__((loader_uninitialized))
#  else
#    define __shared__    static  /* fallback on host */
#  endif
#endif

#ifndef __constant__
#  if defined(__AMDGCN__)
#    define __constant__  __attribute__((address_space(4)))
#  else
#    define __constant__  static const
#  endif
#endif

/* --- Basic types --- */
typedef unsigned char uchar;
typedef unsigned short ushort;
typedef unsigned int uint;
typedef unsigned long long ulonglong;
typedef long long longlong;

typedef int hipError_t;
#define hipSuccess 0

/* dim3 type */
typedef struct {
    unsigned int x, y, z;
} dim3;

/* HIP_KERNEL_NAME macro */
#define HIP_KERNEL_NAME(...) __VA_ARGS__

#ifdef __AMDGCN__
/* ================================================================
 * Device-side code (compiling for AMDGPU target)
 * ================================================================ */

/* --- Thread/block index built-ins via AMDGPU intrinsics --- */
#define hipThreadIdx_x (__builtin_amdgcn_workitem_id_x())
#define hipThreadIdx_y (__builtin_amdgcn_workitem_id_y())
#define hipThreadIdx_z (__builtin_amdgcn_workitem_id_z())

#define hipBlockIdx_x  (__builtin_amdgcn_workgroup_id_x())
#define hipBlockIdx_y  (__builtin_amdgcn_workgroup_id_y())
#define hipBlockIdx_z  (__builtin_amdgcn_workgroup_id_z())

#define hipBlockDim_x  (__builtin_amdgcn_workgroup_size_x())
#define hipBlockDim_y  (__builtin_amdgcn_workgroup_size_y())
#define hipBlockDim_z  (__builtin_amdgcn_workgroup_size_z())

#define hipGridDim_x   (__builtin_amdgcn_grid_size_x() / __builtin_amdgcn_workgroup_size_x())
#define hipGridDim_y   (__builtin_amdgcn_grid_size_y() / __builtin_amdgcn_workgroup_size_y())
#define hipGridDim_z   (__builtin_amdgcn_grid_size_z() / __builtin_amdgcn_workgroup_size_z())

/* --- CUDA-style index structs (for kernels using threadIdx.x etc.) --- */
struct __idx3 {
    unsigned int x, y, z;
};
struct __dim3 {
    unsigned int x, y, z;
};

/* threadIdx */
static __inline__ struct __idx3 __get_threadIdx(void) {
    struct __idx3 t;
    t.x = __builtin_amdgcn_workitem_id_x();
    t.y = __builtin_amdgcn_workitem_id_y();
    t.z = __builtin_amdgcn_workitem_id_z();
    return t;
}
#define threadIdx (__get_threadIdx())

/* blockIdx */
static __inline__ struct __idx3 __get_blockIdx(void) {
    struct __idx3 b;
    b.x = __builtin_amdgcn_workgroup_id_x();
    b.y = __builtin_amdgcn_workgroup_id_y();
    b.z = __builtin_amdgcn_workgroup_id_z();
    return b;
}
#define blockIdx (__get_blockIdx())

/* blockDim */
static __inline__ struct __dim3 __get_blockDim(void) {
    struct __dim3 d;
    d.x = __builtin_amdgcn_workgroup_size_x();
    d.y = __builtin_amdgcn_workgroup_size_y();
    d.z = __builtin_amdgcn_workgroup_size_z();
    return d;
}
#define blockDim (__get_blockDim())

/* gridDim */
static __inline__ struct __dim3 __get_gridDim(void) {
    struct __dim3 d;
    d.x = __builtin_amdgcn_grid_size_x() / __builtin_amdgcn_workgroup_size_x();
    d.y = __builtin_amdgcn_grid_size_y() / __builtin_amdgcn_workgroup_size_y();
    d.z = __builtin_amdgcn_grid_size_z() / __builtin_amdgcn_workgroup_size_z();
    return d;
}
#define gridDim (__get_gridDim())

/* --- Synchronization --- */
#define __syncthreads() __builtin_amdgcn_s_barrier()

/* --- Atomics (device side) --- */
static inline int atomicAdd(int *addr, int val) {
    return __atomic_fetch_add(addr, val, 0);
}
static inline unsigned int atomicAdd(unsigned int *addr, unsigned int val) {
    return __atomic_fetch_add(addr, val, 0);
}
static inline unsigned long long atomicAdd(unsigned long long *addr, unsigned long long val) {
    return __atomic_fetch_add(addr, val, 0);
}
static inline float atomicAdd(float *addr, float val) {
    unsigned int ival, iold, inew;
    float fold, fnew;
    do {
        iold = __atomic_load_n((unsigned int *)addr, 0);
        __builtin_memcpy(&fold, &iold, sizeof(float));
        fnew = fold + val;
        __builtin_memcpy(&inew, &fnew, sizeof(float));
        ival = iold;
    } while (!__atomic_compare_exchange_n((unsigned int *)addr, &ival, inew, 0, 0, 0));
    return fold;
}
static inline int atomicSub(int *addr, int val) {
    return __atomic_fetch_sub(addr, val, 0);
}
static inline unsigned int atomicSub(unsigned int *addr, unsigned int val) {
    return __atomic_fetch_sub(addr, val, 0);
}
static inline int atomicMin(int *addr, int val) {
    return __atomic_fetch_min(addr, val, 0);
}
static inline unsigned int atomicMin(unsigned int *addr, unsigned int val) {
    return __atomic_fetch_min(addr, val, 0);
}
static inline int atomicMax(int *addr, int val) {
    return __atomic_fetch_max(addr, val, 0);
}
static inline unsigned int atomicMax(unsigned int *addr, unsigned int val) {
    return __atomic_fetch_max(addr, val, 0);
}
static inline int atomicExch(int *addr, int val) {
    return __atomic_exchange_n(addr, val, 0);
}
static inline unsigned int atomicExch(unsigned int *addr, unsigned int val) {
    return __atomic_exchange_n(addr, val, 0);
}
static inline float atomicExch(float *addr, float val) {
    unsigned int ival;
    __builtin_memcpy(&ival, &val, sizeof(float));
    unsigned int old = __atomic_exchange_n((unsigned int *)addr, ival, 0);
    float res;
    __builtin_memcpy(&res, &old, sizeof(float));
    return res;
}
static inline unsigned int atomicCAS(unsigned int *addr, unsigned int cmp, unsigned int val) {
    __atomic_compare_exchange_n(addr, &cmp, val, 0, 0, 0);
    return cmp;
}
static inline int atomicOr(int *addr, int val) {
    return __atomic_fetch_or(addr, val, 0);
}
static inline unsigned int atomicOr(unsigned int *addr, unsigned int val) {
    return __atomic_fetch_or(addr, val, 0);
}
static inline int atomicAnd(int *addr, int val) {
    return __atomic_fetch_and(addr, val, 0);
}
static inline unsigned int atomicAnd(unsigned int *addr, unsigned int val) {
    return __atomic_fetch_and(addr, val, 0);
}
static inline int atomicXor(int *addr, int val) {
    return __atomic_fetch_xor(addr, val, 0);
}
static inline unsigned int atomicXor(unsigned int *addr, unsigned int val) {
    return __atomic_fetch_xor(addr, val, 0);
}

/* --- Math (clang provides these as builtins for AMDGPU) --- */
#define expf   __builtin_expf
#define sqrtf  __builtin_sqrtf
#define fabsf  __builtin_fabsf
#define floorf __builtin_floorf
#define ceilf  __builtin_ceilf
#define logf   __builtin_logf
#define powf   __builtin_powf
#define fminf  __builtin_fminf
#define fmaxf  __builtin_fmaxf
#define fmaf   __builtin_fmaf
#define rsqrtf(x) (1.0f / __builtin_sqrtf(x))
#define sinf   __builtin_sinf
#define cosf   __builtin_cosf
#define tanf   __builtin_tanf
#define log2f  __builtin_log2f
#define exp2f  __builtin_exp2f
#define truncf __builtin_truncf
#define roundf __builtin_roundf

/* tanhf: tanh(x) = (exp(2x)-1)/(exp(2x)+1) */
static __inline__ float tanhf(float x) {
    float e2x = __builtin_expf(2.0f * x);
    return (e2x - 1.0f) / (e2x + 1.0f);
}

/* sinhf/coshf */
static __inline__ float sinhf(float x) {
    float ex = __builtin_expf(x);
    return 0.5f * (ex - 1.0f / ex);
}
static __inline__ float coshf(float x) {
    float ex = __builtin_expf(x);
    return 0.5f * (ex + 1.0f / ex);
}

/* acosf/asinf/atanf/atan2f: software approximations (no device lib required) */
static __inline__ float acosf(float x) {
    /* Use identity: acos(x) = pi/2 - asin(x), asin(x) ~ x + x^3/6 + ... */
    /* Better: acos(x) = atan2(sqrt(1-x*x), x) using Newton-Raphson sqrt */
    float abs_x = x < 0.0f ? -x : x;
    float tmp = 1.0f - abs_x;
    if (tmp < 0.0f) tmp = 0.0f;
    float s = __builtin_sqrtf(tmp * (1.0f + abs_x));
    /* atan approximation for s/abs_x in [0,1] */
    float z = s;
    float r;
    if (abs_x < 1e-7f) {
        r = 1.5707963f; /* pi/2 */
    } else {
        float t = z / (abs_x + 1e-30f);
        /* Minimax atan approximation */
        float t2 = t * t;
        r = t * (1.5707963f + t2 * (-0.2145988f + t2 * 0.0889390f));
        /* atan(s/x) -> angle from x-axis */
        r = 2.0f * r; /* acos approximation via 2*atan(sqrt((1-x)/(1+x))) style */
    }
    if (x < 0.0f)
        return 3.14159265f - r;
    return r;
}

static __inline__ float asinf(float x) {
    return 1.5707963f - acosf(x);
}

static __inline__ float atanf(float x) {
    /* Simple minimax atan approximation */
    int neg = x < 0.0f;
    if (neg) x = -x;
    int inv = x > 1.0f;
    if (inv) x = 1.0f / x;
    float t = x;
    float t2 = t * t;
    float r = t * (1.0f + t2 * (-0.3333315f + t2 * (0.1999355f + t2 * (-0.1420888f + t2 * 0.1065626f))));
    if (inv) r = 1.5707963f - r;
    if (neg) r = -r;
    return r;
}

static __inline__ float atan2f(float y, float x) {
    if (x > 0.0f) return atanf(y / x);
    if (x < 0.0f && y >= 0.0f) return atanf(y / x) + 3.14159265f;
    if (x < 0.0f && y < 0.0f) return atanf(y / x) - 3.14159265f;
    if (y > 0.0f) return 1.5707963f;
    if (y < 0.0f) return -1.5707963f;
    return 0.0f;
}

#else
/* ================================================================
 * Host-side code
 * ================================================================ */

/* Stub thread/block variables for host-side compilation */
#ifndef hipThreadIdx_x
#define hipThreadIdx_x 0u
#define hipThreadIdx_y 0u
#define hipThreadIdx_z 0u
#define hipBlockIdx_x  0u
#define hipBlockIdx_y  0u
#define hipBlockIdx_z  0u
#define hipBlockDim_x  1u
#define hipBlockDim_y  1u
#define hipBlockDim_z  1u
#define hipGridDim_x   1u
#define hipGridDim_y   1u
#define hipGridDim_z   1u
#endif

static inline void __syncthreads(void) {}

/* Math functions use standard lib on host */
#include <math.h>

#endif /* __AMDGCN__ */

#endif /* HIP_RUNTIME_H */
