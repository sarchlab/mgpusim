#include <hip/hip_runtime.h>

// Kernel 1: compute diffusion coefficients
__global__ void srad1(float *J, float *dN, float *dS, float *dW, float *dE,
                      float *c, int rows, int cols, float q0sqr) {
    int col = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int row = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (row >= rows || col >= cols) return;

    int idx = row * cols + col;

    // Neighbors with clamping at boundaries
    int iN = (row > 0) ? (row - 1) : 0;
    int iS = (row < rows - 1) ? (row + 1) : (rows - 1);
    int jW = (col > 0) ? (col - 1) : 0;
    int jE = (col < cols - 1) ? (col + 1) : (cols - 1);

    float Jc = J[idx];

    // Directional derivatives
    dN[idx] = J[iN * cols + col] - Jc;
    dS[idx] = J[iS * cols + col] - Jc;
    dW[idx] = J[row * cols + jW] - Jc;
    dE[idx] = J[row * cols + jE] - Jc;

    float G2 = (dN[idx] * dN[idx] + dS[idx] * dS[idx] +
                dW[idx] * dW[idx] + dE[idx] * dE[idx]) / (Jc * Jc);
    float L = (dN[idx] + dS[idx] + dW[idx] + dE[idx]) / Jc;
    float num = (0.5f * G2) - ((1.0f / 16.0f) * (L * L));
    float den = 1.0f + (0.25f * L);
    float qsqr = num / (den * den);

    // Diffusion coefficient (Perona-Malik)
    den = (qsqr - q0sqr) / (q0sqr * (1.0f + q0sqr));
    c[idx] = 1.0f / (1.0f + den);
    if (c[idx] < 0.0f) c[idx] = 0.0f;
    if (c[idx] > 1.0f) c[idx] = 1.0f;
}

// Kernel 2: update image using diffusion coefficients
__global__ void srad2(float *J, float *dN, float *dS, float *dW, float *dE,
                      float *c, int rows, int cols, float lambda) {
    int col = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int row = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (row >= rows || col >= cols) return;

    int idx = row * cols + col;

    // Neighbor diffusion coefficients
    int iS = (row < rows - 1) ? (row + 1) : (rows - 1);
    int jE = (col < cols - 1) ? (col + 1) : (cols - 1);

    float cN = c[idx];
    float cS = c[iS * cols + col];
    float cW = c[idx];
    float cE = c[row * cols + jE];

    // Divergence
    float D = cN * dN[idx] + cS * dS[idx] + cW * dW[idx] + cE * dE[idx];

    // Update
    J[idx] += 0.25f * lambda * D;
}
