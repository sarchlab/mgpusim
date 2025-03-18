/**********************************************************************
Copyright ©2015 Advanced Micro Devices, Inc. All rights reserved.

Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:

•   Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.
•   Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or
 other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
 WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY
 DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS
 OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
 NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
********************************************************************/


#include "MatrixTranspose.hpp"

int
MatrixTranspose::setupMatrixTranspose()
{
    cl_uint inputSizeBytes;

    // allocate and init memory used by host
    inputSizeBytes = width * height * sizeof(cl_float);
    input = (cl_float *) malloc(inputSizeBytes);
    CHECK_ALLOCATION(input, "Failed to allocate host memory. (input)");

    // random initialisation of input
    fillRandom<cl_float>(input, width, height, 0, 255);

    output = (cl_float *) malloc(inputSizeBytes);
    CHECK_ALLOCATION(output, "Failed to allocate host memory. (output)");

    if(sampleArgs->verify)
    {
        verificationOutput = (cl_float *) malloc(inputSizeBytes);
        CHECK_ALLOCATION(verificationOutput,
                         "Failed to allocate host memory. (verificationOutput)");
    }

    // Unless quiet mode has been enabled, print the INPUT array.
    if(!sampleArgs->quiet)
    {
        printArray<cl_float>(
            "Input",
            input,
            width,
            1);
    }

    return SDK_SUCCESS;
}

int
MatrixTranspose::genBinaryImage()
{
    /*
     * Have a look at the available platforms and pick either
     * the AMD one if available or a reasonable default.
     */

    bifData binaryData;
    binaryData.kernelName = std::string("MatrixTranspose_Kernels.cl");
    binaryData.flagsStr = std::string("");
    if(sampleArgs->isComplierFlagsSpecified())
    {
        binaryData.flagsFileName = std::string(sampleArgs->flags.c_str());
    }

    binaryData.binaryName = std::string(sampleArgs->dumpBinary.c_str());
    int status = generateBinaryImage(binaryData);
    return status;
}

