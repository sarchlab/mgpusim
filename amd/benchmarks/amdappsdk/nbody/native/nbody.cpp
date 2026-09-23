/*
NBody HIP kernel for gfx942
*/

#include <hip/hip_runtime.h>

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
    float4 acc = (float4)(0.0f, 0.0f, 0.0f, 0.0f);

    for(int i = 0; i < numTiles; ++i)
    {
        // load one tile into local memory
        int idx = i * localSize + tid;
        localPos[tid] = pos[idx];

        // Synchronize to make sure data is available for processing
        __syncthreads();

        // calculate acceleration effect due to each body
        // a[i->j] = m[j] * r[i->j] / (r^2 + epsSqr)^(3/2)
        for(int j = 0; j < localSize; ++j)
        {
            // Calculate acceleartion caused by particle j on particle i
            float4 r = localPos[j] - myPos;
            float distSqr = r.x * r.x  +  r.y * r.y  +  r.z * r.z;
            float invDist = 1.0f / sqrt(distSqr + epsSqr);
            float invDistCube = invDist * invDist * invDist;
            float s = localPos[j].w * invDistCube;

            // accumulate effect of all particles
            acc += s * r;
        }

        // Synchronize so that next tile can be loaded
        __syncthreads();
    }

    float4 oldVel = vel[gid];

    // updated position and velocity
    float4 newPos = myPos + oldVel * deltaTime + acc * 0.5f * deltaTime * deltaTime;
    newPos.w = myPos.w;
    float4 newVel = oldVel + acc * deltaTime;

    // write to global memory
    newPosition[gid] = newPos;
    newVelocity[gid] = newVel;
}

int main() {
    return 0;
}
