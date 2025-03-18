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


#include "FloydWarshall.hpp"

int FloydWarshall::setupFloydWarshall()
{
    cl_uint matrixSizeBytes;

    // allocate and init memory used by host
    matrixSizeBytes = numNodes * numNodes * sizeof(cl_uint);
    pathDistanceMatrix = (cl_uint *) malloc(matrixSizeBytes);
    CHECK_ALLOCATION(pathDistanceMatrix,
                     "Failed to allocate host memory. (pathDistanceMatrix)");

    pathMatrix = (cl_uint *) malloc(matrixSizeBytes);
    CHECK_ALLOCATION(pathMatrix, "Failed to allocate host memory. (pathMatrix)");

    // random initialisation of input

    /*
     * pathMatrix is the intermediate node from which the path passes
     * pathMatrix(i,j) = k means the shortest path from i to j
     * passes through an intermediate node k
     * Initialized such that pathMatrix(i,j) = i
     */

    fillRandom<cl_uint>(pathDistanceMatrix, numNodes, numNodes, 0, MAXDISTANCE);
    for(cl_int i = 0; i < numNodes; ++i)
    {
        cl_uint iXWidth = i * numNodes;
        pathDistanceMatrix[iXWidth + i] = 0;
    }

    /*
     * pathMatrix is the intermediate node from which the path passes
     * pathMatrix(i,j) = k means the shortest path from i to j
     * passes through an intermediate node k
     * Initialized such that pathMatrix(i,j) = i
     */
    for(cl_int i = 0; i < numNodes; ++i)
    {
        for(cl_int j = 0; j < i; ++j)
        {
            pathMatrix[i * numNodes + j] = i;
            pathMatrix[j * numNodes + i] = j;
        }
        pathMatrix[i * numNodes + i] = i;
    }

    /*
     * Unless sampleArgs->quiet mode has been enabled, print the INPUT array.
     */
    if(!sampleArgs->quiet)
    {
        printArray<cl_uint>(
            "Path Distance",
            pathDistanceMatrix,
            numNodes,
            1);

        printArray<cl_uint>(
            "Path ",
            pathMatrix,
            numNodes,
            1);
    }

    if(sampleArgs->verify)
    {
        verificationPathDistanceMatrix = (cl_uint *) malloc(numNodes * numNodes *
                                         sizeof(cl_int));
        CHECK_ALLOCATION(verificationPathDistanceMatrix,
                         "Failed to allocate host memory. (verificationPathDistanceMatrix)");

        verificationPathMatrix = (cl_uint *) malloc(numNodes * numNodes * sizeof(
                                     cl_int));
        CHECK_ALLOCATION(verificationPathMatrix,
                         "Failed to allocate host memory. (verificationPathMatrix)");

        memcpy(verificationPathDistanceMatrix, pathDistanceMatrix,
               numNodes * numNodes * sizeof(cl_int));
        memcpy(verificationPathMatrix, pathMatrix, numNodes*numNodes*sizeof(cl_int));
    }

    return SDK_SUCCESS;
}

int
FloydWarshall::genBinaryImage()
{
    bifData binaryData;
    binaryData.kernelName = std::string("FloydWarshall_Kernels.cl");
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
FloydWarshall::setupCL(void)
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
            std::cout << "GPU not found. Fall back to CPU device" << std::endl;
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
    context = clCreateContextFromType(cps,
                                      dType,
                                      NULL,
                                      NULL,
                                      &status);
    CHECK_OPENCL_ERROR(status, "clCreateContextFromType failed.");

    // getting device on which to run the sample
    status = getDevices(context, &devices, sampleArgs->deviceId,
                        sampleArgs->isDeviceIdEnabled());
    CHECK_ERROR(status, SDK_SUCCESS, "getDevices() failed");

    {
        // The block is to move the declaration of prop closer to its use
        cl_command_queue_properties prop = 0;
        commandQueue = clCreateCommandQueue(context,
                                            devices[sampleArgs->deviceId],
                                            prop,
                                            &status);
        CHECK_OPENCL_ERROR(status, "clCreateCommandQueue failed.");
    }

    pathDistanceBuffer = clCreateBuffer(context,
                                        CL_MEM_READ_WRITE,
                                        sizeof(cl_uint) * numNodes * numNodes,
                                        NULL,
                                        &status);
    CHECK_OPENCL_ERROR(status, "clCreateBuffer failed. (pathDistanceBuffer)");

    pathBuffer = clCreateBuffer(context,
                                CL_MEM_WRITE_ONLY | CL_MEM_ALLOC_HOST_PTR,
                                sizeof(cl_uint) * numNodes * numNodes,
                                NULL,
                                &status);
    CHECK_OPENCL_ERROR(status, "clCreateBuffer failed. (pathBuffer)");

    // create a CL program using the kernel source
    buildProgramData buildData;
    buildData.kernelName = std::string("FloydWarshall_Kernels.cl");
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
    CHECK_ERROR(retValue, 0, "buildOpenCLProgram() failed");


    // get a kernel object handle for a kernel with the given name
    kernel = clCreateKernel(program, "floydWarshallPass", &status);
    CHECK_OPENCL_ERROR(status, "clCreateKernel failed.");

    return SDK_SUCCESS;
}

