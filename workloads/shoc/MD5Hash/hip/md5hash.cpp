// md5hash.cpp -- SHOC MD5Hash benchmark (HIP)
// Brute-force MD5 hash computation.
// Each thread computes MD5 of its global thread ID.

#include "bench_common_hip.h"

// ---------------------------------------------------------------------------
// MD5 constants (stored in constant memory)
// ---------------------------------------------------------------------------
__constant__ unsigned int d_k[64] = {
    0xd76aa478, 0xe8c7b756, 0x242070db, 0xc1bdceee,
    0xf57c0faf, 0x4787c62a, 0xa8304613, 0xfd469501,
    0x698098d8, 0x8b44f7af, 0xffff5bb1, 0x895cd7be,
    0x6b901122, 0xfd987193, 0xa679438e, 0x49b40821,
    0xf61e2562, 0xc040b340, 0x265e5a51, 0xe9b6c7aa,
    0xd62f105d, 0x02441453, 0xd8a1e681, 0xe7d3fbc8,
    0x21e1cde6, 0xc33707d6, 0xf4d50d87, 0x455a14ed,
    0xa9e3e905, 0xfcefa3f8, 0x676f02d9, 0x8d2a4c8a,
    0xfffa3942, 0x8771f681, 0x6d9d6122, 0xfde5380c,
    0xa4beea44, 0x4bdecfa9, 0xf6bb4b60, 0xbebfbc70,
    0x289b7ec6, 0xeaa127fa, 0xd4ef3085, 0x04881d05,
    0xd9d4d039, 0xe6db99e5, 0x1fa27cf8, 0xc4ac5665,
    0xf4292244, 0x432aff97, 0xab9423a7, 0xfc93a039,
    0x655b59c3, 0x8f0ccc92, 0xffeff47d, 0x85845dd1,
    0x6fa87e4f, 0xfe2ce6e0, 0xa3014314, 0x4e0811a1,
    0xf7537e82, 0xbd3af235, 0x2ad7d2bb, 0xeb86d391
};

__constant__ unsigned int d_r[64] = {
    7,12,17,22, 7,12,17,22, 7,12,17,22, 7,12,17,22,
    5, 9,14,20, 5, 9,14,20, 5, 9,14,20, 5, 9,14,20,
    4,11,16,23, 4,11,16,23, 4,11,16,23, 4,11,16,23,
    6,10,15,21, 6,10,15,21, 6,10,15,21, 6,10,15,21
};

// ---------------------------------------------------------------------------
// Device function: left-rotate
// ---------------------------------------------------------------------------
__device__ __forceinline__ unsigned int leftrotate(unsigned int x,
                                                    unsigned int c) {
    return (x << c) | (x >> (32 - c));
}

// ---------------------------------------------------------------------------
// Kernel: compute MD5 of each thread's global ID
// ---------------------------------------------------------------------------
__global__ void md5hash_kernel(unsigned int* __restrict__ output, int N) {
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    if (idx >= N) return;

    // Message: 4-byte integer (the thread index)
    unsigned int msg = (unsigned int)idx;

    // Prepare 16-word block (512 bits) with MD5 padding
    unsigned int w[16];
    w[0] = msg;                    // message (4 bytes)
    w[1] = 0x80;                   // padding: 1-bit after message
    for (int i = 2; i < 14; ++i)
        w[i] = 0;
    w[14] = 32;                    // message length in bits (4 bytes = 32 bits)
    w[15] = 0;

    // Initial hash values
    unsigned int a0 = 0x67452301;
    unsigned int b0 = 0xefcdab89;
    unsigned int c0 = 0x98badcfe;
    unsigned int d0 = 0x10325476;

    unsigned int a = a0, b = b0, c = c0, d = d0;

    // 64 rounds
    for (int i = 0; i < 64; ++i) {
        unsigned int f, g;
        if (i < 16) {
            f = (b & c) | ((~b) & d);
            g = i;
        } else if (i < 32) {
            f = (d & b) | ((~d) & c);
            g = (5 * i + 1) % 16;
        } else if (i < 48) {
            f = b ^ c ^ d;
            g = (3 * i + 5) % 16;
        } else {
            f = c ^ (b | (~d));
            g = (7 * i) % 16;
        }

        unsigned int temp = d;
        d = c;
        c = b;
        b = b + leftrotate(a + f + d_k[i] + w[g], d_r[i]);
        a = temp;
    }

    a0 += a;
    b0 += b;
    c0 += c;
    d0 += d;

    // Store 128-bit hash (4 x uint32)
    output[idx * 4 + 0] = a0;
    output[idx * 4 + 1] = b0;
    output[idx * 4 + 2] = c0;
    output[idx * 4 + 3] = d0;
}

