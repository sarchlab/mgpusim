# Project Specification

## What do you want to build?

We need to support emulating a wide range of gfx942 hip kernels. This work is already partially started. Please complete the task.

## How do you consider the project is success?

We should be able to support all the benchmarks from shoc, polybench, Rodinia, parboil. But also find other berchmark suite. You can get the CUDA version and convert it to HIP. You can either rewrite the code yourself, or you can use hipify to convert the code. Then, for every benchmark, always write a go version of the code. So that you can compare the calculation results. There is a docker file so that you can use to compile the kernels. The eventual goal is that the emulated the kernel generate byte-level correct result. No need to work on timing simulation at this stage.
