#include <stdio.h>
#include <stdlib.h>
 
#ifdef __APPLE__
#include <OpenCL/opencl.h>
#else
#include <CL/cl.h>
#endif
 
#define MAX_SOURCE_SIZE (0x100000)
 
int main(void) {
    // Create the two input vectors
        int nthreads = 2;
        int num = 1;
        int channels = 1;
        int height = 2;
        int width = 4;
        int ph = 1;
        int pw = 2;
        int kh = 2;
        int kw = 2;
        int sh = 2;
        int sw = 2;
        int poh=0;
        int pow=0;
        float* bottom = (float*) malloc(8*sizeof(float));
        float* top = (float*) malloc(2*sizeof(float));
        int* mask = (int*) malloc(2*sizeof(int));
        //int* indexs = (int*) malloc(100*sizeof(int));

    for(int i = 0; i < 8; i++) {
	bottom[i] = (float)(i+1);
    }

    // Load the kernel source code into the array source_str
    FILE *fp;
    char *source_str;
    size_t source_size;
 
    fp = fopen("./maxpooling.cl", "r");
    if (!fp) {
        fprintf(stderr, "Failed to load kernel.\n");
        exit(1);
    }
    source_str = (char*)malloc(MAX_SOURCE_SIZE);
    source_size = fread( source_str, 1, MAX_SOURCE_SIZE, fp);
    fclose( fp );
 
    // Get platform and device information
    cl_platform_id platform_id = NULL;
    cl_device_id device_id = NULL;   
    cl_uint ret_num_devices;
    cl_uint ret_num_platforms;
    cl_int ret = clGetPlatformIDs(1, &platform_id, &ret_num_platforms);
    ret = clGetDeviceIDs( platform_id, CL_DEVICE_TYPE_DEFAULT, 1, 
            &device_id, &ret_num_devices);
 
    // Create an OpenCL context
    cl_context context = clCreateContext( NULL, 1, &device_id, NULL, NULL, &ret);
 
    // Create a command queue
    cl_command_queue command_queue = clCreateCommandQueue(context, device_id, 0, &ret);
 
    // Create memory buffers on the device for each vector 
    
    cl_mem bottom_m = clCreateBuffer(context, CL_MEM_READ_ONLY,
            8 * sizeof(float), NULL, &ret);
    cl_mem top_m = clCreateBuffer(context, CL_MEM_WRITE_ONLY, 
            2 * sizeof(float), NULL, &ret);
    cl_mem mask_m = clCreateBuffer(context, CL_MEM_WRITE_ONLY,
            2 * sizeof(int), NULL, &ret);
/*    cl_mem index_m = clCreateBuffer(context, CL_MEM_WRITE_ONLY,
            100 * sizeof(int), NULL, &ret);
*/ 
    // Copy the lists A and B to their respective memory buffers
    
    ret = clEnqueueWriteBuffer(command_queue, bottom_m, CL_TRUE, 0,
            8 * sizeof(float), bottom, 0, NULL, NULL);
    
 
    // Create a program from the kernel source
    cl_program program = clCreateProgramWithSource(context, 1, 
            (const char **)&source_str, (const size_t *)&source_size, &ret);
 
    // Build the program
    ret = clBuildProgram(program, 1, &device_id, NULL, NULL, NULL);
 
    // Create the OpenCL kernel
    cl_kernel kernel = clCreateKernel(program, "MaxPoolForward", &ret);
 
    // Set the arguments of the kernel
    ret = clSetKernelArg(kernel, 0, sizeof(int), &nthreads);
    ret = clSetKernelArg(kernel, 1, sizeof(cl_mem), (void *)&bottom_m);
    ret = clSetKernelArg(kernel, 2, sizeof(int), &num);
    ret = clSetKernelArg(kernel, 3, sizeof(int), &channels);
    ret = clSetKernelArg(kernel, 4, sizeof(int), &height);
    ret = clSetKernelArg(kernel, 5, sizeof(int), &width);
    ret = clSetKernelArg(kernel, 6, sizeof(int), &ph);
    ret = clSetKernelArg(kernel, 7, sizeof(int), &pw);
    ret = clSetKernelArg(kernel, 8, sizeof(int), &kh);
    ret = clSetKernelArg(kernel, 9, sizeof(int), &kw);
    ret = clSetKernelArg(kernel, 10, sizeof(int), &sh);
    ret = clSetKernelArg(kernel, 11, sizeof(int), &sw);
    ret = clSetKernelArg(kernel, 12, sizeof(int), &poh);
    ret = clSetKernelArg(kernel, 13, sizeof(int), &pow);
    ret = clSetKernelArg(kernel, 14, sizeof(cl_mem), (void*)&top_m);
    ret = clSetKernelArg(kernel, 15, sizeof(cl_mem), (void*)&mask_m);
    // Execute the OpenCL kernel on the list
    size_t global_item_size = 8; // Process the entire lists
    size_t local_item_size = 4; // Divide work items into groups of 64
    ret = clEnqueueNDRangeKernel(command_queue, kernel, 1, NULL, 
            &global_item_size, &local_item_size, 0, NULL, NULL);
 
    // Read the memory buffer C on the device to the local variable C
    //int *C = (int*)malloc(sizeof(int)*LIST_SIZE);
    ret = clEnqueueReadBuffer(command_queue, top_m, CL_TRUE, 0, 
            2 * sizeof(float), top, 0, NULL, NULL);
    ret = clEnqueueReadBuffer(command_queue, mask_m, CL_TRUE, 0,
            2 * sizeof(int), mask, 0, NULL, NULL);
 
    // Display the result to the screen
    printf("result\n");
    for(int i = 0; i < 2; i++)
        printf("%f\n", top[i]);
    printf("indices\n");
    for(int i = 0; i < 2; i++)
        printf("%d\n", mask[i]);
 
    // Clean up
    /*ret = clFlush(command_queue);
    ret = clFinish(command_queue);
    ret = clReleaseKernel(kernel);
    ret = clReleaseProgram(program);
    ret = clReleaseMemObject(a_mem_obj);
    ret = clReleaseMemObject(b_mem_obj);
    ret = clReleaseMemObject(c_mem_obj);
    ret = clReleaseCommandQueue(command_queue);
    ret = clReleaseContext(context);
    free(A);
    free(B);
    free(C);*/
    return 0;
}
