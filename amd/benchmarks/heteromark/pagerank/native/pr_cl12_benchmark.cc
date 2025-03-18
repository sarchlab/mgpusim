/* Copyright (c) 2015 Northeastern University
 * All rights reserved.
 *
 * Developed by:Northeastern University Computer Architecture Research (NUCAR)
 * Group, Northeastern University, http://www.ece.neu.edu/groups/nucar/
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 *  with the Software without restriction, including without limitation
 * the rights to use, copy, modify, merge, publish, distribute, sublicense, and/
 * or sell copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 *   Redistributions of source code must retain the above copyright notice, this
 *   list of conditions and the following disclaimers. Redistributions in binary
 *   form must reproduce the above copyright notice, this list of conditions and
 *   the following disclaimers in the documentation and/or other materials
 *   provided with the distribution. Neither the names of NUCAR, Northeastern
 *   University, nor the names of its contributors may be used to endorse or
 *   promote products derived from this Software without specific prior written
 *   permission.
 *
 *   THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 *   IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 *   FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 *   CONTRIBUTORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 *   LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
 *   FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
 *   DEALINGS WITH THE SOFTWARE.
 */

#include "src/pr/cl12/pr_cl12_benchmark.h"
#include <stdio.h>
#include <string.h>
#include <cstdlib>

void PrCl12Benchmark::Initialize() {
  PrBenchmark::Initialize();

  ClBenchmark::InitializeCl();

  InitializeKernels();
  InitializeBuffers();
}

void PrCl12Benchmark::InitializeKernels() {
  cl_int err;
  file_->open("kernels.cl");

  const char *source = file_->getSourceChar();
  program_ = clCreateProgramWithSource(context_, 1, (const char **)&source,
                                       NULL, &err);
  checkOpenCLErrors(err, "Failed to create program with source...\n");

  err = clBuildProgram(program_, 0, NULL, NULL, NULL, NULL);
  checkOpenCLErrors(err, "Failed to create program...\n");

  pr_kernel_ = clCreateKernel(program_, "PageRankUpdateGpu", &err);
  checkOpenCLErrors(err, "Failed to create kernel PageRankUpdateGpu\n");
}

void PrCl12Benchmark::InitializeBuffers() {
  cl_int err;

  dev_page_rank_ = clCreateBuffer(context_, CL_MEM_READ_WRITE,
                                  num_nodes_ * sizeof(float), NULL, &err);
  checkOpenCLErrors(err, "Failed to create page rank buffer");

  dev_page_rank_temp_ = clCreateBuffer(context_, CL_MEM_READ_WRITE,
                                       num_nodes_ * sizeof(float), NULL, &err);
  checkOpenCLErrors(err, "Failed to create page rank temp buffer");

  dev_row_offsets_ = clCreateBuffer(context_, CL_MEM_READ_ONLY,
                                    (num_nodes_ + 1) * sizeof(int), NULL, &err);
  checkOpenCLErrors(err, "Failed to create page row offsets buffer");

  dev_column_numbers_ = clCreateBuffer(
      context_, CL_MEM_READ_ONLY, num_connections_ * sizeof(int), NULL, &err);
  checkOpenCLErrors(err, "Failed to create page column numbers buffer");

  dev_values_ = clCreateBuffer(context_, CL_MEM_READ_ONLY,
                               num_connections_ * sizeof(float), NULL, &err);
  checkOpenCLErrors(err, "Failed to create values buffer");
}

void PrCl12Benchmark::CopyDataToDevice() {
  cl_int err;

  float init_data = 1.0 / num_nodes_;
  err =
      clEnqueueFillBuffer(cmd_queue_, dev_page_rank_, &init_data, sizeof(float),
                          0, num_nodes_ * sizeof(float), 0, NULL, NULL);
  checkOpenCLErrors(err, "Failed to write page rank buffer");

  err = clEnqueueWriteBuffer(cmd_queue_, dev_row_offsets_, CL_TRUE, 0,
                             (num_nodes_ + 1) * sizeof(int), row_offsets_, 0,
                             NULL, NULL);
  checkOpenCLErrors(err, "Failed to write row offsets buffer");

  err = clEnqueueWriteBuffer(cmd_queue_, dev_column_numbers_, CL_TRUE, 0,
                             num_connections_ * sizeof(int), column_numbers_, 0,
                             NULL, NULL);
  checkOpenCLErrors(err, "Failed to write column numbers buffer");

  err = clEnqueueWriteBuffer(cmd_queue_, dev_values_, CL_TRUE, 0,
                             num_connections_ * sizeof(int), values_, 0, NULL,
                             NULL);
  checkOpenCLErrors(err, "Failed to write values buffer");
}

