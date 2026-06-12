// cutcp.cpp -- HIP benchmark for Parboil Short-range Electrostatic Potential
// Kernel: Each thread computes the electrostatic potential at a grid point
// by summing contributions from nearby atoms within a cutoff distance
// (direct Coulomb summation with cutoff).

#include "bench_common_hip.h"

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

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_atoms = parseIntParam(argc, argv, "--num_atoms", 4096);
    int grid_size = parseIntParam(argc, argv, "--grid_size", 32);
    int cutoff_x10 = parseIntParam(argc, argv, "--cutoff_distance_x10", 12);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);

    DATA_TYPE cutoff = cutoff_x10 / 10.0f;
    DATA_TYPE cutoff2 = cutoff * cutoff;
    DATA_TYPE spacing = 0.5f;  // grid spacing in Angstroms
    int total_points = grid_size * grid_size * grid_size;

    // Determine the extent of the grid for placing atoms
    DATA_TYPE grid_extent = (grid_size - 1) * spacing;

    // Allocate host memory
    Atom *h_atoms = (Atom *)malloc(num_atoms * sizeof(Atom));
    DATA_TYPE *h_potential =
        (DATA_TYPE *)malloc(total_points * sizeof(DATA_TYPE));

    // Initialize atoms randomly within the grid
    srand(42);
    for (int i = 0; i < num_atoms; i++) {
        h_atoms[i].x = ((DATA_TYPE)(rand() % 10000) / 10000.0f) * grid_extent;
        h_atoms[i].y = ((DATA_TYPE)(rand() % 10000) / 10000.0f) * grid_extent;
        h_atoms[i].z = ((DATA_TYPE)(rand() % 10000) / 10000.0f) * grid_extent;
        h_atoms[i].charge =
            ((DATA_TYPE)(rand() % 2000) / 1000.0f) - 1.0f; // [-1, 1]
    }

    // Initialize potential grid to zero
    for (int i = 0; i < total_points; i++)
        h_potential[i] = 0.0f;

    // Device allocations
    Atom *d_atoms;
    DATA_TYPE *d_potential;
    HIP_CHECK(hipMalloc(&d_atoms, num_atoms * sizeof(Atom)));
    HIP_CHECK(hipMalloc(&d_potential, total_points * sizeof(DATA_TYPE)));

    HIP_CHECK(hipMemcpy(d_atoms, h_atoms, num_atoms * sizeof(Atom),
                        hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_potential, h_potential,
                        total_points * sizeof(DATA_TYPE),
                        hipMemcpyHostToDevice));

    // Kernel launch config
    dim3 block(block_size);
    dim3 grid((total_points + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_atoms);

    printCSVHeader();

    BenchResult r1 = runBenchmark(
        "cutoff_potential", problemSize, iterations, [&]() {
            // Reset potential each iteration for consistent results
            (void)hipMemset(d_potential, 0, total_points * sizeof(DATA_TYPE));
            cutoff_potential<<<grid, block>>>(d_potential, d_atoms, num_atoms,
                                              grid_size, spacing, cutoff2);
        });
    printCSVRow(r1);

    // Cleanup
    HIP_CHECK(hipFree(d_atoms));
    HIP_CHECK(hipFree(d_potential));
    free(h_atoms);
    free(h_potential);

    return 0;
}