int
MatrixTranspose::setupCL(void)
{
    cl_int status = 0;
    cl_device_type dType;

    if(sampleArgs->deviceType.compare("cpu") == 0)
    {
        dType = CL_DEVICE_TYPE_CPU;
    }
    else //deviceType = "gpu"
    {
        dType = CL_DEVICE_TYPE_GPU;
        if(sampleArgs->isThereGPU() == false)
        {
            std::cout << "GPU not found. Falling back to CPU device" << std::endl;
            dType = CL_DEVICE_TYPE_CPU;
        }
    }

    /*
     * Have a look at the available platforms and pick either
     * the AMD one if available or a reasonable default.
     */

    cl_platform_id platform = NULL;
    int retValue = getPlatform(platform, sampleArgs->platformId,
                               sampleArgs->isPlatformEnabled());
    CHECK_ERROR(retValue, SDK_SUCCESS, "getPlatform() failed");

    // Display available devices.
    retValue = displayDevices(platform, dType);
    CHECK_ERROR(retValue, SDK_SUCCESS, "displayDevices() failed");

    /*
     * If we could find our platform, use it. Otherwise use just available platform.
     */

    cl_context_properties cps[3] =
    {
        CL_CONTEXT_PLATFORM,
        (cl_context_properties)platform,
        0
    };

    context = clCreateContextFromType(
                  cps,
                  dType,
                  NULL,
                  NULL,
                  &status);

    CHECK_OPENCL_ERROR(status, "clCreateContextFromType failed.");

    // getting device on which to run the sample
    status = getDevices(context, &devices, sampleArgs->deviceId,
                        sampleArgs->isDeviceIdEnabled());
    CHECK_ERROR(status, SDK_SUCCESS, "getDevices() failed");

    // Get Device specific Information, Set device info of given cl_device_id
    retValue = deviceInfo.setDeviceInfo(devices[sampleArgs->deviceId]);
    CHECK_ERROR(retValue, SDK_SUCCESS, "SDKDeviceInfo::setDeviceInfo() failed");

    {
        // The block is to move the declaration of prop closer to its use
        cl_command_queue_properties prop = CL_QUEUE_PROFILING_ENABLE;
        commandQueue = clCreateCommandQueue(
                           context,
                           devices[sampleArgs->deviceId],
                           prop,
                           &status);
        CHECK_ERROR(status, 0, "clCreateCommandQueue failed.");
    }

    // Set Persistent memory only for AMD platform
    cl_mem_flags inMemFlags = CL_MEM_READ_ONLY;
    if(sampleArgs->isAmdPlatform())
        // To achieve best performance, use persistent memory together with
        // clEnqueueMapBuffer (instead of clEnqeueRead/Write).
        // At the same time, in general, the best performance is the function
        // of access pattern and size of the buffer.
    {
        inMemFlags |= CL_MEM_USE_PERSISTENT_MEM_AMD;
    }

    inputBuffer = clCreateBuffer(
                      context,
                      inMemFlags,
                      sizeof(cl_float) * width * height,
                      NULL,
                      &status);
    CHECK_OPENCL_ERROR(status, "clCreateBuffer failed. (inputBuffer)");

    outputBuffer = clCreateBuffer(
                       context,
                       CL_MEM_WRITE_ONLY,
                       sizeof(cl_float) * width * height,
                       NULL,
                       &status);
    CHECK_OPENCL_ERROR(status, "clCreateBuffer failed. (outputBuffer)");

    // create a CL program using the kernel source
    buildProgramData buildData;
    buildData.kernelName = std::string("MatrixTranspose_Kernels.cl");
    buildData.devices = devices;
    buildData.deviceId = sampleArgs->deviceId;
    buildData.flagsStr = std::string("");
    if(sampleArgs->isLoadBinaryEnabled())
    {
        buildData.binaryName = std::string(sampleArgs->loadBinary.c_str());
    }

    if(sampleArgs->isComplierFlagsSpecified())
    {
        buildData.flagsFileName = std::string(sampleArgs->flags.c_str());
    }

    retValue = buildOpenCLProgram(program, context, buildData);
    CHECK_ERROR(retValue, SDK_SUCCESS, "buildOpenCLProgram() failed");

    // get a kernel object handle for a kernel with the given name
    kernel = clCreateKernel(program, "matrixTranspose", &status);
    CHECK_OPENCL_ERROR(status, "clCreateKernel failed.");

    status =  kernelInfo.setKernelWorkGroupInfo(kernel,
              devices[sampleArgs->deviceId]);
    CHECK_ERROR(status, SDK_SUCCESS, "setKernelWorkGroupInfo() failed");

    availableLocalMemory = deviceInfo.localMemSize - kernelInfo.localMemoryUsed;

    // each work item is going to work on [elemsPerThread1Dim x elemsPerThread1Dim] matrix elements,
    // therefore the total size of needed local memory is calculated as
    // # of WIs in a group multiplied by # of matrix elements per a WI
    neededLocalMemory    = blockSize * blockSize * elemsPerThread1Dim *
                           elemsPerThread1Dim * sizeof(cl_float);

    if(neededLocalMemory > availableLocalMemory)
    {
        std::cout << "Unsupported: Insufficient local memory on device." << std::endl;
        return SDK_EXPECTED_FAILURE;
    }

    if((cl_uint)(blockSize * blockSize) > kernelInfo.kernelWorkGroupSize)
    {
        if(kernelInfo.kernelWorkGroupSize >= 64)
        {
            blockSize = 8;
        }
        else if(kernelInfo.kernelWorkGroupSize >= 32)
        {
            blockSize = 4;
        }
        else
        {
            std::cout << "Out of Resources!" << std::endl;
            std::cout << "Group Size specified : " << blockSize * blockSize << std::endl;
            std::cout << "Max Group Size supported on the kernel : "
                      << kernelInfo.kernelWorkGroupSize << std::endl;
            return SDK_FAILURE;
        }
    }

    if(blockSize > deviceInfo.maxWorkItemSizes[0] ||
            blockSize > deviceInfo.maxWorkItemSizes[1] ||
            (size_t)blockSize * blockSize > deviceInfo.maxWorkGroupSize)
    {
        std::cout <<
                  "Unsupported: Device does not support requested number of work items." <<
                  std::endl;
        return SDK_FAILURE;
    }

    return SDK_SUCCESS;
}