int
FloydWarshall::runCLKernels(void)
{
    cl_int   status;
    cl_uint numPasses = numNodes;
    size_t globalThreads[2] = {numNodes, numNodes};
    size_t localThreads[2] = {blockSize, blockSize};

    totalKernelTime = 0;

    // Check group size against kernelWorkGroupSize
    status = kernelInfo.setKernelWorkGroupInfo(kernel,
             devices[sampleArgs->deviceId]);
    CHECK_OPENCL_ERROR(status, "kernelInfo.setKernelWorkGroupInfo failed.");

    if((cl_uint)(localThreads[0] * localThreads[0]) >
            kernelInfo.kernelWorkGroupSize)
    {
        if(!sampleArgs->quiet)
        {
            std::cout << "Out of Resources!" << std::endl;
            std::cout << "Group Size specified : "<<localThreads[0]<<std::endl;
            std::cout << "Max Group Size supported on the kernel : "
                      << kernelInfo.kernelWorkGroupSize<<std::endl;
            std::cout << "Changing the group size to " << kernelInfo.kernelWorkGroupSize
                      << std::endl;
        }

        blockSize = 4;

        localThreads[0] = blockSize;
        localThreads[1] = blockSize;
    }

    /*
    * The floyd Warshall algorithm is a multipass algorithm
    * that calculates the shortest path between each pair of
    * nodes represented by pathDistanceBuffer.
    *
    * In each pass a node k is introduced and the pathDistanceBuffer
    * which has the shortest distance between each pair of nodes
    * considering the (k-1) nodes (that are introduced in the previous
    * passes) is updated such that
    *
    * ShortestPath(x,y,k) = min(ShortestPath(x,y,k-1), ShortestPath(x,k,k-1) + ShortestPath(k,y,k-1))
    * where x and y are the pair of nodes between which the shortest distance
    * is being calculated.
    *
    * pathBuffer stores the intermediate nodes through which the shortest
    * path goes for each pair of nodes.
    */

    // Set input data
    cl_event writeEvt;
    status = clEnqueueWriteBuffer(
                 commandQueue,
                 pathDistanceBuffer,
                 CL_FALSE,
                 0,
                 sizeof(cl_uint) * numNodes * numNodes,
                 pathDistanceMatrix,
                 0,
                 NULL,
                 &writeEvt);
    CHECK_OPENCL_ERROR(status, "clEnqueueWriteBuffer failed. (pathDistanceBuffer)");

    status = clFlush(commandQueue);
    CHECK_OPENCL_ERROR(status, "clFlush failed.");

    status = waitForEventAndRelease(&writeEvt);
    CHECK_ERROR(status, SDK_SUCCESS, "WaitForEventAndRelease(writeEvt) Failed");

    /*
     * Set appropriate arguments to the kernel
     *
     * First argument of the kernel is the adjacency matrix
     */
    status = clSetKernelArg(kernel,
                            0,
                            sizeof(cl_mem),
                            (void*)&pathDistanceBuffer);
    CHECK_OPENCL_ERROR(status, "clSetKernelArg failed.(pathDistanceBuffer)");

    /*
     * Second argument to the kernel is the path matrix
     * the matrix that stores the nearest node through which the shortest path
     * goes.
     */
    status = clSetKernelArg(kernel,
                            1,
                            sizeof(cl_mem),
                            (void*)&pathBuffer);
    CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (PathBuffer)");

    /*
     * Third argument is the number of nodes in the graph
     */
    status = clSetKernelArg(kernel,
                            2,
                            sizeof(cl_uint),
                            (void*)&numNodes);
    CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (numNodes)");

    // numNodes - i.e number of elements in the array
    status = clSetKernelArg(kernel,
                            3,
                            sizeof(cl_uint),
                            (void*)&numNodes);
    CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (numNodes)");

    for(cl_uint i = 0; i < numPasses; i += 1)
    {
        /*
         * Kernel needs which pass of the algorithm is running
         * which is sent as the Fourth argument
         */
        status = clSetKernelArg(kernel,
                                3,
                                sizeof(cl_uint),
                                (void*)&i);
        CHECK_OPENCL_ERROR(status, "clSetKernelArg failed. (pass)");


        // Enqueue a kernel run call.

        cl_event ndrEvt;
        status = clEnqueueNDRangeKernel(commandQueue,
                                        kernel,
                                        2,
                                        NULL,
                                        globalThreads,
                                        localThreads,
                                        0,
                                        NULL,
                                        &ndrEvt);
        CHECK_OPENCL_ERROR(status, "clEnqueueNDRangeKernel failed.");

        status = clFlush(commandQueue);
        CHECK_OPENCL_ERROR(status, "clFlush failed.");

        status = waitForEventAndRelease(&ndrEvt);
        CHECK_ERROR(status, SDK_SUCCESS, "WaitForEventAndRelease(ndrEvt) Failed");
    }

    // Enqueue readBuffer
    cl_event readEvt1;
    status = clEnqueueReadBuffer(commandQueue,
                                 pathBuffer,
                                 CL_TRUE,
                                 0,
                                 numNodes * numNodes * sizeof(cl_uint),
                                 pathMatrix,
                                 0,
                                 NULL,
                                 &readEvt1);
    CHECK_OPENCL_ERROR(status, "clEnqueueReadBuffer failed.");

    status = clFlush(commandQueue);
    CHECK_OPENCL_ERROR(status, "clFlush failed.");

    status = waitForEventAndRelease(&readEvt1);
    CHECK_ERROR(status, SDK_SUCCESS, "WaitForEventAndRelease(readEvt1) Failed");


    // Enqueue readBuffer
    cl_event readEvt2;
    status = clEnqueueReadBuffer(commandQueue,
                                 pathDistanceBuffer,
                                 CL_TRUE,
                                 0,
                                 numNodes * numNodes * sizeof(cl_uint),
                                 pathDistanceMatrix,
                                 0,
                                 NULL,
                                 &readEvt2);
    CHECK_OPENCL_ERROR(status, "clEnqueueReadBuffer failed.");

    status = clFlush(commandQueue);
    CHECK_OPENCL_ERROR(status, "clFlush failed.");

    status = waitForEventAndRelease(&readEvt2);
    CHECK_ERROR(status, SDK_SUCCESS, "WaitForEventAndRelease(readEvt2) Failed");

    return SDK_SUCCESS;
}

