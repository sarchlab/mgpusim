/*
 * OpenCL host harness that runs the same kernel mgpusim simulates
 * (amd/benchmarks/amdappsdk/matrixmultiplication/MatrixMultiplication_Kernels.cl)
 * on a real GPU (e.g. an H100) and reports wall-clock kernel time.
 *
 * Usage:
 *   ./run_matrixmultiplication <kernel.cl> <X> <Y> <Z> [iterations]
 *
 * X, Y, Z must match the dims passed to the mgpusim sample's -x -y -z flags
 * (A is XxY, B is YxZ, C is XxZ) and must be multiples of 32 (local size 8 x
 * TILE 4) for the kernel's tiling assumptions to hold.
 */
#include <CL/cl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

static char *read_file(const char *path, size_t *out_len) {
    FILE *f = fopen(path, "rb");
    if (!f) { perror("fopen"); exit(1); }
    fseek(f, 0, SEEK_END);
    long len = ftell(f);
    fseek(f, 0, SEEK_SET);
    char *buf = malloc((size_t)len + 1);
    fread(buf, 1, (size_t)len, f);
    buf[len] = '\0';
    fclose(f);
    if (out_len) *out_len = (size_t)len;
    return buf;
}

static void check(cl_int err, const char *msg) {
    if (err != CL_SUCCESS) {
        fprintf(stderr, "%s failed: %d\n", msg, err);
        exit(1);
    }
}

static double now_seconds(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (double)ts.tv_sec + (double)ts.tv_nsec * 1e-9;
}

int main(int argc, char **argv) {
    if (argc < 5) {
        fprintf(stderr, "usage: %s <kernel.cl> <X> <Y> <Z> [iterations]\n", argv[0]);
        return 1;
    }
    const char *kernel_path = argv[1];
    unsigned X = (unsigned)atoi(argv[2]); /* height of A, height of C */
    unsigned Y = (unsigned)atoi(argv[3]); /* width of A, height of B */
    unsigned Z = (unsigned)atoi(argv[4]); /* width of B, width of C */
    int iterations = argc > 5 ? atoi(argv[5]) : 10;

    cl_platform_id platform;
    cl_device_id device;
    cl_int err;

    check(clGetPlatformIDs(1, &platform, NULL), "clGetPlatformIDs");
    check(clGetDeviceIDs(platform, CL_DEVICE_TYPE_GPU, 1, &device, NULL), "clGetDeviceIDs");

    char name[256];
    clGetDeviceInfo(device, CL_DEVICE_NAME, sizeof(name), name, NULL);
    fprintf(stderr, "Running on device: %s\n", name);

    cl_context ctx = clCreateContext(NULL, 1, &device, NULL, NULL, &err);
    check(err, "clCreateContext");
    cl_command_queue queue = clCreateCommandQueueWithProperties(ctx, device, 0, &err);
    check(err, "clCreateCommandQueue");

    size_t src_len;
    char *src = read_file(kernel_path, &src_len);
    cl_program program = clCreateProgramWithSource(ctx, 1, (const char **)&src, &src_len, &err);
    check(err, "clCreateProgramWithSource");
    err = clBuildProgram(program, 1, &device, "-cl-std=CL1.2", NULL, NULL);
    if (err != CL_SUCCESS) {
        size_t log_size;
        clGetProgramBuildInfo(program, device, CL_PROGRAM_BUILD_LOG, 0, NULL, &log_size);
        char *log = malloc(log_size + 1);
        clGetProgramBuildInfo(program, device, CL_PROGRAM_BUILD_LOG, log_size, log, NULL);
        log[log_size] = '\0';
        fprintf(stderr, "Build log:\n%s\n", log);
        return 1;
    }
    cl_kernel kernel = clCreateKernel(program, "mmmKernel_local", &err);
    check(err, "clCreateKernel");

    size_t sizeA = (size_t)X * Y * sizeof(float);
    size_t sizeB = (size_t)Y * Z * sizeof(float);
    size_t sizeC = (size_t)X * Z * sizeof(float);

    float *hA = malloc(sizeA), *hB = malloc(sizeB);
    srand(0);
    for (size_t i = 0; i < (size_t)X * Y; i++) hA[i] = (float)rand() / RAND_MAX;
    for (size_t i = 0; i < (size_t)Y * Z; i++) hB[i] = (float)rand() / RAND_MAX;

    cl_mem dA = clCreateBuffer(ctx, CL_MEM_READ_ONLY | CL_MEM_COPY_HOST_PTR, sizeA, hA, &err);
    check(err, "clCreateBuffer A");
    cl_mem dB = clCreateBuffer(ctx, CL_MEM_READ_ONLY | CL_MEM_COPY_HOST_PTR, sizeB, hB, &err);
    check(err, "clCreateBuffer B");
    cl_mem dC = clCreateBuffer(ctx, CL_MEM_WRITE_ONLY, sizeC, NULL, &err);
    check(err, "clCreateBuffer C");

    int widthA = (int)Y; /* kernel's "widthA" = inner dimension, matches mgpusim's mA.Width */
    cl_mem blockA = clCreateBuffer(ctx, CL_MEM_READ_WRITE, 32 * 32 * sizeof(float), NULL, &err);
    check(err, "clCreateBuffer blockA");

    clSetKernelArg(kernel, 0, sizeof(cl_mem), &dA);
    clSetKernelArg(kernel, 1, sizeof(cl_mem), &dB);
    clSetKernelArg(kernel, 2, sizeof(cl_mem), &dC);
    clSetKernelArg(kernel, 3, sizeof(int), &widthA);
    clSetKernelArg(kernel, 4, 32 * 32 * sizeof(float), NULL); /* __local blockA, sized by work-group */

    size_t global[2] = { Z / 4, X / 4 };
    size_t local[2] = { 8, 8 };

    /* warm-up */
    clEnqueueNDRangeKernel(queue, kernel, 2, NULL, global, local, 0, NULL, NULL);
    clFinish(queue);

    double total = 0;
    for (int i = 0; i < iterations; i++) {
        double t0 = now_seconds();
        cl_event ev;
        clEnqueueNDRangeKernel(queue, kernel, 2, NULL, global, local, 0, NULL, &ev);
        clWaitForEvents(1, &ev);
        double t1 = now_seconds();
        total += (t1 - t0);
        clReleaseEvent(ev);
    }
    double avg = total / iterations;

    printf("device=%s X=%u Y=%u Z=%u iterations=%d avg_kernel_seconds=%.9f\n",
           name, X, Y, Z, iterations, avg);

    clReleaseMemObject(dA);
    clReleaseMemObject(dB);
    clReleaseMemObject(dC);
    clReleaseMemObject(blockA);
    clReleaseKernel(kernel);
    clReleaseProgram(program);
    clReleaseCommandQueue(queue);
    clReleaseContext(ctx);
    free(hA); free(hB); free(src);
    return 0;
}
