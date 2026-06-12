// sad.cpp -- HIP benchmark for Parboil SAD (Sum of Absolute Differences)
// Motion estimation for video encoding: for each macroblock in the reference
// frame, find the best matching candidate block in the search window by
// computing SAD. Each thread handles one candidate position.

#include "bench_common_hip.h"

// Kernel: compute SAD for each candidate position in the search window
// for a single reference macroblock. Each thread computes one SAD value.
__global__ void ComputeSAD(unsigned char *ref_frame, unsigned char *cur_frame,
                           int frame_width, int block_size_sad,
                           int search_range, int *sad_out,
                           int num_mb_x, int num_mb_y) {
    // Each block handles one reference macroblock
    int mb_idx = hipBlockIdx_x;
    int mb_y = mb_idx / num_mb_x;
    int mb_x = mb_idx - mb_y * num_mb_x;

    // Each thread handles one candidate position in the search window
    int search_dim = 2 * search_range + 1;
    int total_candidates = search_dim * search_dim;
    int tid = hipThreadIdx_x;

    // Reference macroblock top-left corner
    int ref_x = mb_x * block_size_sad;
    int ref_y = mb_y * block_size_sad;

    for (int c = tid; c < total_candidates; c += hipBlockDim_x) {
        int dy = c / search_dim - search_range;
        int dx = c - (c / search_dim) * search_dim - search_range;

        int cand_x = ref_x + dx;
        int cand_y = ref_y + dy;

        int sad = 0;

        // Compute SAD between reference block and candidate block
        for (int by = 0; by < block_size_sad; by++) {
            for (int bx = 0; bx < block_size_sad; bx++) {
                int ry = ref_y + by;
                int rx = ref_x + bx;
                int cy = cand_y + by;
                int cx = cand_x + bx;

                // Bounds checking
                unsigned char ref_val = 0;
                unsigned char cur_val = 0;
                if (ry >= 0 && ry < frame_width && rx >= 0 && rx < frame_width)
                    ref_val = ref_frame[ry * frame_width + rx];
                if (cy >= 0 && cy < frame_width && cx >= 0 && cx < frame_width)
                    cur_val = cur_frame[cy * frame_width + cx];

                int diff = (int)ref_val - (int)cur_val;
                sad += (diff < 0) ? -diff : diff;
            }
        }

        // Store SAD value
        sad_out[mb_idx * total_candidates + c] = sad;
    }
}

// Kernel: find minimum SAD and its position for each macroblock
__global__ void FindMinSAD(int *sad_values, int *min_sad, int *min_pos,
                           int total_candidates, int num_macroblocks) {
    int mb = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (mb >= num_macroblocks) return;

    int best_sad = sad_values[mb * total_candidates];
    int best_pos = 0;

    for (int c = 1; c < total_candidates; c++) {
        int val = sad_values[mb * total_candidates + c];
        if (val < best_sad) {
            best_sad = val;
            best_pos = c;
        }
    }

    min_sad[mb] = best_sad;
    min_pos[mb] = best_pos;
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int frame_width = parseIntParam(argc, argv, "--frame_width", 256);
    int block_size_sad = parseIntParam(argc, argv, "--block_size_sad", 16);
    int search_range = parseIntParam(argc, argv, "--search_range", 16);
    int num_threads_per_block = parseIntParam(argc, argv,
                                              "--num_threads_per_block", 256);

    int num_mb_x = frame_width / block_size_sad;
    int num_mb_y = frame_width / block_size_sad;
    int num_macroblocks = num_mb_x * num_mb_y;

    int search_dim = 2 * search_range + 1;
    int total_candidates = search_dim * search_dim;

    int frame_size = frame_width * frame_width;

    // Allocate host memory
    unsigned char *h_ref = (unsigned char *)malloc(frame_size);
    unsigned char *h_cur = (unsigned char *)malloc(frame_size);
    int *h_min_sad = (int *)malloc(num_macroblocks * sizeof(int));
    int *h_min_pos = (int *)malloc(num_macroblocks * sizeof(int));

    // Initialize data programmatically
    srand(42);
    for (int i = 0; i < frame_size; i++) {
        h_ref[i] = (unsigned char)(rand() % 256);
    }
    // Current frame is a slightly modified version of reference
    for (int i = 0; i < frame_size; i++) {
        int val = (int)h_ref[i] + (rand() % 21) - 10;
        if (val < 0) val = 0;
        if (val > 255) val = 255;
        h_cur[i] = (unsigned char)val;
    }

    // Device allocations
    unsigned char *d_ref, *d_cur;
    int *d_sad_out, *d_min_sad, *d_min_pos;

    HIP_CHECK(hipMalloc(&d_ref, frame_size));
    HIP_CHECK(hipMalloc(&d_cur, frame_size));
    HIP_CHECK(hipMalloc(&d_sad_out,
        (long long)num_macroblocks * total_candidates * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_min_sad, num_macroblocks * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_min_pos, num_macroblocks * sizeof(int)));

    HIP_CHECK(hipMemcpy(d_ref, h_ref, frame_size,
                        hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_cur, h_cur, frame_size,
                        hipMemcpyHostToDevice));

    // Kernel launch configs
    // ComputeSAD: one block per macroblock
    dim3 grid_sad(num_macroblocks);
    dim3 block_sad(num_threads_per_block);

    // FindMinSAD: standard 1D launch
    dim3 block_min(256);
    dim3 grid_min((num_macroblocks + 255) / 256);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", frame_width);

    printCSVHeader();

    // Benchmark ComputeSAD
    BenchResult r1 = runBenchmark(
        "ComputeSAD", problemSize, iterations, [&]() {
            ComputeSAD<<<grid_sad, block_sad>>>(d_ref, d_cur,
                                                 frame_width, block_size_sad,
                                                 search_range, d_sad_out,
                                                 num_mb_x, num_mb_y);
        });
    printCSVRow(r1);

    // Benchmark FindMinSAD
    BenchResult r2 = runBenchmark(
        "FindMinSAD", problemSize, iterations, [&]() {
            FindMinSAD<<<grid_min, block_min>>>(d_sad_out, d_min_sad, d_min_pos,
                                                 total_candidates,
                                                 num_macroblocks);
        });
    printCSVRow(r2);

    // Cleanup
    HIP_CHECK(hipFree(d_ref));
    HIP_CHECK(hipFree(d_cur));
    HIP_CHECK(hipFree(d_sad_out));
    HIP_CHECK(hipFree(d_min_sad));
    HIP_CHECK(hipFree(d_min_pos));
    free(h_ref);
    free(h_cur);
    free(h_min_sad);
    free(h_min_pos);

    return 0;
}