/*
 * Returns the lesser of the two unsigned integers a and b
 */
cl_uint
FloydWarshall::minimum(cl_uint a, cl_uint b)
{
    return (b < a) ? b : a;
}

/*
 * Calculates the shortest path between each pair of nodes in a graph
 * pathDistanceMatrix gives the shortest distance between each node
 * in the graph.
 * pathMatrix gives the path intermediate node through which the shortest
 * distance in calculated
 * numNodes is the number of nodes in the graph
 */
void
FloydWarshall::floydWarshallCPUReference(cl_uint * pathDistanceMatrix,
        cl_uint * pathMatrix,
        const cl_uint numNodes)
{
    cl_uint distanceYtoX, distanceYtoK, distanceKtoX, indirectDistance;

    /*
     * pathDistanceMatrix is the adjacency matrix(square) with
     * the dimension equal to the number of nodes in the graph.
     */
    cl_uint width = numNodes;
    cl_uint yXwidth;

    /*
     * for each intermediate node k in the graph find the shortest distance between
     * the nodes i and j and update as
     *
     * ShortestPath(i,j,k) = min(ShortestPath(i,j,k-1), ShortestPath(i,k,k-1) + ShortestPath(k,j,k-1))
     */
    for(cl_uint k = 0; k < numNodes; ++k)
    {
        for(cl_uint y = 0; y < numNodes; ++y)
        {
            yXwidth =  y*numNodes;
            for(cl_uint x = 0; x < numNodes; ++x)
            {
                distanceYtoX = pathDistanceMatrix[yXwidth + x];
                distanceYtoK = pathDistanceMatrix[yXwidth + k];
                distanceKtoX = pathDistanceMatrix[k * width + x];

                indirectDistance = distanceYtoK + distanceKtoX;

                if(indirectDistance < distanceYtoX)
                {
                    pathDistanceMatrix[yXwidth + x] = indirectDistance;
                    pathMatrix[yXwidth + x]         = k;
                }
            }
        }
    }
}

