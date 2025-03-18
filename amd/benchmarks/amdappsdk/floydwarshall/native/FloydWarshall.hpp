/**********************************************************************
Copyright �2015 Advanced Micro Devices, Inc. All rights reserved.

Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:

�   Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.
�   Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or
 other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
 WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY
 DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS
 OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
 NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
********************************************************************/


#ifndef FLOYDWARSHALL_H_
#define FLOYDWARSHALL_H_

#define SAMPLE_VERSION "AMD-APP-SDK-v3.0.130.2"

/**
 * Header Files
 */
#include <CL/cl.h>
#include <stdio.h>
#include <stdlib.h>
#include <assert.h>
#include <string.h>

#include "CLbuffering.hpp"

using namespace appsdk;

/**
 * FloydWarshall
 * Class implements OpenCL FloydWarshall Pathfinding sample

 */

/*
 * MACROS
 * By default, MAXDISTANCE between two nodes
 */
#define MAXDISTANCE    (200)

class FloydWarshall
{
        cl_uint
        seed;  /**< Seed value for random number generation */
        cl_double                setupTime;  /**< Time for setting up OpenCL */
        cl_double          totalKernelTime;  /**< Time for kernel execution */
        cl_double         totalProgramTime;  /**< Time for program execution */
        cl_double      referenceKernelTime;  /**< Time for reference implementation */
        cl_int                    numNodes;  /**< Number of nodes in the graph */
        cl_uint        *pathDistanceMatrix;  /**< path distance array */
        cl_uint                *pathMatrix;  /**< path arry */
        cl_uint *verificationPathDistanceMatrix;/**< path distance array for reference implementation */
        cl_uint
        *verificationPathMatrix; /**< path array for reference implementation */
        cl_context                 context; /**< CL context */
        cl_device_id              *devices; /**< CL device list */
        cl_mem          pathDistanceBuffer; /**< CL path distance memory buffer */
        cl_mem                  pathBuffer; /**< CL path memory buffer */
        cl_command_queue      commandQueue; /**< CL command queue */
        cl_program                 program; /**< CL program  */
        cl_kernel                   kernel; /**< CL kernel */
        int
        iterations; /**< Number of iterations to execute kernel */
        cl_uint
        blockSize;      /**< use local memory of size blockSize x blockSize */
        KernelWorkGroupInfo
        kernelInfo;/**< KernelWorkGroupInfo object to hold kernel properties */
        SDKTimer    *sampleTimer;      /**< SDKTimer object */

    public:

        CLCommandArgs   *sampleArgs;   /**< CLCommand argument class */
        /**
         * Constructor
         * Initialize member variables
         * @param name name of sample (string)
         */
        FloydWarshall()
        {
            seed = 123;
            numNodes = 256;
            pathDistanceMatrix = NULL;
            pathMatrix = NULL;
            verificationPathDistanceMatrix = NULL;
            verificationPathMatrix         = NULL;
            setupTime = 0;
            totalKernelTime = 0;
            iterations = 1;
            blockSize = 16;
            sampleArgs = new CLCommandArgs() ;
            sampleTimer = new SDKTimer();
            sampleArgs->sampleVerStr = SAMPLE_VERSION;
        }

        /**
         * Allocate and initialize host memory array with random values
         * @return SDK_SUCCESS on success and SDK_FAILURE on failure
         */
        int setupFloydWarshall();

        /**
         * Override from SDKSample, Generate binary image of given kernel
         * and exit application
         * @return SDK_SUCCESS on success and SDK_FAILURE on failure
         */
        int genBinaryImage();

        /**
         * OpenCL related initialisations.
         * Set up Context, Device list, Command Queue, Memory buffers
         * Build CL kernel program executable
         * @return SDK_SUCCESS on success and SDK_FAILURE on failure
         */
        int setupCL();

        /**
         * Set values for kernels' arguments, enqueue calls to the kernels
         * on to the command queue, wait till end of kernel execution.
         * Get kernel start and end time if timing is enabled
         * @return SDK_SUCCESS on success and SDK_FAILURE on failure
         */
        int runCLKernels();

        /**
         * Returns the lesser of the two unsigned integers a and b
         */
        cl_uint minimum(cl_uint a, cl_uint b);

        /**
         * Reference CPU implementation of FloydWarshall PathFinding
         * for performance comparison
         * @param pathDistanceMatrix Distance between nodes of a graph
         * @param intermediate node between two nodes of a graph
         * @param number of nodes in the graph
         */
        void floydWarshallCPUReference(cl_uint * pathDistanceMatrix,
                                       cl_uint * pathMatrix, cl_uint numNodes);

        /**
         * Override from SDKSample. Print sample stats.
         */
        void printStats();

        /**
         * Override from SDKSample. Initialize
         * command line parser, add custom options
         * @return SDK_SUCCESS on success and SDK_FAILURE on failure
         */
        int initialize();

        /**
         * Override from SDKSample, adjust width and height
         * of execution domain, perform all sample setup
         * @return SDK_SUCCESS on success and SDK_FAILURE on failure
         */
        int setup();

        /**
         * Override from SDKSample
         * Run OpenCL FloydWarshall Path finding
         * @return SDK_SUCCESS on success and SDK_FAILURE on failure
         */
        int run();

        /**
         * Override from SDKSample
         * Cleanup memory allocations
         * @return SDK_SUCCESS on success and SDK_FAILURE on failure
         */
        int cleanup();

        /**
         * Override from SDKSample
         * Verify against reference implementation
         * @return SDK_SUCCESS on success and SDK_FAILURE on failure
         */
        int verifyResults();
};
#endif
