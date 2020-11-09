#include <CL/opencl.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>

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

int main(int argc, char *argv[]) {
  unsigned int dim = 4;
  unsigned int num_element = 280;
  unsigned int input_size[4] = {2, 4, 3, 3};
  unsigned int output_size[4] = {2, 4, 5, 7};
  unsigned int dilate[2] = {2, 3};
  float h_input[] = {
      1.111, 1.112, 1.113, 1.121, 1.122, 1.123, 1.131, 1.132, 1.133,
      1.211, 1.212, 1.213, 1.221, 1.222, 1.223, 1.231, 1.232, 1.233,
      1.311, 1.312, 1.313, 1.321, 1.322, 1.323, 1.331, 1.332, 1.333,
      1.411, 1.412, 1.413, 1.421, 1.422, 1.423, 1.431, 1.432, 1.433,
      2.111, 2.112, 2.113, 2.121, 2.122, 2.123, 2.131, 2.132, 2.133,
      2.211, 2.212, 2.213, 2.221, 2.222, 2.223, 2.231, 2.232, 2.233,
      2.311, 2.312, 2.313, 2.321, 2.322, 2.323, 2.331, 2.332, 2.333,
      2.411, 2.412, 2.413, 2.421, 2.422, 2.423, 2.431, 2.432, 2.433,
  };

  // Device input buffers
  cl_mem d_input;
  cl_mem d_output;
  cl_mem d_in_size;
  cl_mem d_out_size;
  cl_mem d_dilate;
  cl_mem d_in_index_buf;
  cl_mem d_out_index_buf;

  cl_platform_id clPlatform;  // OpenCL platform
  cl_device_id device_id;     // device ID
  cl_context context;         // context
  cl_command_queue queue;     // command queue
  cl_program program;         // program
  cl_kernel kernel;           // kernel

  size_t globalSize, localSize;
  cl_int err;

  // Number of work items in each local work group
  localSize = 64;

  // Number of total work items
  globalSize = num_element;

  // Bind to platform
  err = clGetPlatformIDs(1, &clPlatform, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to get platform IDs");
    exit(1);
  }

  // Get ID for the device
  err = clGetDeviceIDs(clPlatform, CL_DEVICE_TYPE_GPU, 1, &device_id, NULL);
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
  char *kernelSource = read_file("dilate.cl");
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
  kernel = clCreateKernel(program, "dilate_tensor", &err);
  if (err != CL_SUCCESS) {
    printf("fail to create kernel");
    exit(1);
  }

  // Create the input and output arrays in device memory for our calculation
  d_input =
      clCreateBuffer(context, CL_MEM_READ_ONLY, sizeof(h_input), NULL, NULL);
  d_output =
      clCreateBuffer(context, CL_MEM_WRITE_ONLY, sizeof(h_input), NULL, NULL);
  d_in_size = clCreateBuffer(context, CL_MEM_READ_ONLY, dim * sizeof(uint32_t),
                             NULL, NULL);
  d_out_size = clCreateBuffer(context, CL_MEM_READ_ONLY, dim * sizeof(uint32_t),
                              NULL, NULL);
  d_dilate = clCreateBuffer(context, CL_MEM_READ_ONLY, dim * sizeof(uint32_t),
                           NULL, NULL);
  d_in_index_buf =
      clCreateBuffer(context, CL_MEM_READ_WRITE,
                     dim * num_element * sizeof(int32_t), NULL, NULL);
  int *h_in_index_buf = malloc(dim * num_element * sizeof(int32_t));
  d_out_index_buf =
      clCreateBuffer(context, CL_MEM_READ_WRITE,
                     dim * num_element * sizeof(int32_t), NULL, NULL);
  int *h_out_index_buf = malloc(dim * num_element * sizeof(int32_t));
  float *h_out = malloc(num_element * sizeof(float));

  // Write our data set into the input array in device memory
  err = clEnqueueWriteBuffer(queue, d_input, CL_TRUE, 0, sizeof(h_input),
                             h_input, 0, NULL, NULL);
  err |= clEnqueueWriteBuffer(queue, d_in_size, CL_TRUE, 0, sizeof(input_size),
                              input_size, 0, NULL, NULL);
  err |= clEnqueueWriteBuffer(queue, d_out_size, CL_TRUE, 0,
                              sizeof(output_size), output_size, 0, NULL, NULL);
  err |= clEnqueueWriteBuffer(queue, d_dilate, CL_TRUE, 0,
                              sizeof(dilate), dilate, 0, NULL, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to enqueue write buffer");
    exit(1);
  }

  // Set the arguments to our compute kernel
  err = clSetKernelArg(kernel, 0, sizeof(cl_mem), &d_input);
  err |= clSetKernelArg(kernel, 1, sizeof(cl_mem), &d_output);
  err |= clSetKernelArg(kernel, 2, sizeof(cl_mem), &d_in_size);
  err |= clSetKernelArg(kernel, 3, sizeof(cl_mem), &d_out_size);
  err |= clSetKernelArg(kernel, 4, sizeof(cl_mem), &d_dilate);
  err |= clSetKernelArg(kernel, 5, sizeof(cl_mem), &d_in_index_buf);
  err |= clSetKernelArg(kernel, 6, sizeof(cl_mem), &d_out_index_buf);
  err |= clSetKernelArg(kernel, 7, sizeof(cl_int), &dim);

  // Execute the kernel over the entire range of the data set
  err = clEnqueueNDRangeKernel(queue, kernel, 1, NULL, &globalSize, &localSize,
                               0, NULL, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to enqueue ND Range Kernel");
    exit(1);
  }

  // Wait for the command queue to get serviced before reading back results
  err = clFinish(queue);
  if (err != CL_SUCCESS) {
    printf("fail to finish");
    exit(1);
  }

  // Read the results from the device
  clEnqueueReadBuffer(queue, d_out_index_buf, CL_TRUE, 0,
                      dim * num_element * sizeof(int), h_out_index_buf,
                      0, NULL, NULL);
  clEnqueueReadBuffer(queue, d_in_index_buf, CL_TRUE, 0,
                      dim * num_element * sizeof(int), h_in_index_buf,
                      0, NULL, NULL);
  clEnqueueReadBuffer(queue, d_output, CL_TRUE, 0,
                      num_element * sizeof(float), h_out,
                      0, NULL, NULL);
  err = clFinish(queue);
  if (err != CL_SUCCESS) {
    printf("fail to read buffer");
    exit(1);
  }

  printf("\n\nIndex:\n");
  for (int r = 0; r < num_element; r++) {
    printf("%d: ", r);

    for (int c = 0; c < dim; c++) {
      printf("%d ", h_out_index_buf[r * dim + c]);
    }

    printf(" -> ");

    for (int c = 0; c < dim; c++) {
      printf("%d ", h_in_index_buf[r * dim + c]);
    }

    printf("\n");
  }

  printf("\n\nOut:\n");
  for (int r = 0; r < 8; r++) {
    printf("%d: ", r);

    for (int c = 0; c < 35; c++) {
      printf("%f ", h_out[r * 9 + c]);
    }

    printf("\n");
  }

  // release OpenCL resources
  clReleaseProgram(program);
  clReleaseKernel(kernel);
  clReleaseCommandQueue(queue);
  clReleaseContext(context);


  return 0;
}
