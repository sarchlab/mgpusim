// nbody.cpp — HIP benchmark for N-body gravitational simulation
// Kernel copied from amd/benchmarks/amdappsdk/nbody/native/nbody.cpp
// Problem size: 1024 particles

#include "bench_common.h"

extern "C" __global__
void nbody_sim(
    float4* pos,
    float4* vel,
    int numBodies,
    float deltaTime,
    float epsSqr,
    float4* localPos,
    float4* newPosition,
    float4* newVelocity)
{
    unsigned int tid = hipThreadIdx_x;
    unsigned int gid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    unsigned int localSize = hipBlockDim_x;

    // Number of tiles we need to iterate
    unsigned int numTiles = numBodies / localSize;

    // position of this work-item
    float4 myPos = pos[gid];
    float4 acc = make_float4(0.0f, 0.0f, 0.0f, 0.0f);

    for(int i = 0; i < (int)numTiles; ++i)
    {
        // load one tile into local memory
        int idx = i * localSize + tid;
        localPos[tid] = pos[idx];

        // Synchronize to make sure data is available for processing
        __syncthreads();

        // calculate acceleration effect due to each body
        for(unsigned int j = 0; j < localSize; ++j)
        {
            // Calculate acceleration caused by particle j on particle i
            float4 r;
            r.x = localPos[j].x - myPos.x;
            r.y = localPos[j].y - myPos.y;
            r.z = localPos[j].z - myPos.z;
            r.w = 0.0f;
            float distSqr = r.x * r.x + r.y * r.y + r.z * r.z;
            float invDist = 1.0f / sqrtf(distSqr + epsSqr);
            float invDistCube = invDist * invDist * invDist;
            float s = localPos[j].w * invDistCube;

            // accumulate effect of all particles
            acc.x += s * r.x;
            acc.y += s * r.y;
            acc.z += s * r.z;
        }

        // Synchronize so that next tile can be loaded
        __syncthreads();
    }

    float4 oldVel = vel[gid];

    // updated position and velocity
    float4 newPos;
    newPos.x = myPos.x + oldVel.x * deltaTime + acc.x * 0.5f * deltaTime * deltaTime;
    newPos.y = myPos.y + oldVel.y * deltaTime + acc.y * 0.5f * deltaTime * deltaTime;
    newPos.z = myPos.z + oldVel.z * deltaTime + acc.z * 0.5f * deltaTime * deltaTime;
    newPos.w = myPos.w;

    float4 newVel;
    newVel.x = oldVel.x + acc.x * deltaTime;
    newVel.y = oldVel.y + acc.y * deltaTime;
    newVel.z = oldVel.z + acc.z * deltaTime;
    newVel.w = oldVel.w;

    // write to global memory
    newPosition[gid] = newPos;
    newVelocity[gid] = newVel;
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    // NUM_BODIES must be a multiple of THREADS (256)
    int NUM_BODIES = parseIntParam(argc, argv, "--bodies", 1024);
    const float DELTA_TIME = 0.005f;
    const float EPS_SQR = 50.0f;

    // Host allocations
    std::vector<float4> h_pos(NUM_BODIES);
    std::vector<float4> h_vel(NUM_BODIES);

    srand(42);
    for (int i = 0; i < NUM_BODIES; i++) {
        h_pos[i].x = (float)(rand() % 1000) / 500.0f - 1.0f;
        h_pos[i].y = (float)(rand() % 1000) / 500.0f - 1.0f;
        h_pos[i].z = (float)(rand() % 1000) / 500.0f - 1.0f;
        h_pos[i].w = 1.0f; // mass

        h_vel[i].x = 0.0f;
        h_vel[i].y = 0.0f;
        h_vel[i].z = 0.0f;
        h_vel[i].w = 0.0f;
    }

    // Device allocations
    float4 *d_pos, *d_vel, *d_newPos, *d_newVel, *d_localPos;
    HIP_CHECK(hipMalloc(&d_pos, NUM_BODIES * sizeof(float4)));
    HIP_CHECK(hipMalloc(&d_vel, NUM_BODIES * sizeof(float4)));
    HIP_CHECK(hipMalloc(&d_newPos, NUM_BODIES * sizeof(float4)));
    HIP_CHECK(hipMalloc(&d_newVel, NUM_BODIES * sizeof(float4)));

    // localPos is used as shared memory via __shared__ in original, but the native
    // kernel passes it as a parameter. Allocate per-block shared memory buffer.
    // Each block needs localSize float4s. We allocate for all blocks.
    const int THREADS = 256;
    int numBlocks = (NUM_BODIES + THREADS - 1) / THREADS;
    HIP_CHECK(hipMalloc(&d_localPos, numBlocks * THREADS * sizeof(float4)));

    HIP_CHECK(hipMemcpy(d_pos, h_pos.data(), NUM_BODIES * sizeof(float4), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_vel, h_vel.data(), NUM_BODIES * sizeof(float4), hipMemcpyHostToDevice));

    dim3 block(THREADS);
    dim3 grid(numBlocks);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d_particles", NUM_BODIES);

    BenchResult r = runBenchmark("nbody", problemSize, iterations, [&]() {
        nbody_sim<<<grid, block>>>(
            d_pos, d_vel, NUM_BODIES, DELTA_TIME, EPS_SQR,
            d_localPos, d_newPos, d_newVel);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_pos));
    HIP_CHECK(hipFree(d_vel));
    HIP_CHECK(hipFree(d_newPos));
    HIP_CHECK(hipFree(d_newVel));
    HIP_CHECK(hipFree(d_localPos));

    return 0;
}
