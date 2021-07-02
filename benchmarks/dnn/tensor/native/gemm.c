#include <CL/opencl.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <time.h>

uint64_t getTimeInNSecs(){
    struct timespec time;
    clock_gettime(CLOCK_MONOTONIC, &time);
    uint64_t timeInSec = time.tv_sec * 1e9 + time.tv_nsec;
    return timeInSec;
}

float f32abs(float a) {
  if (a < 0) {
    return -a;
  }

  return a;
}

char *read_file(const char *filename) {
  char *buffer = 0;
  long length;
  FILE *f = fopen(filename, "rb");

  if (f) {
    fseek(f, 0, SEEK_END);
    length = ftell(f);
    fseek(f, 0, SEEK_SET);
    buffer = (char *)(malloc(length));
    if (buffer) {
      fread(buffer, 1, length, f);
    }
    fclose(f);
  }

  return buffer;
}

// m = 17, n = 11, k = 9
// alpha * a * b + beta * c
// alpha = 3.1, beta = 4.2
// d = numpy.matmul(a, b) * alpha + beta * c

int main(int argc, char *argv[]) {
  int m = 1024;
  int n = 1024;
  int k = 1024;
  float alpha = 3.1;
  float beta = 4.2;
  
  float *h_input_a;
  float *h_input_b;
  float *h_input_c;
  float *cpu_out;
  float *gpu_out;

  h_input_a = malloc(m * k * sizeof(float));
  h_input_b = malloc(n * k * sizeof(float));
  h_input_c = malloc(m * n * sizeof(float));
  cpu_out = malloc(m * n * sizeof(float));
  gpu_out = malloc(m * n * sizeof(float));

  for (int i = 0; i < m * k; i++) {
    h_input_a[i] = (float)rand() / (float)RAND_MAX;
  }

  for (int i = 0; i < n * k; i++) {
    h_input_b[i] = (float)rand() / (float)RAND_MAX;
  }

   for (int i = 0; i < m * n; i++) {
    h_input_c[i] = (float)rand() / (float)RAND_MAX;
  }

  for (int x = 0; x < n; x++) {
    for (int y = 0; y < m; y++) {
      float sum = 0;
      for (int i = 0; i < k; i++) {
        sum += h_input_a[y*k+i]* h_input_b[i*n + x];
      }

      cpu_out[y*n + x] = beta * h_input_c[y*n+x] + alpha*sum;
    }
  }




  // Device input buffers
  cl_mem d_input_a;
  cl_mem d_input_b;
  cl_mem d_input_c;
  cl_mem d_output;


  cl_platform_id cpPlatform;  // OpenCL platform
  cl_device_id device_id;     // device ID
  cl_context context;         // context
  cl_command_queue queue;     // command queue
  cl_program program;         // program
  cl_kernel kernel;           // kernel

  int tile_size = 16;
  size_t globalSize[2] = {
    ((n-1)/tile_size +1)*tile_size, 
    ((m-1)/tile_size +1)*tile_size
  }; 
  size_t localSize[2] = {tile_size, tile_size};
  cl_int err;

  // Bind to platform
  err = clGetPlatformIDs(1, &cpPlatform, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to get platform IDs");
    exit(1);
  }

  // Get ID for the device
  err = clGetDeviceIDs(cpPlatform, CL_DEVICE_TYPE_GPU, 1, &device_id, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to get device IDs");
    exit(1);
  }

  // Create a context
  context = clCreateContext(0, 1, &device_id, NULL, NULL, &err);
  if (err != CL_SUCCESS) {
    printf("fail to get create context");
    exit(1);
  }

  // Create a command queue
  queue = clCreateCommandQueue(context, device_id, 0, &err);
  if (err != CL_SUCCESS) {
    printf("fail to get create queue");
    exit(1);
  }

  // Create the compute program from the source buffer
  char *kernelSource = read_file("gemm.cl");
  program = clCreateProgramWithSource(context, 1, (const char **)&kernelSource,
                                      NULL, &err);
  if (err != CL_SUCCESS) {
    printf("fail to create program with source");
    exit(1);
  }

  // Build the program executable
  err = clBuildProgram(program, 0, NULL, NULL, NULL, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to build program");
    if (err == CL_BUILD_PROGRAM_FAILURE) {
      // Determine the size of the log
      size_t log_size;
      clGetProgramBuildInfo(program, device_id, CL_PROGRAM_BUILD_LOG, 0, NULL,
                            &log_size);

      // Allocate memory for the log
      char *log = (char *)malloc(log_size);

      // Get the log
      clGetProgramBuildInfo(program, device_id, CL_PROGRAM_BUILD_LOG, log_size,
                            log, NULL);

      // Print the log
      printf("%s\n", log);
    }
    exit(1);
  }

  // Create the compute kernel in the program we wish to run
  kernel = clCreateKernel(program, "gemm", &err);
  if (err != CL_SUCCESS) {
    printf("fail to create kernel %d\n", err);
    exit(1);
  }

  // Create the input and output arrays in device memory for our calculation
  d_input_a =
      clCreateBuffer(context, CL_MEM_READ_ONLY, sizeof(float)*m*n , NULL, NULL);
  d_input_b =
      clCreateBuffer(context, CL_MEM_READ_ONLY, sizeof(float)*n*k, NULL, NULL);
  d_input_c =
      clCreateBuffer(context, CL_MEM_READ_ONLY, sizeof(float)*m*n, NULL, NULL);
  d_output =
      clCreateBuffer(context, CL_MEM_WRITE_ONLY, sizeof(float)*m*n, NULL, NULL);

  // Write our data set into the input array in device memory
  err = clEnqueueWriteBuffer(queue, d_input_a, CL_TRUE, 0, m*k*sizeof(float),
                             h_input_a, 0, NULL, NULL);
  err = clEnqueueWriteBuffer(queue, d_input_b, CL_TRUE, 0, n*k*sizeof(float),
                               h_input_b, 0, NULL, NULL);
  err = clEnqueueWriteBuffer(queue, d_input_c, CL_TRUE, 0, m*n*sizeof(float),
                               h_input_c, 0, NULL, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to enqueue write buffer %d", err);
    exit(1);
  }

  // Set the arguments to our compute kernel
  err |= clSetKernelArg(kernel, 0, sizeof(cl_int), &m);
  err |= clSetKernelArg(kernel, 1, sizeof(cl_int), &n);
  err |= clSetKernelArg(kernel, 2, sizeof(cl_int), &k);
  err |= clSetKernelArg(kernel, 3, sizeof(cl_float), &alpha);
  err |= clSetKernelArg(kernel, 4, sizeof(cl_float), &beta);
  err |= clSetKernelArg(kernel, 5, sizeof(cl_mem), &d_input_a);
  err |= clSetKernelArg(kernel, 6, sizeof(cl_mem), &d_input_b);
  err |= clSetKernelArg(kernel, 7, sizeof(cl_mem), &d_input_c);
  err |= clSetKernelArg(kernel, 8, sizeof(cl_mem), &d_output);

  // Execute the kernel over the entire range of the data set
  uint64_t start = getTimeInNSecs();
  err = clEnqueueNDRangeKernel(queue, kernel, 2, NULL, globalSize, localSize,
                               0, NULL, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to enqueue ND Range Kernel");
    exit(1);
  }

  // Wait for the command queue to get serviced before reading back results
  err = clFinish(queue);
  uint64_t end = getTimeInNSecs();
  if (err != CL_SUCCESS) {
    printf("fail to finish");
    exit(1);
  }

  printf("Time %ld\n", end-start);

  // Read the results from the device
  clEnqueueReadBuffer(queue, d_output, CL_TRUE, 0,
                      m * n * sizeof(float), gpu_out,
                      0, NULL, NULL);
  err = clFinish(queue);
  if (err != CL_SUCCESS) {
    printf("fail to read buffer");
    exit(1);
  }

  for (int y = 0; y < m; y++) {
    for (int x = 0; x < n; x++) {
      if (f32abs(cpu_out[y*n + x]-gpu_out[y*n+x]) > 1) {
        printf("Error at (%d, %d), expedted %f, but get %f\n",
          x, y, cpu_out[y*n + x], gpu_out[y*n+x]);
      }
    }
  }

  // release OpenCL resources
  clReleaseProgram(program);
  clReleaseKernel(kernel);
  clReleaseCommandQueue(queue);
  clReleaseContext(context);

  return 0;
}