int
MatrixTranspose::runCLKernels(void)
{
    cl_int   status;

    // every thread in [blockSize x blockSize] workgroup will execute [elemsPerThread1Dim x elemsPerThread1Dim] elements
    size_t globalThreads[2]= {width/elemsPerThread1Dim, height/elemsPerThread1Dim};

    if(blockSize > globalThreads[0])
    {
        blockSize = (cl_uint)globalThreads[0];
    }

    size_t localThreads[2] = {blockSize, blockSize};

    cl_int eventStatus = CL_QUEUED;

    cl_event inMapEvt;
    cl_event inUnmapEvt;
    cl_event outMapEvt;
    cl_event outUnmapEvt;
    void* inMapPtr = NULL;
    void* outMapPtr = NULL;

    inMapPtr = clEnqueueMapBuffer(
                   commandQueue,
                   inputBuffer,
                   CL_TRUE,
                   CL_MAP_WRITE,
                   0,
                   width * height * sizeof(cl_float),
                   0,
                   NULL,
                   &inMapEvt,
                   &status);
    CHECK_OPENCL_ERROR(status, "clEnqueueMapBuffer failed. (inputBuffer)");

	  status = waitForEventAndRelease(&inMapEvt);
    CHECK_ERROR(status, SDK_SUCCESS, "WaitForEventAndRelease(inMapEvt) Failed");

    memcpy(inMapPtr, input, sizeof(cl_float) * width * height);

    status = clEnqueueUnmapMemObject(
                 commandQueue,
                 inputBuffer,
                 inMapPtr,
                 0,
                 NULL,
                 &inUnmapEvt);
    CHECK_OPENCL_ERROR(status, "clEnqueueUnmapMemObject failed. (inputBuffer)");

    status = clFlush(commandQueue);
    CHECK_OPENCL_ERROR(status, "clFlush failed.");

    status = waitForEventAndRelease(&inUnmapEvt);
    CHECK_ERROR(status, SDK_SUCCESS, "WaitForEventAndRelease(inUnmapEvt) Failed");

    // Set appropriate arguments to the kernel

    // 1st kernel argument - output
    status = clSetKernelArg(
                 kernel,
                 0,
                 sizeof(cl_mem),
                 (void *)&outputBuffer);
    CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (outputBuffer)");

    // 2nd kernel argument - input
    status = clSetKernelArg(
                 kernel,
                 1,
                 sizeof(cl_mem),
                 (void *)&inputBuffer);
    CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (inputBuffer)");

    // 3rd kernel argument - size of input buffer
    status = clSetKernelArg(
                 kernel,
                 2,
                 (size_t)neededLocalMemory,
                 NULL);
    CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (block)");
    
    // 4th kernel argument - number of workitems in the x-axis
    status = clSetKernelArg(
                 kernel,
                 3,
                 sizeof(uint32_t),
                 (void *)&globalThreads[0]);
    CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (width_x)");

    // 5th kernel argument - number of workitems in the y-axis
    status = clSetKernelArg(
                 kernel,
                 4,
                 sizeof(uint32_t),
                 (void *)&globalThreads[1]);
    CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (width_y)");



    // 6th kernel argument - number of work groups in the x-axis
    uint32_t num_of_blocks_x = globalThreads[0] / localThreads[0];
    status = clSetKernelArg(
                 kernel,
                 5,
                 sizeof(uint32_t),
                 (void *)&num_of_blocks_x);
    CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (num_of_blocks_x)");

    // 7th kernel argument - number of work groups in the x-axis
    uint32_t group_x_offset = 0;
    status = clSetKernelArg(
                 kernel,
                 6,
                 sizeof(uint32_t),
                 (void *)&group_x_offset);
    CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (group_x_offset)");
    

    // 8th kernel argument - number of work groups in the x-axis
    uint32_t group_y_offset = 0;
    status = clSetKernelArg(
                 kernel,
                 7,
                 sizeof(uint32_t),
                 (void *)&group_y_offset);
    CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (group_y_offset)");






    // Enqueue a kernel run call.
    cl_event ndrEvt;
    status = clEnqueueNDRangeKernel(
                 commandQueue,
                 kernel,
                 2,
                 NULL,
                 globalThreads,
                 localThreads,
                 0,
                 NULL,
                 &ndrEvt);
    CHECK_OPENCL_ERROR(status, "clEnqueueNDRangeKernel failed.");

    status = clFinish(commandQueue);
    CHECK_OPENCL_ERROR(status, "clFinish failed.");

	// accumulate NDRange time
	double evTime = 0.0;
    status = ReadEventTime(ndrEvt, &evTime);
	CHECK_OPENCL_ERROR(status, "ReadEventTime failed.");

	totalNDRangeTime += evTime;
	
    status = clReleaseEvent(ndrEvt);
    CHECK_OPENCL_ERROR(status, "clReleaseEvent failed.(endTime)");

    outMapPtr = clEnqueueMapBuffer(
                    commandQueue,
                    outputBuffer,
                    CL_FALSE,
                    CL_MAP_READ,
                    0,
                    width * height * sizeof(cl_float),
                    0,
                    NULL,
                    &outMapEvt,
                    &status);
    CHECK_OPENCL_ERROR(status, "clEnqueueMapBuffer failed. (resultBuf)");

    status = clFlush(commandQueue);
    CHECK_OPENCL_ERROR(status, "clFlush failed.");

    status = waitForEventAndRelease(&outMapEvt);
    CHECK_ERROR(status, SDK_SUCCESS, "WaitForEventAndRelease(outMapEvt) Failed");
    memcpy(output, outMapPtr, sizeof(cl_float) * width * height);

    status = clEnqueueUnmapMemObject(
                 commandQueue,
                 outputBuffer,
                 outMapPtr,
                 0,
                 NULL,
                 &outUnmapEvt);
    CHECK_OPENCL_ERROR(status, "clEnqueueUnmapMemObject failed. (resultBuf)");

    status = clFlush(commandQueue);
    CHECK_OPENCL_ERROR(status, "clFlush failed.");

    status = waitForEventAndRelease(&outUnmapEvt);
    CHECK_ERROR(status, SDK_SUCCESS, "WaitForEventAndRelease(outUnmapEvt) Failed");


    return SDK_SUCCESS;
}

