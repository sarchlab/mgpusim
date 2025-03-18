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


#include "FastWalshTransform.hpp"

int
FastWalshTransform::setupFastWalshTransform()
{
    cl_uint inputSizeBytes;

    if(length < 512)
    {
        length = 512;
    }

    // allocate and init memory used by host
    inputSizeBytes = length * sizeof(cl_float);
    input = (cl_float *) malloc(inputSizeBytes);
    CHECK_ALLOCATION(input, "Failed to allocate host memory. (input)");

    output = (cl_float *) malloc(inputSizeBytes);
    CHECK_ALLOCATION(output, "Failed to allocate host memory. (output)");

    // random initialisation of input
    fillRandom<cl_float>(input, length, 1, 0, 255);

    if(sampleArgs->verify)
    {
        verificationInput = (cl_float *) malloc(inputSizeBytes);
        CHECK_ALLOCATION(verificationInput,
                         "Failed to allocate host memory. (verificationInput)");
        memcpy(verificationInput, input, inputSizeBytes);
    }

    // Unless sampleArgs->quiet mode has been enabled, print the INPUT array.
    if(!sampleArgs->quiet)
    {
        printArray<cl_float>(
            "Input",
            input,
            length,
            1);
    }
    return SDK_SUCCESS;
}


int
FastWalshTransform::genBinaryImage()
{
    bifData binaryData;
    binaryData.kernelName = std::string("FastWalshTransform_Kernels.cl");
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
FastWalshTransform::setupCL(void)
{
    cl_int status = 0;
    cl_device_type dType;

    if(sampleArgs->deviceType.compare("cpu") == 0)
    {
        dType = CL_DEVICE_TYPE_CPU;
    }
    else //sampleArgs->deviceType = "gpu"
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
    CHECK_OPENCL_ERROR( status, "clCreateContextFromType failed.");

    // getting device on which to run the sample
    status = getDevices(context, &devices, sampleArgs->deviceId,
                        sampleArgs->isDeviceIdEnabled());
    CHECK_ERROR(status, SDK_SUCCESS, "getDevices() failed");

    {
        // The block is to move the declaration of prop closer to its use
        cl_command_queue_properties prop = 0;
        commandQueue = clCreateCommandQueue(
                           context,
                           devices[sampleArgs->deviceId],
                           prop,
                           &status);
        CHECK_OPENCL_ERROR( status, "clCreateCommandQueue failed.");
    }

    //Set device info of given cl_device_id
    retValue = deviceInfo.setDeviceInfo(devices[sampleArgs->deviceId]);
    CHECK_ERROR(retValue, SDK_SUCCESS, "SDKDeviceInfo::setDeviceInfo() failed");

    inputBuffer = clCreateBuffer(
                      context,
                      CL_MEM_READ_WRITE,
                      sizeof(cl_float) * length,
                      0,
                      &status);
    CHECK_OPENCL_ERROR(status, "clCreateBuffer failed. (inputBuffer)");

    // create a CL program using the kernel source
    buildProgramData buildData;
    buildData.kernelName = std::string("FastWalshTransform_Kernels.cl");
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
    kernel = clCreateKernel(program, "fastWalshTransform", &status);
    CHECK_OPENCL_ERROR(status, "clCreateKernel failed.");

    return SDK_SUCCESS;
}


int
FastWalshTransform::runCLKernels(void)
{
    cl_int   status;
    size_t globalThreads[1];
    size_t localThreads[1];

    // Enqueue write input to inputBuffer
    cl_event writeEvt;
    status = clEnqueueWriteBuffer(
                 commandQueue,
                 inputBuffer,
                 CL_FALSE,
                 0,
                 length * sizeof(cl_float),
                 input,
                 0,
                 NULL,
                 &writeEvt);
    CHECK_OPENCL_ERROR(status, "clEnqueueWriteBuffer failed.");

    status = clFlush(commandQueue);
    CHECK_OPENCL_ERROR(status, "clFlush failed.(commandQueue)");

    status = waitForEventAndRelease(&writeEvt);
    CHECK_ERROR(status, SDK_SUCCESS, "WaitForEventAndRelease(writeEvt) Failed");

    /*
     * The kernel performs a butterfly operation and it runs for half the
     * total number of input elements in the array.
     * In each pass of the kernel two corresponding elements are found using
     * the butterfly operation on an array of numbers and their sum and difference
     * is stored in the same locations as the numbers
     */
    globalThreads[0] = length / 2;
    localThreads[0]  = 256;

    // Check group size against kernelWorkGroupSize
    status = kernelInfo.setKernelWorkGroupInfo(kernel,
             devices[sampleArgs->deviceId]);
    CHECK_OPENCL_ERROR(status, "kernelInfo.setKernelWorkGroupInfo failed.");

    if((cl_uint)(localThreads[0]) > kernelInfo.kernelWorkGroupSize)
    {
        if(!sampleArgs->quiet)
        {
            std::cout << "Out of Resources!" << std::endl;
            std::cout << "Group Size specified : " << localThreads[0] << std::endl;
            std::cout << "Max Group Size supported on the kernel : "
                      << kernelInfo.kernelWorkGroupSize << std::endl;
            std::cout<<"Changing the group size to " << kernelInfo.kernelWorkGroupSize
                     << std::endl;
        }
        localThreads[0] = kernelInfo.kernelWorkGroupSize;
    }

    // Set appropriate arguments to the kernel

    // the input array - also acts as output
    status = clSetKernelArg(
                 kernel,
                 0,
                 sizeof(cl_mem),
                 (void *)&inputBuffer);
    CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (inputBuffer)");

    for(cl_int step = 1; step < length; step <<= 1)
    {
        // stage of the algorithm
        status = clSetKernelArg(
                     kernel,
                     1,
                     sizeof(cl_int),
                     (void *)&step);
        CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (step)");

        // Enqueue a kernel run call
        cl_event ndrEvt;
        status = clEnqueueNDRangeKernel(
                     commandQueue,
                     kernel,
                     1,
                     NULL,
                     globalThreads,
                     localThreads,
                     0,
                     NULL,
                     &ndrEvt);
        CHECK_OPENCL_ERROR(status, "clEnqueueNDRangeKernel failed.");

        status = clFlush(commandQueue);
        CHECK_OPENCL_ERROR(status, "clFlush failed.(commandQueue)");

        status = waitForEventAndRelease(&ndrEvt);
        CHECK_ERROR(status, SDK_SUCCESS, "WaitForEventAndRelease(ndrEvt) Failed");
    }


    // Enqueue readBuffer
    cl_event readEvt;
    status = clEnqueueReadBuffer(
                 commandQueue,
                 inputBuffer,
                 CL_FALSE,
                 0,
                 length *  sizeof(cl_float),
                 output,
                 0,
                 NULL,
                 &readEvt);
    CHECK_OPENCL_ERROR(status, "clEnqueueReadBuffer failed.");

    status = clFlush(commandQueue);
    CHECK_OPENCL_ERROR(status, "clFlush failed.(commandQueue)");

    status = waitForEventAndRelease(&readEvt);
    CHECK_ERROR(status, SDK_SUCCESS, "WaitForEventAndRelease(readEvt) Failed");

    return SDK_SUCCESS;
}

/*
 * This is the reference implementation of the FastWalsh transform
 * Here we perform the buttery operation on an array on numbers
 * to get and pair and a match indices. Their sum and differences are
 * stored in the corresponding locations and is used in the future
 * iterations to get a transformed array
 */
void
FastWalshTransform::fastWalshTransformCPUReference(
    cl_float * vinput,
    const cl_uint length)
{
    // for each pass of the algorithm
    for(cl_uint step = 1; step < length; step <<= 1)
    {
        // length of each block
        cl_uint jump = step << 1;
        // for each blocks
        for(cl_uint group = 0; group < step; ++group)
        {
            // for each pair of elements with in the block
            for(cl_uint pair = group; pair < length; pair += jump)
            {
                // find its partner
                cl_uint match = pair + step;

                cl_float T1 = vinput[pair];
                cl_float T2 = vinput[match];

                // store the sum and difference of the numbers in the same locations
                vinput[pair] = T1 + T2;
                vinput[match] = T1 - T2;
            }
        }
    }
}

int
FastWalshTransform::initialize()
{
    // Call base class Initialize to get default configuration
    if(sampleArgs->initialize() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    // Now add customized options
    Option* signal_length = new Option;
    CHECK_ALLOCATION(signal_length, "Memory allocation error.\n");

    signal_length->_sVersion = "x";
    signal_length->_lVersion = "length";
    signal_length->_description = "Length of input array";
    signal_length->_type = CA_ARG_INT;
    signal_length->_value = &length;
    sampleArgs->AddOption(signal_length);
    delete signal_length;

    Option* num_iterations = new Option;
    CHECK_ALLOCATION(num_iterations, "Memory allocation error.\n");

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
FastWalshTransform::setup()
{
    // make sure the length is the power of 2
    if(isPowerOf2(length))
    {
        length = roundToPowerOf2(length);
    }

    if(setupFastWalshTransform() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    int timer = sampleTimer->createTimer();
    sampleTimer->resetTimer(timer);
    sampleTimer->startTimer(timer);

    if(setupCL() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    sampleTimer->stopTimer(timer);
    setupTime = (cl_double)sampleTimer->readTimer(timer);

    return SDK_SUCCESS;
}


int
FastWalshTransform::run()
{
    // Warm up
    for(int i = 0; i < 2 && iterations != 1; i++)
    {
        // Arguments are set and execution call is enqueued on command buffer
        if(runCLKernels() != SDK_SUCCESS)
        {
            return SDK_FAILURE;
        }
    }

    std::cout << "Executing kernel for " << iterations
              << " iterations" << std::endl;
    std::cout << "-------------------------------------------" << std::endl;

    int timer = sampleTimer->createTimer();
    sampleTimer->resetTimer(timer);
    sampleTimer->startTimer(timer);

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

    if(!sampleArgs->quiet)
    {
        printArray<cl_float>("Output", input, length, 1);
    }

    return SDK_SUCCESS;
}

int
FastWalshTransform::verifyResults()
{
    if(sampleArgs->verify)
    {
        /*
         * reference implementation
         * it overwrites the input array with the output
         */
        int refTimer = sampleTimer->createTimer();
        sampleTimer->resetTimer(refTimer);
        sampleTimer->startTimer(refTimer);

        fastWalshTransformCPUReference(verificationInput, length);

        sampleTimer->stopTimer(refTimer);
        referenceKernelTime = sampleTimer->readTimer(refTimer);

        // compare the results and see if they match
        if(compare(output, verificationInput, length))
        {
            std::cout << "Passed!\n" << std::endl;
            return SDK_SUCCESS;
        }
        else
        {
            std::cout << "Failed\n" << std::endl;
            return SDK_FAILURE;
        }
    }

    return SDK_SUCCESS;
}

void
FastWalshTransform::printStats()
{
    if(sampleArgs->timing)
    {
        std::string strArray[3] = {"Length", "Time(sec)", "[Transfer+Kernel]Time(sec)"};
        std::string stats[3];

        sampleTimer->totalTime = setupTime + totalKernelTime ;

        stats[0] = toString(length, std::dec);
        stats[1] = toString(sampleTimer->totalTime, std::dec);
        stats[2] = toString(totalKernelTime, std::dec);

        printStatistics(strArray, stats, 3);
    }
}
int
FastWalshTransform::cleanup()
{
    // Releases OpenCL resources (Context, Memory etc.)
    cl_int status;

    status = clReleaseKernel(kernel);
    CHECK_OPENCL_ERROR(status, "clReleaseKernel failed.(kernel)");

    status = clReleaseProgram(program);
    CHECK_OPENCL_ERROR(status, "clReleaseProgram failed.(program)");

    status = clReleaseMemObject(inputBuffer);
    CHECK_OPENCL_ERROR(status, "clReleaseMemObject failed.(inputBuffer)");

    status = clReleaseCommandQueue(commandQueue);
    CHECK_OPENCL_ERROR(status, "clReleaseCommandQueue failed.(commandQueue)");

    status = clReleaseContext(context);
    CHECK_OPENCL_ERROR(status, "clReleaseContext failed. (context)");

    // release program resources (input memory etc.)

    FREE(input);
    FREE(output);
    FREE(verificationInput);
    FREE(devices);

    return SDK_SUCCESS;
}

int
main(int argc, char * argv[])
{
    FastWalshTransform clFastWalshTransform;

    // Initialize
    if( clFastWalshTransform.initialize() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    if(clFastWalshTransform.sampleArgs->parseCommandLine(argc, argv))
    {
        return SDK_FAILURE;
    }

    if(clFastWalshTransform.sampleArgs->isDumpBinaryEnabled())
    {
        return clFastWalshTransform.genBinaryImage();
    }

    // Setup
    if(clFastWalshTransform.setup() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    // Run
    if(clFastWalshTransform.run() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    // VerifyResults
    if(clFastWalshTransform.verifyResults() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    // Cleanup
    if(clFastWalshTransform.cleanup() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }
    clFastWalshTransform.printStats();

    return SDK_SUCCESS;
}
