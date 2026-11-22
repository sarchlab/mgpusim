#ifndef SHARED_MEM_PATTERN_H
#define SHARED_MEM_PATTERN_H

static_assert(sizeof(float) == 4, "the address model requires four-byte words");
static constexpr int SMEM_FLOATS = 12288;
static constexpr int SMEM_LANE_GROUP = 32;
static_assert(SMEM_FLOATS % (SMEM_LANE_GROUP * SMEM_LANE_GROUP) == 0,
              "shared array must contain complete transpose tiles");

#if defined(__CUDACC__) || defined(__HIPCC__)
#define SMEM_PATTERN_HD __host__ __device__
#else
#define SMEM_PATTERN_HD
#endif

// Transpose each 32x32 tile. This is a permutation of the entire shared array:
// every logical index retains a unique physical location. Adjacent lanes in
// each logical group of 32 have a stride of 32 words, without address aliasing.
SMEM_PATTERN_HD inline int smem_conflict_index(int logical)
{
    const int tile = SMEM_LANE_GROUP * SMEM_LANE_GROUP;
    return (logical / tile) * tile +
           (logical % SMEM_LANE_GROUP) * SMEM_LANE_GROUP +
           (logical % tile) / SMEM_LANE_GROUP;
}

#undef SMEM_PATTERN_HD
#endif