int FloydWarshall::initialize()
{
    // Call base class Initialize to get default configuration
    if(sampleArgs->initialize())
    {
        return SDK_FAILURE;
    }

    Option* num_nodes = new Option;
    CHECK_ALLOCATION(num_nodes, "Memory allocation error.\n");

    num_nodes->_sVersion = "x";
    num_nodes->_lVersion = "nodes";
    num_nodes->_description = "number of nodes";
    num_nodes->_type = CA_ARG_INT;
    num_nodes->_value = &numNodes;
    sampleArgs->AddOption(num_nodes);
    delete num_nodes;

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

int FloydWarshall::setup()
{
    // numNodes should be multiples of blockSize
    if(numNodes % blockSize != 0)
    {
        numNodes = (numNodes / blockSize + 1) * blockSize;
    }

    if(setupFloydWarshall() != SDK_SUCCESS)
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


int FloydWarshall::run()
{
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
        printArray<cl_uint>("Output Path Distance Matrix", pathDistanceMatrix, numNodes,
                            1);
        printArray<cl_uint>("Output Path Matrix", pathMatrix, numNodes, 1);
    }

    return SDK_SUCCESS;
}

int FloydWarshall::verifyResults()
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
        floydWarshallCPUReference(verificationPathDistanceMatrix,
                                  verificationPathMatrix, numNodes);
        sampleTimer->stopTimer(refTimer);
        referenceKernelTime = sampleTimer->readTimer(refTimer);

        if(sampleArgs -> timing)
        {
            std::cout << "CPU time " << referenceKernelTime << std::endl;
        }

        // compare the results and see if they match
        if(memcmp(pathDistanceMatrix, verificationPathDistanceMatrix,
                  numNodes*numNodes*sizeof(cl_uint)) == 0)
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

void FloydWarshall::printStats()
{
    if(sampleArgs->timing)
    {
        std::string strArray[3] = {"Nodes", "Time(sec)", "[Transfer+Kernel]Time(sec)"};
        std::string stats[3];

        sampleTimer->totalTime = setupTime + totalKernelTime;

        stats[0] = toString(numNodes, std::dec);
        stats[1] = toString(sampleTimer->totalTime, std::dec);
        stats[2] = toString(totalKernelTime, std::dec);

        printStatistics(strArray, stats, 3);
    }
}

int FloydWarshall::cleanup()
{
    // Releases OpenCL resources (Context, Memory etc.)
    cl_int status;

    status = clReleaseKernel(kernel);
    CHECK_OPENCL_ERROR(status, "clReleaseKernel failed.");

    status = clReleaseProgram(program);
    CHECK_OPENCL_ERROR(status, "clReleaseProgram failed.");

    status = clReleaseMemObject(pathDistanceBuffer);
    CHECK_OPENCL_ERROR(status, "clReleaseMemObject failed.");

    status = clReleaseMemObject(pathBuffer);
    CHECK_OPENCL_ERROR(status, "clReleaseMemObject failed.");

    status = clReleaseCommandQueue(commandQueue);
    CHECK_OPENCL_ERROR(status, "clReleaseCommandQueue failed.");

    status = clReleaseContext(context);
    CHECK_OPENCL_ERROR(status, "clReleaseContext failed.");

    // release program resources (input memory etc.)
    FREE(pathDistanceMatrix);
    FREE(pathMatrix);
    FREE(verificationPathDistanceMatrix);
    FREE(verificationPathMatrix);
    FREE(devices);

    return SDK_SUCCESS;
}

int
main(int argc, char * argv[])
{
    FloydWarshall clFloydWarshall;

    // Initialize
    if(clFloydWarshall.initialize() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    if(clFloydWarshall.sampleArgs->parseCommandLine(argc, argv) != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    if(clFloydWarshall.sampleArgs->isDumpBinaryEnabled())
    {
        return clFloydWarshall.genBinaryImage();
    }

    // Setup
    if(clFloydWarshall.setup() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    // Run
    if(clFloydWarshall.run() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    // VerifyResults
    if(clFloydWarshall.verifyResults() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }

    // Cleanup
    if(clFloydWarshall.cleanup() != SDK_SUCCESS)
    {
        return SDK_FAILURE;
    }
    clFloydWarshall.printStats();

    return SDK_SUCCESS;
}
