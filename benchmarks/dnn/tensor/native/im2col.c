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
  // Length of vectors
  unsigned int input_size[4] = {2, 3, 3, 3};
  unsigned int kernel_size[4] = {1, 3, 3, 3};
  unsigned int stride[2] = {1, 1};
  unsigned int pad[2] = {1, 1};
  unsigned int dilate[2] = {1, 1};
  unsigned int channel = 3;
  unsigned int batch = 2;
  unsigned int num_kernel = 1;

  unsigned int input_width = input_size[3];
  unsigned int input_height = input_size[2];
  unsigned int kernel_width = kernel_size[3];
  unsigned int kernel_height = kernel_size[2];

  unsigned int eff_kernel_height = (kernel_height - 1) * dilate[0] + 1;
  unsigned int eff_kernel_width = (kernel_width - 1) * dilate[1] + 1;

  unsigned int field_height =
      (input_height - eff_kernel_height + 2*pad[0]) / stride[0] + 1;
  unsigned int field_width =
      (input_width - eff_kernel_width + 2*pad[1]) / stride[1] + 1;

  unsigned int output_size[4] = {batch, num_kernel, field_height,
                                 field_width};

  unsigned int im2col_size[2] = {kernel_width * kernel_height * channel,
                                 field_width * field_height * batch};

  float h_input[] = {
      111, 111, 111, 112, 112, 122, 113, 113, 113, 121, 121, 121, 122, 122,
      122, 123, 123, 123, 131, 131, 131, 132, 132, 132, 133, 133, 133, 211,
      211, 211, 212, 212, 222, 213, 213, 213, 221, 221, 221, 222, 222, 222,
      223, 223, 223, 231, 231, 231, 232, 232, 232, 233, 233, 233,
  };
  float *h_im2col =
      (float *)malloc(im2col_size[0] * im2col_size[1] * sizeof(float));
  float h_kernel[] = {1, 2, 3, 4, 5, 6, 7, 8, 9};

  // Device input buffers
  cl_mem d_input;
  cl_mem d_im2col;

  cl_platform_id cpPlatform;  // OpenCL platform
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
  globalSize = field_width * field_height * batch;

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
  char *kernelSource = read_file("operator.cl");
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
  kernel = clCreateKernel(program, "im2col", &err);
  if (err != CL_SUCCESS) {
    printf("fail to create kernel");
    exit(1);
  }

  // Create the input and output arrays in device memory for our calculation
  d_input =
      clCreateBuffer(context, CL_MEM_READ_ONLY, sizeof(h_input), NULL, NULL);
  d_im2col = clCreateBuffer(context, CL_MEM_READ_ONLY,
                            im2col_size[0] * im2col_size[1] * sizeof(float),
                            NULL, NULL);

  // Write our data set into the input array in device memory
  err = clEnqueueWriteBuffer(queue, d_input, CL_TRUE, 0, sizeof(h_input),
                             h_input, 0, NULL, NULL);
  if (err != CL_SUCCESS) {
    printf("fail to enqueue write buffer");
    exit(1);
  }

  // Set the arguments to our compute kernel
  err = clSetKernelArg(kernel, 0, sizeof(cl_mem), &d_input);
  err |= clSetKernelArg(kernel, 1, sizeof(cl_mem), &d_im2col);

  cl_uint2 input_dimensions;
  input_dimensions.x = input_width;
  input_dimensions.y = input_height;
  err |= clSetKernelArg(kernel, 2, sizeof(cl_uint2), &input_dimensions);

  cl_uint2 kernel_dimensions;
  kernel_dimensions.x = kernel_width;
  kernel_dimensions.y = kernel_height;
  err |= clSetKernelArg(kernel, 3, sizeof(cl_uint2), &kernel_dimensions);

  cl_uint2 stride_dimensions;
  stride_dimensions.x = stride[1];
  stride_dimensions.y = stride[0];
  err |= clSetKernelArg(kernel, 4, sizeof(cl_uint2), &stride_dimensions);

  cl_uint2 pad_dimensions;
  pad_dimensions.x = pad[1];
  pad_dimensions.y = pad[0];
  err |= clSetKernelArg(kernel, 5, sizeof(cl_uint2), &pad_dimensions);

  cl_uint2 dilate_dimensions;
  dilate_dimensions.x = dilate[1];
  dilate_dimensions.y = dilate[0];
  err |= clSetKernelArg(kernel, 6, sizeof(cl_uint2), &dilate_dimensions);

  err |= clSetKernelArg(kernel, 7, sizeof(cl_uint), &channel);
  err |= clSetKernelArg(kernel, 8, sizeof(cl_uint), &batch);

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
  clEnqueueReadBuffer(queue, d_im2col, CL_TRUE, 0,
                      im2col_size[0] * im2col_size[1] * sizeof(float), h_im2col,
                      0, NULL, NULL);
  err = clFinish(queue);
  if (err != CL_SUCCESS) {
    printf("fail to read buffer");
    exit(1);
  }

  printf("\n\nim2col out:\n");
  for (int r = 0; r < im2col_size[0]; r++) {
    for (int c = 0; c < im2col_size[1]; c++) {
      printf("%.2f ", h_im2col[r * im2col_size[1] + c]);
    }
    printf("\n");
  }

  // release OpenCL resources
  clReleaseMemObject(d_input);
  clReleaseMemObject(d_im2col);
  clReleaseProgram(program);
  clReleaseKernel(kernel);
  clReleaseCommandQueue(queue);
  clReleaseContext(context);

  // release host memory
  free(h_im2col);

  return 0;
}
