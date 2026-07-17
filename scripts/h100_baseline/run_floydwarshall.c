/*
 * OpenCL host harness that runs the same kernel mgpusim simulates
 * (amd/benchmarks/amdappsdk/floydwarshall/native/FloydWarshall_Kernels.cl,
 * entry point "floydWarshallPass") on a real GPU (e.g. an H100) and reports
 * wall-clock kernel time.
 *
 * Floyd-Warshall launches the kernel once per "pass" (one per intermediate
 * node k), each pass depending on the previous one's output -- see
 * amd/benchmarks/amdappsdk/floydwarshall/floydwarshall.go's exec()/
 * launchKernelIteration(). This harness replicates that exact launch
 * sequence (NumIterations launches, block size 8x8) and sums the
 * wall-clock time of every launch, matching what mgpusim's Driver-level
 * kernel_time metric accumulates across the whole run.
 *
 * Usage:
 *   ./run_floydwarshall <kernel.cl> <NumNodes> <NumIterations> [iterations]
 *
 * NumIterations=0 means "use NumNodes" (matches mgpusim's -iter 0 default).
 * NumNodes must be a multiple of 8 (fixed 8x8 work-group size) -- matches
 * mgpusim's default -node 16.
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
        fprintf(stderr, "usage: %s <kernel.cl> <NumNodes> <NumIterations> [iterations]\n", argv[0]);
        return 1;
    }
    const char *kernel_path = argv[1];
    unsigned numNodes = (unsigned)atoi(argv[2]);
    unsigned numIterations = argc > 3 ? (unsigned)atoi(argv[3]) : 0;
    int outerIterations = argc > 4 ? atoi(argv[4]) : 10;

    if (numIterations == 0 || numIterations > numNodes) numIterations = numNodes;

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
    cl_kernel kernel = clCreateKernel(program, "floydWarshallPass", &err);
    check(err, "clCreateKernel");

    size_t numData = (size_t)numNodes * numNodes;
    cl_uint *hDist = malloc(numData * sizeof(cl_uint));
    cl_uint *hPath = malloc(numData * sizeof(cl_uint));
    srand(1);
    for (unsigned i = 0; i < numNodes; i++) {
        for (unsigned j = 0; j < i; j++) {
            cl_uint w = (cl_uint)(rand() % 10);
            hDist[i * numNodes + j] = w;
            hDist[j * numNodes + i] = w;
        }
        hDist[i * numNodes + i] = 0;
    }
    for (unsigned i = 0; i < numNodes; i++) {
        for (unsigned j = 0; j < i; j++) {
            hPath[i * numNodes + j] = i;
            hPath[j * numNodes + i] = j;
        }
        hPath[i * numNodes + i] = i;
    }

    cl_mem dDist = clCreateBuffer(ctx, CL_MEM_READ_WRITE | CL_MEM_COPY_HOST_PTR,
                                   numData * sizeof(cl_uint), hDist, &err);
    check(err, "clCreateBuffer dist");
    cl_mem dPath = clCreateBuffer(ctx, CL_MEM_READ_WRITE | CL_MEM_COPY_HOST_PTR,
                                   numData * sizeof(cl_uint), hPath, &err);
    check(err, "clCreateBuffer path");

    size_t global[2] = { numNodes, numNodes };
    size_t local[2] = { 8, 8 };

    /* warm-up */
    {
        cl_uint pass0 = 0;
        clSetKernelArg(kernel, 0, sizeof(cl_mem), &dDist);
        clSetKernelArg(kernel, 1, sizeof(cl_mem), &dPath);
        clSetKernelArg(kernel, 2, sizeof(cl_uint), &numNodes);
        clSetKernelArg(kernel, 3, sizeof(cl_uint), &pass0);
        clEnqueueNDRangeKernel(queue, kernel, 2, NULL, global, local, 0, NULL, NULL);
        clFinish(queue);
    }

    double total = 0;
    for (int it = 0; it < outerIterations; it++) {
        double iter_time = 0;
        for (cl_uint pass = 0; pass < numIterations; pass++) {
            clSetKernelArg(kernel, 0, sizeof(cl_mem), &dDist);
            clSetKernelArg(kernel, 1, sizeof(cl_mem), &dPath);
            clSetKernelArg(kernel, 2, sizeof(cl_uint), &numNodes);
            clSetKernelArg(kernel, 3, sizeof(cl_uint), &pass);

            double t0 = now_seconds();
            cl_event ev;
            clEnqueueNDRangeKernel(queue, kernel, 2, NULL, global, local, 0, NULL, &ev);
            clWaitForEvents(1, &ev);
            double t1 = now_seconds();
            iter_time += (t1 - t0);
            clReleaseEvent(ev);
        }
        total += iter_time;
    }
    double avg = total / outerIterations;

    printf("device=%s NumNodes=%u NumIterations=%u iterations=%d avg_kernel_seconds=%.9f\n",
           name, numNodes, numIterations, outerIterations, avg);

    clReleaseMemObject(dDist);
    clReleaseMemObject(dPath);
    clReleaseKernel(kernel);
    clReleaseProgram(program);
    clReleaseCommandQueue(queue);
    clReleaseContext(ctx);
    free(hDist); free(hPath); free(src);
    return 0;
}
