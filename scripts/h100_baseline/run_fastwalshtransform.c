/*
 * OpenCL host harness that runs the same kernel mgpusim simulates
 * (amd/benchmarks/amdappsdk/fastwalshtransform/native/FastWalshTransform_Kernels.cl,
 * entry point "fastWalshTransform") on a real GPU (e.g. an H100) and
 * reports wall-clock kernel time.
 *
 * The benchmark enqueues one kernel launch per butterfly "step" (step =
 * 1,2,4,...,Length/2) into a single in-order queue with only one drain at
 * the very end -- see
 * amd/benchmarks/amdappsdk/fastwalshtransform/fastwalshtransform.go's
 * exec(). This harness replicates that exact back-to-back-enqueue/single-
 * wait pattern (work-group size 256, global size Length/2).
 *
 * Usage:
 *   ./run_fastwalshtransform <kernel.cl> <Length> [iterations]
 *
 * Length must be a power of two and Length/2 must be a multiple of 256
 * (fixed work-group size) -- matches mgpusim's default -length 1024.
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
    if (argc < 3) {
        fprintf(stderr, "usage: %s <kernel.cl> <Length> [iterations]\n", argv[0]);
        return 1;
    }
    const char *kernel_path = argv[1];
    unsigned length = (unsigned)atoi(argv[2]);
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
    cl_kernel kernel = clCreateKernel(program, "fastWalshTransform", &err);
    check(err, "clCreateKernel");

    size_t numData = length;
    cl_float *hArray = malloc(numData * sizeof(cl_float));
    srand(123);
    for (size_t i = 0; i < numData; i++) {
        hArray[i] = (float)rand() / RAND_MAX + (float)(rand() % 255);
    }

    cl_mem dArray = clCreateBuffer(ctx, CL_MEM_READ_WRITE | CL_MEM_COPY_HOST_PTR,
                                    numData * sizeof(cl_float), hArray, &err);
    check(err, "clCreateBuffer array");

    size_t global = length / 2;
    size_t local = 256;

    /* warm-up */
    {
        cl_int step0 = 1;
        clSetKernelArg(kernel, 0, sizeof(cl_mem), &dArray);
        clSetKernelArg(kernel, 1, sizeof(cl_int), &step0);
        clEnqueueNDRangeKernel(queue, kernel, 1, NULL, &global, &local, 0, NULL, NULL);
        clFinish(queue);
    }

    double total = 0;
    for (int it = 0; it < iterations; it++) {
        double t0 = now_seconds();
        cl_event ev;
        for (cl_int step = 1; (unsigned)step < length; step <<= 1) {
            clSetKernelArg(kernel, 0, sizeof(cl_mem), &dArray);
            clSetKernelArg(kernel, 1, sizeof(cl_int), &step);
            clEnqueueNDRangeKernel(queue, kernel, 1, NULL, &global, &local, 0, NULL, &ev);
        }
        clWaitForEvents(1, &ev);
        double t1 = now_seconds();
        total += (t1 - t0);
        clReleaseEvent(ev);
    }
    double avg = total / iterations;

    printf("device=%s Length=%u iterations=%d avg_kernel_seconds=%.9f\n",
           name, length, iterations, avg);

    clReleaseMemObject(dArray);
    clReleaseKernel(kernel);
    clReleaseProgram(program);
    clReleaseCommandQueue(queue);
    clReleaseContext(ctx);
    free(hArray); free(src);
    return 0;
}
