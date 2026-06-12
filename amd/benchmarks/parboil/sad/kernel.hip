#include <hip/hip_runtime.h>

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
