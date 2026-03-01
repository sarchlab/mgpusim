#include <hip/hip_runtime.h>

// define types based on compiler "command line"
#define VALTYPE float
#define LROWS 16

__device__ inline int ToGlobalRow(int gidRow, int lszRow, int lidRow) {
  // assumes coordinates and dimensions are logical (without halo)
  // returns logical global row (without halo)
  return gidRow * lszRow + lidRow;
}

__device__ inline int ToGlobalCol(int gidCol, int lszCol, int lidCol) {
  // assumes coordinates and dimensions are logical (without halo)
  // returns logical global column (without halo)
  return gidCol * lszCol + lidCol;
}

__device__ inline int ToFlatHaloedIdx(int row, int col, int rowPitch) {
  // assumes input coordinates and dimensions are logical (without halo)
  // and a halo of width 1
  return (row + 1) * (rowPitch + 2) + (col + 1);
}

__device__ inline int ToFlatIdx(int row, int col, int pitch) { return row * pitch + col; }

extern "C" __global__ void CopyRect(VALTYPE* dest, int doffset, int dpitch,
                       VALTYPE* src, int soffset, int spitch,
                       int width, int height) {
  int gid = hipBlockIdx_x;
  int lid = hipThreadIdx_x;
  int gsz = hipBlockDim_x * hipGridDim_x;
  int lsz = hipBlockDim_x;
  int grow = gid * lsz + lid;

  if (grow < height) {
    for (int c = 0; c < width; c++) {
      (dest + doffset)[ToFlatIdx(grow, c, dpitch)] =
          (src + soffset)[ToFlatIdx(grow, c, spitch)];
    }
  }
}

extern "C" __global__ void StencilKernel(VALTYPE* data, VALTYPE* newData,
                            const int alignment, VALTYPE wCenter,
                            VALTYPE wCardinal, VALTYPE wDiagonal) {
  // Declare shared memory inside the kernel
  // Size: (LROWS+2) * (64+2) = 18 * 66 = 1188 floats
  __shared__ VALTYPE sh[(LROWS+2)*(64+2)];

  // determine our location in the HIP coordinate system
  // To match with the row-major ordering used to store the 2D
  // array in both the host and on the device, we use:
  //   dimension 0 == rows,
  //   dimension 1 == columns
  int gidRow = hipBlockIdx_x;
  int gidCol = hipBlockIdx_y;
  int gszRow = hipGridDim_x;
  int gszCol = hipGridDim_y;
  int lidRow = hipThreadIdx_x;
  int lidCol = hipThreadIdx_y;
  int lszRow = LROWS;
  int lszCol = hipBlockDim_y;

  // determine our logical global data coordinates (without halo)
  int gRow = ToGlobalRow(gidRow, lszRow, lidRow);
  int gCol = ToGlobalCol(gidCol, lszCol, lidCol);

  // determine pitch of rows (without halo)
  int nCols = gszCol * lszCol + 2;  // num columns including halo
  int nPaddedCols =
      nCols +
      (((nCols % alignment) == 0) ? 0 : (alignment - (nCols % alignment)));
  int gRowWidth = nPaddedCols - 2;  // remove the halo

  // Copy my global data item to a shared local buffer.
  // We assume it is large enough to hold all the data computed by
  // our block, plus a halo of width 1.
  int lRowWidth = lszCol;  // logical, not haloed
  for (int i = 0; i < (lszRow + 2); i++) {
    int lidx = ToFlatHaloedIdx(lidRow - 1 + i, lidCol, lRowWidth);
    int gidx = ToFlatHaloedIdx(gRow - 1 + i, gCol, gRowWidth);
    sh[lidx] = data[gidx];
  }

  // Copy the "left" and "right" halo rows into our local memory buffer.
  // Only two threads are involved (first column and last column).
  if (lidCol == 0) {
    for (int i = 0; i < (lszRow + 2); i++) {
      int lidx = ToFlatHaloedIdx(lidRow - 1 + i, lidCol - 1, lRowWidth);
      int gidx = ToFlatHaloedIdx(gRow - 1 + i, gCol - 1, gRowWidth);
      sh[lidx] = data[gidx];
    }
  } else if (lidCol == (lszCol - 1)) {
    for (int i = 0; i < (lszRow + 2); i++) {
      int lidx = ToFlatHaloedIdx(lidRow - 1 + i, lidCol + 1, lRowWidth);
      int gidx = ToFlatHaloedIdx(gRow - 1 + i, gCol + 1, gRowWidth);
      sh[lidx] = data[gidx];
    }
  }

  // let all those loads finish
  __syncthreads();

  // do my part of the smoothing operation
  for (int i = 0; i < lszRow; i++) {
    int cidx = ToFlatHaloedIdx(lidRow + i, lidCol, lRowWidth);
    int nidx = ToFlatHaloedIdx(lidRow - 1 + i, lidCol, lRowWidth);
    int sidx = ToFlatHaloedIdx(lidRow + 1 + i, lidCol, lRowWidth);
    int eidx = ToFlatHaloedIdx(lidRow + i, lidCol + 1, lRowWidth);
    int widx = ToFlatHaloedIdx(lidRow + i, lidCol - 1, lRowWidth);
    int neidx = ToFlatHaloedIdx(lidRow - 1 + i, lidCol + 1, lRowWidth);
    int seidx = ToFlatHaloedIdx(lidRow + 1 + i, lidCol + 1, lRowWidth);
    int nwidx = ToFlatHaloedIdx(lidRow - 1 + i, lidCol - 1, lRowWidth);
    int swidx = ToFlatHaloedIdx(lidRow + 1 + i, lidCol - 1, lRowWidth);

    VALTYPE centerValue = sh[cidx];
    VALTYPE cardinalValueSum = sh[nidx] + sh[sidx] + sh[eidx] + sh[widx];
    VALTYPE diagonalValueSum = sh[neidx] + sh[seidx] + sh[nwidx] + sh[swidx];

    newData[ToFlatHaloedIdx(gRow + i, gCol, gRowWidth)] =
        wCenter * centerValue + wCardinal * cardinalValueSum +
        wDiagonal * diagonalValueSum;
  }
}
