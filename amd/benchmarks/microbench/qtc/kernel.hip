#include <hip/hip_runtime.h>

// QTC (Quality Threshold Clustering): compute pairwise distances and count
// neighbors within a threshold distance for each point.
// Each thread handles one point, computes distances to all other points,
// and counts how many are within the threshold.
__global__ void qtc_kernel(
    const float* __restrict__ X,
    const float* __restrict__ Y,
    int* __restrict__ counts,
    int N,
    float threshold) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid >= N) return;

    float xi = X[gid];
    float yi = Y[gid];
    float thresh2 = threshold * threshold;

    int count = 0;
    for (int j = 0; j < N; j++) {
        if (j == gid) continue;
        float dx = xi - X[j];
        float dy = yi - Y[j];
        float dist2 = dx * dx + dy * dy;
        if (dist2 <= thresh2) {
            count++;
        }
    }

    counts[gid] = count;
}
