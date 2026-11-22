// GPU Benchmarks v1.1, tier2/parboil_cutcp/hip/parboil_cutcp.hip.
// Derived from Parboil benchmarks/cutcp/src/cuda/cutoff6overlap.cu
// at a4c63da28f45699e9a274cbc49af3e067162aa2b.
// (C) Copyright 2008-2010 The Board of Trustees of the University of Illinois.
// All Rights Reserved. See UPSTREAM_LICENSE for the Illinois Open Source License.
#include <hip/hip_runtime.h>

#define NBRLIST_DIM  11
#define NBRLIST_MAXLEN (NBRLIST_DIM * NBRLIST_DIM * NBRLIST_DIM)
__constant__ int NbrListLen;
__constant__ int3 NbrList[NBRLIST_MAXLEN];

#undef NEIGHBOR_COUNT
#ifndef NEIGHBOR_COUNT
typedef float ener_t;
#else
typedef int ener_t;
#endif

// BIN_DEPTH is the fixed per-bin atom capacity. The original Parboil ran at
// BIN_DEPTH=8 and spilled the (Poisson) tail of over-full bins to a separate
// CPU pass. This port does not reproduce that CPU spill pass, so bins must be
// deep enough to hold the tail at the design density (~7 atoms/bin): a Poisson
// mean of ~7 reaches ~15 in the tail, so depth 8 would drop ~13% of atoms at
// the default config. Depth 16 holds the full design-density problem without
// dropping any atom; any config that still overflows is rejected before
// measurement (see the binning loop). BIN_SIZE = BIN_DEPTH*4 floats/bin and
// BIN_SHIFT = log2(BIN_SIZE) must track BIN_DEPTH (issue #63, C-02 / D-02(b)).
#define BIN_DEPTH        16  /* max number of atoms per bin */
#define BIN_SIZE         64  /* size of bin in floats (BIN_DEPTH * 4) */
#define BIN_SHIFT         6  /* # of bits to shift for mul/div by BIN_SIZE */
#define BIN_CACHE_MAXLEN 32  /* max number of atom bins to cache */

#define BIN_LENGTH      4.f  /* spatial length in Angstroms */
#define BIN_INVLEN  (1.f / BIN_LENGTH)

#define REGION_SIZE     512  /* number of floats in lattice region */

