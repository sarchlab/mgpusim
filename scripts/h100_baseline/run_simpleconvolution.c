/*
 * OpenCL host harness that runs the same kernel mgpusim simulates
 * (amd/benchmarks/amdappsdk/simpleconvolution/SimpleConvolution_Kernels.cl,
 * entry point "simpleNonSeparableConvolution") on a real GPU (e.g. an H100)
 * and reports wall-clock kernel time.
 *
 * Launch parameters mirror
 * amd/benchmarks/amdappsdk/simpleconvolution/simpleconvolution.go (work-group
 * size 64x1x1, 1D global size = (Width+padWidth)*(Height+padHeight)) so the
 * work-item/work-group geometry matches mgpusim's own launch math for the
 * same -width/-height/-mask-size values.
 *
 * Usage:
 *   ./run_simpleconvolution <kernel.cl> <Width> <Height> <MaskSize> [iterations]
 *
 * (Width+MaskSize-1)*(Height+MaskSize-1) must be a multiple of 64 for the
 * fixed work-group size of 64 to divide the global size evenly.
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
        fprintf(stderr, "usage: %s <kernel.cl> <Width> <Height> <MaskSize> [iterations]\n", argv[0]);
        return 1;
    }
    const char *kernel_path = argv[1];
    unsigned width = (unsigned)atoi(argv[2]);
    unsigned height = (unsigned)atoi(argv[3]);
    unsigned maskSize = (unsigned)atoi(argv[4]);
    int iterations = argc > 5 ? atoi(argv[5]) : 10;

    unsigned padWidth = maskSize - 1;
    unsigned padHeight = maskSize - 1;
    unsigned exWidth = width + padWidth;
    unsigned exHeight = height + padHeight;

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
    cl_kernel kernel = clCreateKernel(program, "simpleNonSeparableConvolution", &err);
    check(err, "clCreateKernel");

    size_t numInputData = (size_t)exWidth * exHeight;
    size_t numOutputData = (size_t)width * height;
    size_t numMaskData = (size_t)maskSize * maskSize;

    cl_uint *hInput = malloc(numInputData * sizeof(cl_uint));
    cl_float *hMask = malloc(numMaskData * sizeof(cl_float));
    for (size_t i = 0; i < numInputData; i++) hInput[i] = 1;
    for (size_t i = 0; i < numMaskData; i++) hMask[i] = 1.0f;

    cl_mem dInput = clCreateBuffer(ctx, CL_MEM_READ_ONLY | CL_MEM_COPY_HOST_PTR,
                                    numInputData * sizeof(cl_uint), hInput, &err);
    check(err, "clCreateBuffer input");
    cl_mem dMask = clCreateBuffer(ctx, CL_MEM_READ_ONLY | CL_MEM_COPY_HOST_PTR,
                                   numMaskData * sizeof(cl_float), hMask, &err);
    check(err, "clCreateBuffer mask");
    cl_mem dOutput = clCreateBuffer(ctx, CL_MEM_WRITE_ONLY, numOutputData * sizeof(cl_int), NULL, &err);
    check(err, "clCreateBuffer output");

    cl_uint2 inputDimensions = { { width, height } };
    cl_uint2 maskDimensions = { { maskSize, maskSize } };
    cl_uint nExWidth = exWidth;

    clSetKernelArg(kernel, 0, sizeof(cl_mem), &dInput);
    clSetKernelArg(kernel, 1, sizeof(cl_mem), &dMask);
    clSetKernelArg(kernel, 2, sizeof(cl_mem), &dOutput);
    clSetKernelArg(kernel, 3, sizeof(cl_uint2), &inputDimensions);
    clSetKernelArg(kernel, 4, sizeof(cl_uint2), &maskDimensions);
    clSetKernelArg(kernel, 5, sizeof(cl_uint), &nExWidth);

    /* matches simpleconvolution.go: gridSize = exWidth*exHeight, work-group 64x1x1 */
    size_t global = (size_t)exWidth * exHeight;
    size_t local = 64;

    /* warm-up */
    clEnqueueNDRangeKernel(queue, kernel, 1, NULL, &global, &local, 0, NULL, NULL);
    clFinish(queue);

    double total = 0;
    for (int i = 0; i < iterations; i++) {
        double t0 = now_seconds();
        cl_event ev;
        clEnqueueNDRangeKernel(queue, kernel, 1, NULL, &global, &local, 0, NULL, &ev);
        clWaitForEvents(1, &ev);
        double t1 = now_seconds();
        total += (t1 - t0);
        clReleaseEvent(ev);
    }
    double avg = total / iterations;

    printf("device=%s Width=%u Height=%u MaskSize=%u iterations=%d avg_kernel_seconds=%.9f\n",
           name, width, height, maskSize, iterations, avg);

    clReleaseMemObject(dInput);
    clReleaseMemObject(dMask);
    clReleaseMemObject(dOutput);
    clReleaseKernel(kernel);
    clReleaseProgram(program);
    clReleaseCommandQueue(queue);
    clReleaseContext(ctx);
    free(hInput); free(hMask); free(src);
    return 0;
}
