/*
 * HIP kernel for the Altis ray tracing benchmark (gfx942 / CDNA3).
 * Extracted from sarchlab/gpu_benchmarks tier2/altis_raytracing.
 *
 * Casts one ray per pixel through an image plane and intersects each ray
 * with num_spheres spheres, shading the nearest hit with Phong lighting.
 * One thread per pixel.
 *
 * Uses a constant BLOCK_SIZE (not blockDim.x) for the block geometry so the
 * compiler emits no hidden ABI arguments (smaller, simpler kernarg).
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 256

struct Sphere {
    float cx, cy, cz;  // center
    float radius;
    float r, g, b;     // color
};

__device__ static float intersect_sphere(
    float ox, float oy, float oz,   // ray origin
    float dx, float dy, float dz,   // ray direction (normalized)
    float cx, float cy, float cz,   // sphere center
    float radius)
{
    float ex = ox - cx;
    float ey = oy - cy;
    float ez = oz - cz;

    float a = dx * dx + dy * dy + dz * dz;
    float b = 2.0f * (ex * dx + ey * dy + ez * dz);
    float c = ex * ex + ey * ey + ez * ez - radius * radius;

    float disc = b * b - 4.0f * a * c;
    if (disc < 0.0f) return -1.0f;

    float sq = sqrtf(disc);
    float t0 = (-b - sq) / (2.0f * a);
    float t1 = (-b + sq) / (2.0f * a);

    if (t0 > 0.001f) return t0;
    if (t1 > 0.001f) return t1;
    return -1.0f;
}

extern "C" __global__ void raytrace_kernel(
    unsigned char* __restrict__ image,
    const Sphere* __restrict__ spheres,
    int width, int height, int num_spheres)
{
    int idx = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    int total_pixels = width * height;
    if (idx >= total_pixels) return;

    int px = idx % width;
    int py = idx / width;

    // Camera setup: look along -Z, image plane at z = -1
    float aspect = (float)width / (float)height;
    float fov_scale = 1.0f;  // tan(45 deg) ~ 1

    // Map pixel to normalized device coords [-1, 1]
    float u = (2.0f * ((float)px + 0.5f) / (float)width - 1.0f) * aspect * fov_scale;
    float v = (1.0f - 2.0f * ((float)py + 0.5f) / (float)height) * fov_scale;

    // Ray origin and direction
    float ox = 0.0f, oy = 0.0f, oz = 5.0f;
    float dx = u, dy = v, dz = -1.0f;

    // Normalize direction
    float len = sqrtf(dx * dx + dy * dy + dz * dz);
    dx /= len; dy /= len; dz /= len;

    // Find closest intersection
    float closest_t = 1e20f;
    int closest_id = -1;

    for (int s = 0; s < num_spheres; ++s) {
        float t = intersect_sphere(ox, oy, oz, dx, dy, dz,
                                   spheres[s].cx, spheres[s].cy, spheres[s].cz,
                                   spheres[s].radius);
        if (t > 0.0f && t < closest_t) {
            closest_t = t;
            closest_id = s;
        }
    }

    // Shade pixel
    float pr = 0.05f, pg = 0.05f, pb = 0.1f;  // background

    if (closest_id >= 0) {
        // Hit point
        float hx = ox + closest_t * dx;
        float hy = oy + closest_t * dy;
        float hz = oz + closest_t * dz;

        // Normal at hit point
        const Sphere& sp = spheres[closest_id];
        float nx = (hx - sp.cx) / sp.radius;
        float ny = (hy - sp.cy) / sp.radius;
        float nz = (hz - sp.cz) / sp.radius;

        // Light direction (normalized, from upper-right)
        float lx = 0.577f, ly = 0.577f, lz = 0.577f;

        // Diffuse (Lambertian)
        float ndotl = nx * lx + ny * ly + nz * lz;
        if (ndotl < 0.0f) ndotl = 0.0f;

        // Specular (Phong)
        // Reflect light around normal: R = 2(N.L)N - L
        float rx = 2.0f * ndotl * nx - lx;
        float ry = 2.0f * ndotl * ny - ly;
        float rz = 2.0f * ndotl * nz - lz;

        // View direction (from hit to camera)
        float vx = -dx, vy = -dy, vz = -dz;
        float rdotv = rx * vx + ry * vy + rz * vz;
        if (rdotv < 0.0f) rdotv = 0.0f;
        float spec = rdotv * rdotv * rdotv * rdotv;  // specular exponent ~4
        spec = spec * spec;  // exponent ~8

        float ambient = 0.15f;
        pr = sp.r * (ambient + 0.7f * ndotl) + 0.3f * spec;
        pg = sp.g * (ambient + 0.7f * ndotl) + 0.3f * spec;
        pb = sp.b * (ambient + 0.7f * ndotl) + 0.3f * spec;

        // Clamp
        if (pr > 1.0f) pr = 1.0f;
        if (pg > 1.0f) pg = 1.0f;
        if (pb > 1.0f) pb = 1.0f;
    }

    // Write RGBA
    int base = idx * 4;
    image[base + 0] = (unsigned char)(pr * 255.0f);
    image[base + 1] = (unsigned char)(pg * 255.0f);
    image[base + 2] = (unsigned char)(pb * 255.0f);
    image[base + 3] = 255;
}