// ---------------------------------------------------------------------------
// Kernel — cutoff6overlap.cu with thread-local region metadata
// ---------------------------------------------------------------------------
__global__ static void cuda_cutoff_potential_lattice6overlap(
    int binDim_x,
    int binDim_y,
    float4 *binZeroAddr,    /* address of atom bins starting at origin */
    float h,                /* lattice spacing */
    float cutoff2,          /* square of cutoff distance */
    float inv_cutoff2,
    ener_t *regionZeroAddr, /* address of lattice regions starting at origin */
    int zRegionIndex
    )
{
  __shared__ float AtomBinCache[BIN_CACHE_MAXLEN * BIN_DEPTH * 4];
  // Every thread computes these values; they require no shared ownership.
  ener_t *myRegionAddr;
  int3 myBinIndex;

  const int xRegionIndex = blockIdx.x;
  const int yRegionIndex = blockIdx.y;

  /* thread id */
  const int tid = (threadIdx.z*blockDim.y + threadIdx.y)*blockDim.x
    + threadIdx.x;

  /* neighbor index */
  int nbrid;

  /* this is the start of the sub-region indexed by tid */
  myRegionAddr = regionZeroAddr + ((zRegionIndex*gridDim.y
        + yRegionIndex)*gridDim.x + xRegionIndex)*REGION_SIZE;

  /* spatial coordinate of this lattice point */
  float x = (8 * xRegionIndex + threadIdx.x) * h;
  float y = (8 * yRegionIndex + threadIdx.y) * h;
  float z = (8 * zRegionIndex + threadIdx.z) * h;

  int totalbins = 0;
  int numbins;

  /* bin number determined by center of region */
  myBinIndex.x = (int) floorf((8 * xRegionIndex + 4) * h * BIN_INVLEN);
  myBinIndex.y = (int) floorf((8 * yRegionIndex + 4) * h * BIN_INVLEN);
  myBinIndex.z = (int) floorf((8 * zRegionIndex + 4) * h * BIN_INVLEN);

  /* first neighbor in list for me to cache */
  nbrid = (tid >> 4);

  numbins = BIN_CACHE_MAXLEN;

#ifndef NEIGHBOR_COUNT
  ener_t energy0 = 0.f;
  ener_t energy1 = 0.f;
  ener_t energy2 = 0.f;
  ener_t energy3 = 0.f;
#else
  ener_t energy0 = 0, energy1 = 0, energy2 = 0, energy3 = 0;
#endif

  for (totalbins = 0;  totalbins < NbrListLen;  totalbins += numbins) {
    int bincnt;

    /* start of where to write in shared memory */
    int startoff = BIN_SIZE * (tid >> 4);

    /* each half-warp to cache up to 4 atom bins */
    for (bincnt = 0;  bincnt < 4 && nbrid < NbrListLen;  bincnt++, nbrid += 8) {
      int i = myBinIndex.x + NbrList[nbrid].x;
      int j = myBinIndex.y + NbrList[nbrid].y;
      int k = myBinIndex.z + NbrList[nbrid].z;

      /* determine global memory location of atom bin */
      float *p_global = ((float *) binZeroAddr)
        + (((__mul24(k, binDim_y) + j)*binDim_x + i) << BIN_SHIFT);

      /* coalesced read from global memory -
       * retain same ordering in shared memory for now */
      int binIndex = startoff + (bincnt << (3 + BIN_SHIFT));
      int tidmask = tid & 15;

      /* A half-warp (16 lanes) stages the whole bin: BIN_SIZE floats total, so
       * BIN_SIZE/16 floats per lane. The original hardcoded two copies (32
       * floats = BIN_DEPTH 8); this loop tracks BIN_SIZE so deeper bins
       * (BIN_DEPTH 16 -> 64 floats) are fully staged (issue #63, C-02). */
      for (int f = tidmask; f < BIN_SIZE; f += 16)
        AtomBinCache[binIndex + f] = p_global[f];
    }
    __syncthreads();

    /* no warp divergence */
    if (totalbins + BIN_CACHE_MAXLEN > NbrListLen) {
      numbins = NbrListLen - totalbins;
    }

    int stopbin = (numbins << BIN_SHIFT);
    for (bincnt = 0; bincnt < stopbin; bincnt+=BIN_SIZE) {
      int i;

      for (i = 0;  i < BIN_DEPTH;  i++) {
        int off = bincnt + (i<<2);

        float aq = AtomBinCache[off + 3];
        if (0.f == aq)
          break;  /* no more atoms in bin */

        float dx = AtomBinCache[off    ] - x;
        float dz = AtomBinCache[off + 2] - z;
        float dxdz2 = dx*dx + dz*dz;
        float dy = AtomBinCache[off + 1] - y;
        float r2 = dy*dy + dxdz2;

#ifndef NEIGHBOR_COUNT
        if (r2 < cutoff2)
	  {
          float s = (1.f - r2 * inv_cutoff2);
          energy0 += aq * rsqrtf(r2) * s * s;
        }
#else
	energy0 += (r2 < cutoff2);
#endif
        dy -= 2.0f*h;
        r2 = dy*dy + dxdz2;
#ifndef NEIGHBOR_COUNT
	if (r2 < cutoff2)
	  {
          float s = (1.f - r2 * inv_cutoff2);
          energy1 += aq * rsqrtf(r2) * s * s;
        }
#else
	energy1 += (r2 < cutoff2);
#endif
        dy -= 2.0f*h;
        r2 = dy*dy + dxdz2;
#ifndef NEIGHBOR_COUNT
        if (r2 < cutoff2)
	  {
          float s = (1.f - r2 * inv_cutoff2);
          energy2 += aq * rsqrtf(r2) * s * s;
        }
#else
	energy2 += (r2 < cutoff2);
#endif
        dy -= 2.0f*h;
        r2 = dy*dy + dxdz2;
#ifndef NEIGHBOR_COUNT
        if (r2 < cutoff2)
	  {
          float s = (1.f - r2 * inv_cutoff2);
          energy3 += aq * rsqrtf(r2) * s * s;
        }
#else
	energy3 += (r2 < cutoff2);
#endif
      } /* end loop over atoms in bin */
    } /* end loop over cached atom bins */
    __syncthreads();

  } /* end loop over neighbor list */

  /* store into global memory */
  myRegionAddr[(tid>>4)*64 + (tid&15)     ] = energy0;
  myRegionAddr[(tid>>4)*64 + (tid&15) + 16] = energy1;
  myRegionAddr[(tid>>4)*64 + (tid&15) + 32] = energy2;
  myRegionAddr[(tid>>4)*64 + (tid&15) + 48] = energy3;
}
