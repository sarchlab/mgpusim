// tpacf.cpp -- HIP benchmark for Parboil Two-Point Angular Correlation Function
// For each pair of points from a sky catalog, compute the angular separation
// and bin it into a histogram. Uses acos(sin*sin + cos*cos*cos) for angular
// distance.

#include "bench_common_hip.h"
#include <math.h>

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

typedef float DATA_TYPE;

// Kernel: compute angular separations and bin into histogram
// Each thread handles one point i and compares against all points j > i
__global__ void tpacf_kernel(DATA_TYPE *ra, DATA_TYPE *dec,
                             unsigned long long *hist,
                             int num_points, int num_bins,
                             DATA_TYPE min_angle, DATA_TYPE bin_width) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (i < num_points) {
        DATA_TYPE ra_i = ra[i];
        DATA_TYPE dec_i = dec[i];
        DATA_TYPE sin_dec_i = sinf(dec_i);
        DATA_TYPE cos_dec_i = cosf(dec_i);

        for (int j = i + 1; j < num_points; j++) {
            DATA_TYPE ra_j = ra[j];
            DATA_TYPE dec_j = dec[j];

            // Angular separation using spherical law of cosines
            DATA_TYPE cos_angle = sin_dec_i * sinf(dec_j) +
                                  cos_dec_i * cosf(dec_j) * cosf(ra_i - ra_j);

            // Clamp to [-1, 1] to avoid NaN from acos
            if (cos_angle > 1.0f) cos_angle = 1.0f;
            if (cos_angle < -1.0f) cos_angle = -1.0f;

            DATA_TYPE angle = acosf(cos_angle);

            // Bin the angle
            int bin = (int)((angle - min_angle) / bin_width);
            if (bin >= 0 && bin < num_bins) {
                atomicAdd(&hist[bin], 1ULL);
            }
        }
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_points = parseIntParam(argc, argv, "--num_points", 4096);
    int num_bins = parseIntParam(argc, argv, "--num_bins", 64);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int num_random_catalogs = parseIntParam(argc, argv, "--num_random_catalogs", 3);

    DATA_TYPE min_angle = 0.0f;
    DATA_TYPE max_angle = (DATA_TYPE)M_PI;
    DATA_TYPE bin_width = (max_angle - min_angle) / num_bins;

    // Allocate host memory for data catalog (RA and Dec in radians)
    DATA_TYPE *h_data_ra = (DATA_TYPE *)malloc(num_points * sizeof(DATA_TYPE));
    DATA_TYPE *h_data_dec = (DATA_TYPE *)malloc(num_points * sizeof(DATA_TYPE));

    // Allocate host memory for random catalogs
    DATA_TYPE **h_rand_ra = (DATA_TYPE **)malloc(num_random_catalogs * sizeof(DATA_TYPE *));
    DATA_TYPE **h_rand_dec = (DATA_TYPE **)malloc(num_random_catalogs * sizeof(DATA_TYPE *));
    for (int c = 0; c < num_random_catalogs; c++) {
        h_rand_ra[c] = (DATA_TYPE *)malloc(num_points * sizeof(DATA_TYPE));
        h_rand_dec[c] = (DATA_TYPE *)malloc(num_points * sizeof(DATA_TYPE));
    }

    // Host histograms
    unsigned long long *h_DD = (unsigned long long *)calloc(num_bins, sizeof(unsigned long long));
    unsigned long long *h_DR = (unsigned long long *)calloc(num_bins, sizeof(unsigned long long));
    unsigned long long *h_RR = (unsigned long long *)calloc(num_bins, sizeof(unsigned long long));

    // Initialize data programmatically
    srand(42);
    for (int i = 0; i < num_points; i++) {
        // RA in [0, 2*pi), Dec in [-pi/2, pi/2]
        h_data_ra[i] = (DATA_TYPE)(rand() % 10000) / 10000.0f * 2.0f * (DATA_TYPE)M_PI;
        h_data_dec[i] = (DATA_TYPE)(rand() % 10000) / 10000.0f * (DATA_TYPE)M_PI - (DATA_TYPE)(M_PI / 2.0);
    }
    for (int c = 0; c < num_random_catalogs; c++) {
        for (int i = 0; i < num_points; i++) {
            h_rand_ra[c][i] = (DATA_TYPE)(rand() % 10000) / 10000.0f * 2.0f * (DATA_TYPE)M_PI;
            h_rand_dec[c][i] = (DATA_TYPE)(rand() % 10000) / 10000.0f * (DATA_TYPE)M_PI - (DATA_TYPE)(M_PI / 2.0);
        }
    }

    // Device allocations
    DATA_TYPE *d_ra, *d_dec;
    unsigned long long *d_hist;
    HIP_CHECK(hipMalloc(&d_ra, num_points * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_dec, num_points * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_hist, num_bins * sizeof(unsigned long long)));

    // Kernel launch config
    dim3 block(block_size);
    dim3 grid((num_points + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_points);

    printCSVHeader();

    // Benchmark DD (data-data) correlation
    HIP_CHECK(hipMemcpy(d_ra, h_data_ra, num_points * sizeof(DATA_TYPE),
                        hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_dec, h_data_dec, num_points * sizeof(DATA_TYPE),
                        hipMemcpyHostToDevice));

    BenchResult r1 = runBenchmark(
        "tpacf_DD", problemSize, iterations, [&]() {
            HIP_CHECK(hipMemset(d_hist, 0, num_bins * sizeof(unsigned long long)));
            tpacf_kernel<<<grid, block>>>(d_ra, d_dec, d_hist,
                                          num_points, num_bins,
                                          min_angle, bin_width);
        });
    printCSVRow(r1);

    // Benchmark DR (data-random) correlation
    // Use first random catalog for DR
    HIP_CHECK(hipMemcpy(d_ra, h_rand_ra[0], num_points * sizeof(DATA_TYPE),
                        hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_dec, h_rand_dec[0], num_points * sizeof(DATA_TYPE),
                        hipMemcpyHostToDevice));

    BenchResult r2 = runBenchmark(
        "tpacf_DR", problemSize, iterations, [&]() {
            HIP_CHECK(hipMemset(d_hist, 0, num_bins * sizeof(unsigned long long)));
            tpacf_kernel<<<grid, block>>>(d_ra, d_dec, d_hist,
                                          num_points, num_bins,
                                          min_angle, bin_width);
        });
    printCSVRow(r2);

    // Benchmark RR (random-random) correlation
    BenchResult r3 = runBenchmark(
        "tpacf_RR", problemSize, iterations, [&]() {
            HIP_CHECK(hipMemset(d_hist, 0, num_bins * sizeof(unsigned long long)));
            tpacf_kernel<<<grid, block>>>(d_ra, d_dec, d_hist,
                                          num_points, num_bins,
                                          min_angle, bin_width);
        });
    printCSVRow(r3);

    // Cleanup
    HIP_CHECK(hipFree(d_ra));
    HIP_CHECK(hipFree(d_dec));
    HIP_CHECK(hipFree(d_hist));
    free(h_data_ra);
    free(h_data_dec);
    for (int c = 0; c < num_random_catalogs; c++) {
        free(h_rand_ra[c]);
        free(h_rand_dec[c]);
    }
    free(h_rand_ra);
    free(h_rand_dec);
    free(h_DD);
    free(h_DR);
    free(h_RR);

    return 0;
}
