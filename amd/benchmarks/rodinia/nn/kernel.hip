#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

struct LatLong {
    DATA_TYPE lat;
    DATA_TYPE lng;
};

__global__ void nn_kernel(LatLong *d_locations, DATA_TYPE *d_distances,
                          int num_records, DATA_TYPE query_lat,
                          DATA_TYPE query_lng) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (tid < num_records) {
        DATA_TYPE dlat = d_locations[tid].lat - query_lat;
        DATA_TYPE dlng = d_locations[tid].lng - query_lng;
        d_distances[tid] = sqrtf(dlat * dlat + dlng * dlng);
    }
}
