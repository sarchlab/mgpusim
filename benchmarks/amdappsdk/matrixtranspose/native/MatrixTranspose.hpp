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


#ifndef MATRIXTRANSPOSE_H_
#define MATRIXTRANSPOSE_H_


#include <stdio.h>
#include <stdlib.h>
#include <assert.h>
#include <string.h>
#include "CLUtil.hpp"

#define SAMPLE_VERSION "AMD-APP-SDK-v3.0.130.4"

using namespace appsdk;

/**
 * MatrixTranspose
 * Class implements OpenCL Matrix Transpose sample
 */

class MatrixTranspose
{
        cl_uint
        seed;      /**< Seed value for random number generation */
        cl_double           setupTime;      /**< Time for setting up OpenCL */
        cl_double     totalKernelTime;      /**< Time for kernel execution */
        cl_double    totalProgramTime;      /**< Time for program execution */
        cl_double referenceKernelTime;      /**< Time for reference implementation */
        cl_double
        totalNDRangeTime;      /**< Time for kernel execution calculated direclty on NDRange processed event */
        cl_int                  width;      /**< width of the input matrix */
        cl_int                 height;      /**< height of the input matrix */
        cl_float               *input;      /**< Input array */
        cl_float              *output;      /**< Output Array */
        cl_float  *verificationOutput;      /**< Output array for reference implementation */
        cl_uint
        blockSize;      /**< blockSize x blockSize is the number of work items in a work group */
        cl_context            context;      /**< CL context */
        cl_device_id         *devices;      /**< CL device list */
        cl_mem            inputBuffer;      /**< CL memory buffer */
        cl_mem           outputBuffer;      /**< CL memory output Buffer */
        cl_command_queue commandQueue;      /**< CL command queue */
        cl_program            program;      /**< CL program  */
        cl_kernel              kernel;      /**< CL kernel */
        cl_ulong availableLocalMemory;
        cl_ulong    neededLocalMemory;
        int
        iterations;      /**< Number of iterations for kernel execution */
        SDKDeviceInfo deviceInfo; /**< SDKDeviceInfo class object */
        KernelWorkGroupInfo kernelInfo; /**< KernelWorkGroupInfo class Object */

        const cl_uint
        elemsPerThread1Dim;       /**< Number of elements calculated by single WI (or thread) in every dim */

        SDKTimer *sampleTimer;      /**< SDKTimer object */

    public:

        CLCommandArgs   *sampleArgs;   /**< CLCommand argument class */

        /**
         * Constructor
         * Initialize member variables
         */
        MatrixTranspose()
            : elemsPerThread1Dim(4)
        {
            sampleArgs = new CLCommandArgs();
            sampleTimer = new SDKTimer();
            sampleArgs->sampleVerStr = SAMPLE_VERSION;
            seed = 123;
            input = NULL;
            output = NULL;
            verificationOutput = NULL;
            blockSize = 16;
            width = 64;
            height = 64;
            setupTime = 0;
            totalKernelTime = 0;
            iterations = 1;
        }

        /**
         * Allocate and initialize host memory array with random values
         * @return 0 on success and 1 on failure
         */
        int setupMatrixTranspose();

        /**
         * Override from SDKSample, Generate binary image of given kernel
         * and exit application
         */
        int genBinaryImage();

        /**
         * OpenCL related initialisations.
         * Set up Context, Device list, Command Queue, Memory buffers
         * Build CL kernel program executable
         * @return 0 on success and 1 on failure
         */
        int setupCL();

        /**
         * Set values for kernels' arguments, enqueue calls to the kernels
         * on to the command queue, wait till end of kernel execution.
         * Get kernel start and end time if timing is enabled
         * @return 0 on success and 1 on failure
         */
        int runCLKernels();

        /**
         * Reference CPU implementation of matrix transpose
         * @param output stores the transpose of the input
         * @param input  input matrix
         * @param width  width of the input matrix
         * @param height height of the array
         */
        void matrixTransposeCPUReference(
            cl_float * output,
            cl_float * input,
            const cl_uint width,
            const cl_uint height);

        /**
         * Override from SDKSample. Print sample stats.
         */
        void printStats();

        /**
         * Override from SDKSample. Initialize
         * command line parser, add custom options
         */
        int initialize();

        /**
         * Override from SDKSample, adjust width and height
         * of execution domain, perform all sample setup
         */
        int setup();

        /**
         * Override from SDKSample
         * Run OpenCL matrix transpose
         */
        int run();

        /**
         * Override from SDKSample
         * Cleanup memory allocations
         */
        int cleanup();

        /**
         * Override from SDKSample
         * Verify against reference implementation
         */
        int verifyResults();
};



#endif