// ---------------------------------------------------------------------------
// CPU reference MD5 for verification
// ---------------------------------------------------------------------------
static const unsigned int h_k[64] = {
    0xd76aa478, 0xe8c7b756, 0x242070db, 0xc1bdceee,
    0xf57c0faf, 0x4787c62a, 0xa8304613, 0xfd469501,
    0x698098d8, 0x8b44f7af, 0xffff5bb1, 0x895cd7be,
    0x6b901122, 0xfd987193, 0xa679438e, 0x49b40821,
    0xf61e2562, 0xc040b340, 0x265e5a51, 0xe9b6c7aa,
    0xd62f105d, 0x02441453, 0xd8a1e681, 0xe7d3fbc8,
    0x21e1cde6, 0xc33707d6, 0xf4d50d87, 0x455a14ed,
    0xa9e3e905, 0xfcefa3f8, 0x676f02d9, 0x8d2a4c8a,
    0xfffa3942, 0x8771f681, 0x6d9d6122, 0xfde5380c,
    0xa4beea44, 0x4bdecfa9, 0xf6bb4b60, 0xbebfbc70,
    0x289b7ec6, 0xeaa127fa, 0xd4ef3085, 0x04881d05,
    0xd9d4d039, 0xe6db99e5, 0x1fa27cf8, 0xc4ac5665,
    0xf4292244, 0x432aff97, 0xab9423a7, 0xfc93a039,
    0x655b59c3, 0x8f0ccc92, 0xffeff47d, 0x85845dd1,
    0x6fa87e4f, 0xfe2ce6e0, 0xa3014314, 0x4e0811a1,
    0xf7537e82, 0xbd3af235, 0x2ad7d2bb, 0xeb86d391
};

static const unsigned int h_r[64] = {
    7,12,17,22, 7,12,17,22, 7,12,17,22, 7,12,17,22,
    5, 9,14,20, 5, 9,14,20, 5, 9,14,20, 5, 9,14,20,
    4,11,16,23, 4,11,16,23, 4,11,16,23, 4,11,16,23,
    6,10,15,21, 6,10,15,21, 6,10,15,21, 6,10,15,21
};

static unsigned int cpu_leftrotate(unsigned int x, unsigned int c) {
    return (x << c) | (x >> (32 - c));
}

static void md5_cpu(unsigned int idx, unsigned int* out) {
    unsigned int w[16];
    w[0] = idx;
    w[1] = 0x80;
    for (int i = 2; i < 14; ++i) w[i] = 0;
    w[14] = 32;
    w[15] = 0;

    unsigned int a0 = 0x67452301, b0 = 0xefcdab89;
    unsigned int c0 = 0x98badcfe, d0 = 0x10325476;
    unsigned int a = a0, b = b0, c = c0, d = d0;

    for (int i = 0; i < 64; ++i) {
        unsigned int f, g;
        if (i < 16) {
            f = (b & c) | ((~b) & d); g = i;
        } else if (i < 32) {
            f = (d & b) | ((~d) & c); g = (5*i+1)%16;
        } else if (i < 48) {
            f = b ^ c ^ d; g = (3*i+5)%16;
        } else {
            f = c ^ (b | (~d)); g = (7*i)%16;
        }
        unsigned int temp = d;
        d = c; c = b;
        b = b + cpu_leftrotate(a + f + h_k[i] + w[g], h_r[i]);
        a = temp;
    }
    out[0] = a0 + a;
    out[1] = b0 + b;
    out[2] = c0 + c;
    out[3] = d0 + d;
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int N          = parseIntParam(argc, argv, "--size", 1048576);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int iters      = parseIterations(argc, argv);

    size_t bytesOut = (size_t)N * 4 * sizeof(unsigned int);

    // Host allocations
    unsigned int* h_output = (unsigned int*)malloc(bytesOut);

    // Device allocations
    unsigned int* d_output;
    HIP_CHECK(hipMalloc(&d_output, bytesOut));

    int grid = (N + block_size - 1) / block_size;

    // Benchmark
    char size_str[64];
    snprintf(size_str, sizeof(size_str), "%d", N);

    printCSVHeader();

    BenchResult r = runBenchmark("md5hash", size_str, iters, [&]() {
        hipLaunchKernelGGL(md5hash_kernel, dim3(grid), dim3(block_size),
                           0, 0, d_output, N);
    });

    printCSVRow(r);

    // Throughput
    double hashes_per_sec = (double)N / (r.avg_ms * 1e-3);
    fprintf(stderr, "Throughput: %.2f MHash/s\n", hashes_per_sec / 1e6);

    // Verify first 1024 hashes
    int verify_count = (N < 1024) ? N : 1024;
    HIP_CHECK(hipMemcpy(h_output, d_output, bytesOut,
                         hipMemcpyDeviceToHost));

    int errors = 0;
    for (int i = 0; i < verify_count; ++i) {
        unsigned int ref[4];
        md5_cpu((unsigned int)i, ref);
        for (int j = 0; j < 4; ++j) {
            if (h_output[i * 4 + j] != ref[j]) {
                if (errors < 10) {
                    fprintf(stderr,
                            "Mismatch at hash %d word %d: got 0x%08x, "
                            "expected 0x%08x\n",
                            i, j, h_output[i * 4 + j], ref[j]);
                }
                errors++;
            }
        }
    }
    if (errors > 0) {
        fprintf(stderr, "FAIL: %d errors\n", errors);
    } else {
        fprintf(stderr, "PASS\n");
    }

    // Cleanup
    HIP_CHECK(hipFree(d_output));
    free(h_output);

    return (errors > 0) ? 1 : 0;
}
