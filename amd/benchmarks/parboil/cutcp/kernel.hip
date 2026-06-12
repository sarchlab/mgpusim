#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

struct Atom {
    DATA_TYPE x, y, z, charge;
};

// Kernel: compute potential at each grid point from all atoms within cutoff
__global__ void cutoff_potential(DATA_TYPE *potential, Atom *atoms,
                                  int num_atoms, int grid_size,
                                  DATA_TYPE spacing, DATA_TYPE cutoff2) {
    int gid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int total_points = grid_size * grid_size * grid_size;
    if (gid >= total_points) return;

    // Convert linear index to 3D grid coordinates
    int iz = gid / (grid_size * grid_size);
    int iy = (gid / grid_size) % grid_size;
    int ix = gid % grid_size;

    DATA_TYPE gx = ix * spacing;
    DATA_TYPE gy = iy * spacing;
    DATA_TYPE gz = iz * spacing;

    DATA_TYPE energy = 0.0f;

    for (int a = 0; a < num_atoms; a++) {
        DATA_TYPE dx = gx - atoms[a].x;
        DATA_TYPE dy = gy - atoms[a].y;
        DATA_TYPE dz = gz - atoms[a].z;
        DATA_TYPE dist2 = dx * dx + dy * dy + dz * dz;
        if (dist2 < cutoff2 && dist2 > 1e-12f) {
            energy += atoms[a].charge * rsqrtf(dist2);
        }
    }

    potential[gid] += energy;
}