/*
 * Naive matrix transpose implementation
 */
void
MatrixTranspose::matrixTransposeCPUReference(
    cl_float * output,
    cl_float * input,
    const cl_uint width,
    const cl_uint height)
{
    for(cl_uint j=0; j < height; j++)
    {
        for(cl_uint i=0; i < width; i++)
        {
            output[i*height + j] = input[j*width + i];
        }
    }
}

int
MatrixTranspose::initialize()
{
    // Call base class Initialize to get default configuration
    if(sampleArgs->initialize() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    // add command line option for blockSize
    Option* xParam = new Option;
    if(!xParam)
    {
        error("Memory Allocation error.\n");
        return SDK_FAILURE;
    }

    xParam->_sVersion = "x";
    xParam->_lVersion = "width";
    xParam->_description = "width of input matrix";
    xParam->_type     = CA_ARG_INT;
    xParam->_value    = &width;

    sampleArgs->AddOption(xParam);
    delete xParam;

    Option* blockSizeParam = new Option;
    if(!blockSizeParam)
    {
        error("Memory Allocation error.\n");
        return SDK_FAILURE;
    }

    blockSizeParam->_sVersion = "b";
    blockSizeParam->_lVersion = "blockSize";
    blockSizeParam->_description =
        "Use local memory of dimensions blockSize x blockSize";
    blockSizeParam->_type     = CA_ARG_INT;
    blockSizeParam->_value    = &blockSize;
    sampleArgs->AddOption(blockSizeParam);
    delete blockSizeParam;

    Option* num_iterations = new Option;
    if(!num_iterations)
    {
        error("Memory allocation error.\n");
        return SDK_FAILURE;
    }

    num_iterations->_sVersion = "i";
    num_iterations->_lVersion = "iterations";
    num_iterations->_description = "Number of iterations for kernel execution";
    num_iterations->_type = CA_ARG_INT;
    num_iterations->_value = &iterations;

    sampleArgs->AddOption(num_iterations);
    delete num_iterations;

    return SDK_SUCCESS;
}


int
MatrixTranspose::setup()
{
	//width should bigger then 0 
    if(width<=0)
    {
        width = 64;
    }

	//blockSize should bigger then 0 
    if(blockSize<=0)
    {
        blockSize = 16;
    }
 
	// Limiting the width such that to prevent arithmetic integer overflow for width^2 .
	int maxWidth =	(int)((float)pow((float)2,(int)12)*(float)sqrt((float)2)) -1 ;
	if(width > maxWidth)
	{
		width = maxWidth;
	}

	if(isPowerOf2(blockSize))
    {
        blockSize = roundToPowerOf2(blockSize);
    }

    if(isPowerOf2(width))
    {
        width = roundToPowerOf2(width);
    }

    // Square Matrix, so height equals to width
    height = width;

    int status=setupMatrixTranspose();
    if(status!=SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    int timer = sampleTimer->createTimer();
    sampleTimer->resetTimer(timer);
    sampleTimer->startTimer(timer);

    status=setupCL();
    if(status!=SDK_SUCCESS)
    {
        return status;
    }

    sampleTimer->stopTimer(timer);

    setupTime = (cl_double)sampleTimer->readTimer(timer);

    return SDK_SUCCESS;
}


int
MatrixTranspose::run()
{
    //Warm up
    for(int i = 0; i < 2 && iterations != 1; i++)
    {
        //. Arguments are set and execution call is enqueued on command buffer
        if(runCLKernels() != SDK_SUCCESS)
        {
            return SDK_FAILURE;
        }
    }

    int timer = sampleTimer->createTimer();
    sampleTimer->resetTimer(timer);
    sampleTimer->startTimer(timer);

    std::cout << "Executing kernel for " << iterations <<
              " iterations" << std::endl;
    std::cout << "-------------------------------------------" << std::endl;

    totalNDRangeTime = 0;

    for(int i = 0; i < iterations; i++)
    {
        // Arguments are set and execution call is enqueued on command buffer
        if(runCLKernels() != SDK_SUCCESS)
        {
            return SDK_FAILURE;
        }
    }

    sampleTimer->stopTimer(timer);
    totalKernelTime = (double)(sampleTimer->readTimer(timer)) / iterations;

    totalNDRangeTime /= iterations;

    if(!sampleArgs->quiet)
    {
        printArray<cl_float>("Output", output, width, 1);
    }

    return SDK_SUCCESS;
}

int
MatrixTranspose::verifyResults()
{
    if(sampleArgs->verify)
    {
        /*
         * reference implementation
         */
        int refTimer = sampleTimer->createTimer();
        sampleTimer->resetTimer(refTimer);
        sampleTimer->startTimer(refTimer);
        matrixTransposeCPUReference(verificationOutput, input, width, height);
        sampleTimer->stopTimer(refTimer);
        referenceKernelTime = sampleTimer->readTimer(refTimer);

        // compare the results and see if they match
        if(compare(output, verificationOutput, width*height))
        {
            std::cout<<"Passed!\n" << std::endl;
            return SDK_SUCCESS;
        }
        else
        {
            std::cout<<"Failed verification test\n" << std::endl;
            return SDK_FAILURE;
        }
    }
    return SDK_SUCCESS;
}

void
MatrixTranspose::printStats()
{
    if(sampleArgs->timing)
    {
        std::string strArray[4] = {"WxH" , "Setup Time(sec)", "Avg. Kernel Time(sec)", "Kernel Speed(GB/s)"};
        std::string stats[4];

        stats[0]  = toString(width, std::dec)
                    +"x"+toString(height, std::dec);
        stats[1]  = toString(setupTime, std::dec);
        stats[2]  = toString(totalNDRangeTime, std::dec);

        double kernelSpeed = height*width*sizeof(float)*2/totalNDRangeTime;
        stats[3]  = toString(kernelSpeed, std::dec);

        printStatistics(strArray, stats, 4);
    }
}

int
MatrixTranspose::cleanup()
{
    // Releases OpenCL resources (Context, Memory etc.)
    cl_int status;

    status = clReleaseKernel(kernel);
    CHECK_OPENCL_ERROR(status, "clReleaseKernel failed.");

    status = clReleaseProgram(program);
    CHECK_OPENCL_ERROR(status, "clReleaseProgram failed.");

    status = clReleaseMemObject(inputBuffer);
    CHECK_OPENCL_ERROR(status, "clReleaseMemObject failed.");

    status = clReleaseMemObject(outputBuffer);
    CHECK_OPENCL_ERROR(status, "clReleaseMemObject failed.");

    status = clReleaseCommandQueue(commandQueue);
    CHECK_OPENCL_ERROR(status, "clReleaseCommandQueue failed.");

    status = clReleaseContext(context);
    CHECK_OPENCL_ERROR(status, "clReleaseContext failed.");

    // release program resources (input memory etc.)
    FREE(input);
    FREE(output);
    FREE(verificationOutput);
    FREE(devices);

    return SDK_SUCCESS;
}

int
main(int argc, char * argv[])
{
    // Create MonteCalroAsian object
    MatrixTranspose clMatrixTranspose;

    // Initialization
    if(clMatrixTranspose.initialize()!=SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    // Parse command line options
    if(clMatrixTranspose.sampleArgs->parseCommandLine(argc, argv))
    {
        return SDK_FAILURE;
    }

    if(clMatrixTranspose.sampleArgs->isDumpBinaryEnabled())
    {
        return clMatrixTranspose.genBinaryImage();
    }
    else
    {
        // Setup
        int status = clMatrixTranspose.setup();
        if(status)
        {
            return status;
        }

        // Run
        if(clMatrixTranspose.run() == SDK_FAILURE)
        {
            return SDK_FAILURE;
        }

        else
        {
            // Verifty
            if(clMatrixTranspose.verifyResults()==SDK_FAILURE)
            {
                return SDK_FAILURE;
            }
        }
        // Cleanup resources created
        if(clMatrixTranspose.cleanup()==SDK_FAILURE)
        {
            return SDK_FAILURE;
        }
        // Print performance statistics
        clMatrixTranspose.printStats();
    }

    return SDK_SUCCESS;
}

