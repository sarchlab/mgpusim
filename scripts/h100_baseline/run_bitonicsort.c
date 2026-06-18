/*
 * OpenCL host harness that runs the same kernel mgpusim simulates
 * (amd/benchmarks/amdappsdk/bitonicsort/kernels.cl, entry point
 * "BitonicSort") on a real GPU (e.g. an H100) and reports wall-clock kernel
 * time.
 *
 * Bitonic sort launches the kernel once per (stage, passOfStage) pair,
 * draining/syncing between launches since each pass depends on the
 * previous one's output -- see
 * amd/benchmarks/amdappsdk/bitonicsort/bitonicsort.go's exec()/runPass().
 * This harness replicates that exact launch sequence and sums the
 * wall-clock time of every launch, matching what mgpusim's Driver-level
 * kernel_time metric accumulates across the whole run.
 *
 * Usage:
 *   ./run_bitonicsort <kernel.cl> <Length> <OrderAscending 0|1> [iterations]
 *
 * Length must be a power of two and Length/2 must be a multiple of 64
 * (fixed work-group size) for the launch geometry to divide evenly --
 * matches mgpusim's default -length 1024.
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
    if (argc < 4) {
        fprintf(stderr, "usage: %s <kernel.cl> <Length> <OrderAscending 0|1> [iterations]\n", argv[0]);
        return 1;
    }
    const char *kernel_path = argv[1];
    unsigned length = (unsigned)atoi(argv[2]);
    unsigned direction = (unsigned)atoi(argv[3]);
    int iterations = argc > 4 ? atoi(argv[4]) : 10;

    unsigned numStages = 0;
    for (unsigned temp = length; temp > 1; temp >>= 1) numStages++;

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
    cl_kernel kernel = clCreateKernel(program, "BitonicSort", &err);
    check(err, "clCreateKernel");

    size_t numData = length;
    cl_uint *hArray = malloc(numData * sizeof(cl_uint));
    srand(0);
    for (size_t i = 0; i < numData; i++) hArray[i] = (cl_uint)rand();

    cl_mem dArray = clCreateBuffer(ctx, CL_MEM_READ_WRITE | CL_MEM_COPY_HOST_PTR,
                                    numData * sizeof(cl_uint), hArray, &err);
    check(err, "clCreateBuffer array");

    size_t global = length / 2;
    size_t local = 64;

    /* warm-up: run one pass and discard its timing */
    {
        cl_uint stage0 = 0, pass0 = 0;
        clSetKernelArg(kernel, 0, sizeof(cl_mem), &dArray);
        clSetKernelArg(kernel, 1, sizeof(cl_uint), &stage0);
        clSetKernelArg(kernel, 2, sizeof(cl_uint), &pass0);
        clSetKernelArg(kernel, 3, sizeof(cl_uint), &direction);
        clEnqueueNDRangeKernel(queue, kernel, 1, NULL, &global, &local, 0, NULL, NULL);
        clFinish(queue);
    }

    double total = 0;
    for (int it = 0; it < iterations; it++) {
        double iter_time = 0;
        for (cl_uint stage = 0; stage < numStages; stage++) {
            for (cl_uint pass = 0; pass <= stage; pass++) {
                clSetKernelArg(kernel, 0, sizeof(cl_mem), &dArray);
                clSetKernelArg(kernel, 1, sizeof(cl_uint), &stage);
                clSetKernelArg(kernel, 2, sizeof(cl_uint), &pass);
                clSetKernelArg(kernel, 3, sizeof(cl_uint), &direction);

                double t0 = now_seconds();
                cl_event ev;
                clEnqueueNDRangeKernel(queue, kernel, 1, NULL, &global, &local, 0, NULL, &ev);
                clWaitForEvents(1, &ev);
                double t1 = now_seconds();
                iter_time += (t1 - t0);
                clReleaseEvent(ev);
            }
        }
        total += iter_time;
    }
    double avg = total / iterations;

    printf("device=%s Length=%u OrderAscending=%u iterations=%d avg_kernel_seconds=%.9f\n",
           name, length, direction, iterations, avg);

    clReleaseMemObject(dArray);
    clReleaseKernel(kernel);
    clReleaseProgram(program);
    clReleaseCommandQueue(queue);
    clReleaseContext(ctx);
    free(hArray); free(src);
    return 0;
}
