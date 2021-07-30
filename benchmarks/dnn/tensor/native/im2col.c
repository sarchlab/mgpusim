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

void print_matrix(float *m, int h, int w) {
  int i, j;
  for (i = 0; i < h; ++i) {
    for (j = 0; j < w; ++j) {
      printf("%f ", m[i * w + j]);
    }
    printf("\n");
  }
}

int main(int argc, char *argv[]) {
  // Dims
  int batch = 1;
  int input_channel = 1;
  int input_height = 3;
  int input_width = 3;
  int output_channel = 1;
  int kernel_height = 3;
  int kernel_width = 3;
  int stride_x = 1;
  int stride_y = 1;
  int pad_x = 1;
  int pad_y = 1;
  int dilate_x = 1;
  int dilate_y = 1;

  // Length of vectors
  unsigned int input_size[4] = {batch, input_channel, input_height,
                                input_width};
  unsigned int input_element =
      input_height * input_width * input_channel * batch;

  unsigned int kernel_size[4] = {output_channel, input_channel, kernel_height,
                                 kernel_width};
  unsigned int kernel_element =
      kernel_height * kernel_width * input_channel * output_channel;

  unsigned int stride[2] = {stride_x, stride_y};
  unsigned int pad[2] = {pad_x, pad_y};
  unsigned int dilate[2] = {dilate_x, dilate_y};

  unsigned int eff_kernel_height = (kernel_height - 1) * dilate[0] + 1;
  unsigned int eff_kernel_width = (kernel_width - 1) * dilate[1] + 1;

  unsigned int field_height =
      (input_height - eff_kernel_height + 2 * pad[0]) / stride[0] + 1;
  unsigned int field_width =
      (input_width - eff_kernel_width + 2 * pad[1]) / stride[1] + 1;

  unsigned int output_size[4] = {batch, output_channel, field_height,
                                 field_width};

  unsigned int im2col_size[2] = {kernel_width * kernel_height * input_channel,
                                 field_width * field_height * batch};

  // Allocate input
  float *h_input = (float *)malloc(input_element * sizeof(float));

  // Random input
  for (int i = 0; i < input_element; i++) {
    // h_input[i] = (float)rand() / (float)RAND_MAX;
    h_input[i] = i;
  }

  float *h_im2col =
      (float *)malloc(im2col_size[0] * im2col_size[1] * sizeof(float));

  // Allocate kernel
  float *h_kernel = (float *)malloc(kernel_element * sizeof(float));

  // Random kernel
  for (int i = 0; i < kernel_element; i++) {
    h_kernel[i] = (float)rand() / (float)RAND_MAX;
  }

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
  char *kernelSource = read_file("im2col.cl");
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
    printf("fail to create kernel, %d\n", err);
    exit(1);
  }

  // Create the input and output arrays in device memory for our calculation
  d_input = clCreateBuffer(context, CL_MEM_READ_ONLY,
                           input_element * sizeof(float), NULL, NULL);
  d_im2col = clCreateBuffer(context, CL_MEM_READ_ONLY,
                            im2col_size[0] * im2col_size[1] * sizeof(float),
                            NULL, NULL);

  // Write our data set into the input array in device memory
  err = clEnqueueWriteBuffer(queue, d_input, CL_TRUE, 0,
                             input_element * sizeof(float), h_input, 0, NULL,
                             NULL);
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

  err |= clSetKernelArg(kernel, 7, sizeof(cl_uint), &input_channel);
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

  // Run im2col on CPU
  float *im2col_cpu = malloc(im2col_size[0] * im2col_size[1] * sizeof(float));
  for (int i = 0; i < im2col_size[0] * im2col_size[1]; i++) {
    printf("\ni=%d\n", i);

    int out_x = i % im2col_size[1];
    int out_y = i / im2col_size[1];

    printf("\tout_y=%d, out_x=%d\n", out_y, out_x);

    int batch_id = out_x / (field_width * field_height);
    int block_id = out_x % (field_width * field_height);
    int block_x = block_id % field_width;
    int block_y = block_id / field_width;

    printf("\tbatch_id=%d, block_id=%d, block_x=%d, block_y=%d\n", batch_id,
           block_id, block_x, block_y);

    int channel_id = out_y / (kernel_height * kernel_width);
    int local_in_y = out_y % (kernel_height * kernel_width) / kernel_width;
    int local_in_x = out_y % (kernel_height * kernel_width) % kernel_width;

    printf("\tchannel_id=%d, local_in_y=%d, local_in_x=%d\n", channel_id,
           local_in_y, local_in_x);

    int in_y = block_y * stride_y - pad_y + dilate_y * local_in_y;
    int in_x = block_x * stride_x - pad_x + dilate_x * local_in_x;

    printf("\tin_y=%d, in_x=%d\n", in_y, in_x);

    int in_index = batch_id * input_channel * input_height * input_width +
                   channel_id * input_height * input_width +
                   in_y * input_width + in_x;

    printf("\tin_index=%d\n", in_index);

    if (in_y < 0 || in_y >= input_height || in_x < 0 || in_x >= input_width) {
      im2col_cpu[i] = 0;
    } else {
      im2col_cpu[i] = h_input[in_index];
    }
  }

  // Dump CPU & GPU results
  printf("CPU\n");
  print_matrix(im2col_cpu, im2col_size[0], im2col_size[1]);
  printf("\nGPU\n");
  print_matrix(h_im2col, im2col_size[0], im2col_size[1]);

  // CPU GPU results must match
  for (int i = 0; i < im2col_size[0] * im2col_size[1]; i++) {
    if (fabs(h_im2col[i] - im2col_cpu[i]) > 1e-5) {
      printf("im2col CPU - GPU mismatch\n");
      printf("i: %d\n", i);
      printf("GPU: %f\n", h_im2col[i]);
      printf("CPU: %f\n", im2col_cpu[i]);
      // exit(1);
    }
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
  free(h_input);

  return 0;
}
