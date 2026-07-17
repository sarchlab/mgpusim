/*
 * OpenCL host harness that runs the same kernel mgpusim simulates
 * (amd/benchmarks/amdappsdk/matrixtranspose/native/MatrixTranspose_Kernels.cl)
 * on a real GPU (e.g. an H100) and reports wall-clock kernel time.
 *
 * Launch parameters mirror amd/benchmarks/amdappsdk/matrixtranspose/matrixtranspose.go
 * (blockSize=16, elemsPerThread1Dim=4) so the work-item/work-group geometry
 * matches mgpusim's own launch math for the same -width value.
 *
 * Usage:
 *   ./run_matrixtranspose <kernel.cl> <Width> [iterations]
 *
 * Width must match the mgpusim sample's -width flag, and must be a multiple
 * of 64 (block size 16 x elemsPerThread1Dim 4) for the kernel's tiling
 * assumptions to hold.
 */
#include <CL/cl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

static const int BLOCK_SIZE = 16;
static const int ELEMS_PER_THREAD = 4;

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
    if (argc < 3) {
        fprintf(stderr, "usage: %s <kernel.cl> <Width> [iterations]\n", argv[0]);
        return 1;
    }
    const char *kernel_path = argv[1];
    unsigned width = (unsigned)atoi(argv[2]);
    int iterations = argc > 3 ? atoi(argv[3]) : 10;

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
    cl_kernel kernel = clCreateKernel(program, "matrixTranspose", &err);
    check(err, "clCreateKernel");

    size_t numData = (size_t)width * width;
    size_t sizeBytes = numData * sizeof(float);

    float *hInput = malloc(sizeBytes);
    srand(0);
    for (size_t i = 0; i < numData; i++) hInput[i] = (float)rand() / RAND_MAX;

    cl_mem dInput = clCreateBuffer(ctx, CL_MEM_READ_ONLY | CL_MEM_COPY_HOST_PTR, sizeBytes, hInput, &err);
    check(err, "clCreateBuffer input");
    cl_mem dOutput = clCreateBuffer(ctx, CL_MEM_WRITE_ONLY, sizeBytes, NULL, &err);
    check(err, "clCreateBuffer output");

    /* matches matrixtranspose.go: wiWidth = wiHeight = Width / elemsPerThread1Dim */
    unsigned wiWidth = width / ELEMS_PER_THREAD;
    unsigned wiHeight = width / ELEMS_PER_THREAD;
    unsigned numWGWidth = wiWidth / BLOCK_SIZE;
    unsigned groupXOffset = 0;
    unsigned groupYOffset = 0;

    /* matches matrixtranspose.go: blockPtr size = blockSize^2 * elemsPerThread1Dim^2 * 4 bytes */
    size_t localBytes = (size_t)BLOCK_SIZE * BLOCK_SIZE * ELEMS_PER_THREAD * ELEMS_PER_THREAD * 4;

    clSetKernelArg(kernel, 0, sizeof(cl_mem), &dOutput);
    clSetKernelArg(kernel, 1, sizeof(cl_mem), &dInput);
    clSetKernelArg(kernel, 2, localBytes, NULL); /* __local block, sized by work-group */
    clSetKernelArg(kernel, 3, sizeof(unsigned), &wiWidth);
    clSetKernelArg(kernel, 4, sizeof(unsigned), &wiHeight);
    clSetKernelArg(kernel, 5, sizeof(unsigned), &numWGWidth);
    clSetKernelArg(kernel, 6, sizeof(unsigned), &groupXOffset);
    clSetKernelArg(kernel, 7, sizeof(unsigned), &groupYOffset);

    /* mgpusim's EnqueueLaunchKernel takes global size in work-items directly:
     * [gridSizeX, gridSizeY] = [wiWidth, wiHeight], local = [blockSize, blockSize] */
    size_t global[2] = { wiWidth, wiHeight };
    size_t local[2] = { (size_t)BLOCK_SIZE, (size_t)BLOCK_SIZE };

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

    printf("device=%s Width=%u iterations=%d avg_kernel_seconds=%.9f\n",
           name, width, iterations, avg);

    clReleaseMemObject(dInput);
    clReleaseMemObject(dOutput);
    clReleaseKernel(kernel);
    clReleaseProgram(program);
    clReleaseCommandQueue(queue);
    clReleaseContext(ctx);
    free(hInput); free(src);
    return 0;
}