void PrCl12Benchmark::CopyDataBackFromDevice(cl_mem *buffer) {
  cl_int err;

  err = clEnqueueReadBuffer(cmd_queue_, *buffer, CL_TRUE, 0,
                            num_nodes_ * sizeof(float), page_rank_, 0, NULL,
                            NULL);
  checkOpenCLErrors(err, "Failed to copy data back from device");
}

void PrCl12Benchmark::Run() {
  CopyDataToDevice();

  cl_int err;

  err = clSetKernelArg(pr_kernel_, 0, sizeof(uint32_t), &num_nodes_);
  checkOpenCLErrors(err, "Failed to set kernel argument 0");

  err = clSetKernelArg(pr_kernel_, 1, sizeof(cl_mem), &dev_row_offsets_);
  checkOpenCLErrors(err, "Failed to set kernel argument 1");

  err = clSetKernelArg(pr_kernel_, 2, sizeof(cl_mem), &dev_column_numbers_);
  checkOpenCLErrors(err, "Failed to set kernel argument 2");

  err = clSetKernelArg(pr_kernel_, 3, sizeof(cl_mem), &dev_values_);
  checkOpenCLErrors(err, "Failed to set kernel argument 3");

  err = clSetKernelArg(pr_kernel_, 4, sizeof(float) * 64, NULL);
  checkOpenCLErrors(err, "Failed to set kernel argument 4");

  cpu_gpu_logger_->GPUOn();
  uint32_t i;
  for (i = 0; i < max_iteration_; i++) {
    if (i % 2 == 0) {
      err = clSetKernelArg(pr_kernel_, 5, sizeof(cl_mem), &dev_page_rank_);
      checkOpenCLErrors(err, "Failed to set kernel argument 5");

      err = clSetKernelArg(pr_kernel_, 6, sizeof(cl_mem), &dev_page_rank_temp_);
      checkOpenCLErrors(err, "Failed to set kernel argument 6");
    } else {
      err = clSetKernelArg(pr_kernel_, 5, sizeof(cl_mem), &dev_page_rank_temp_);
      checkOpenCLErrors(err, "Failed to set kernel argument 5");

      err = clSetKernelArg(pr_kernel_, 6, sizeof(cl_mem), &dev_page_rank_);
      checkOpenCLErrors(err, "Failed to set kernel argument 6");
    }

    size_t global_work_size[] = {num_nodes_ * 64};
    size_t local_work_size[] = {64};
    err = clEnqueueNDRangeKernel(cmd_queue_, pr_kernel_, 1, NULL,
                                 global_work_size, local_work_size, 0, NULL,
                                 NULL);
    checkOpenCLErrors(err, "Failed to launch kernel");
  }

  clFinish(cmd_queue_);
  cpu_gpu_logger_->GPUOff();

  if (!i % 2 == 0) {
    CopyDataBackFromDevice(&dev_page_rank_);
  } else {
    CopyDataBackFromDevice(&dev_page_rank_temp_);
  }
  cpu_gpu_logger_->Summarize();
}

void PrCl12Benchmark::Cleanup() {
  PrBenchmark::Cleanup();

  cl_int ret;
  ret = clReleaseKernel(pr_kernel_);
  ret = clReleaseProgram(program_);

  ret = clReleaseMemObject(dev_page_rank_);
  ret = clReleaseMemObject(dev_page_rank_temp_);
  ret = clReleaseMemObject(dev_row_offsets_);
  ret = clReleaseMemObject(dev_column_numbers_);
  ret = clReleaseMemObject(dev_values_);

  checkOpenCLErrors(ret, "Release objects.\n");
}
