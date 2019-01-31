# Prepare Benchmarks

In the previous tutorial, we discussed how to create an experiment running an existing benchmark on a pre-configured platform. In this tutorial, we introduce how to prepare a new benchmark and in the next tutorial, we introduce how to configure the platform as desired.

## Prepare HSACO From OpenCL

HSACO stands for Heterogeneous System Architecture (HSA) Code Object. It is the binary format that is supported by the Radeon Open Compute Platform (ROCm). Akita GCN3 support unmodified HSACO file for simulation.

To generate an HSACO file from an OpenCL source code file, `clang-ocl` is required. `clang-ocl` is shipped with ROCm installation and you should be able to find it at `/opt/rocm/bin`. You do not need to install the full version of ROCm, but you can install the `rocm-dev` instead, which simply install the compilation tools and will not install the GPU drivers.

Suppose the OpenCL file you want to compile is `kernels.cl`, you can run the following command to generate an HSACO:

```bash
clang-ocl -mcpu=gfx803 kernels.cl -O kernels.hsaco
```

Here, `gfx803` is the instruction set architecture~(ISA) that Akita GCN3 supports. In case you want to dump the human-readable assembly, you can slightly change the command above to:

```bash
clang-ocl -mcpu=gfx803 kernels.cl -S kernels.asm
```

As you may notice, `clang-ocl` add 3 extra arguments to the compiled kernel, including `HiddenGlobalOffsetX`, `HiddenGlobalOffsetY`, and `HiddenGlobalOffsetZ`. These fields may be helpful when we prepare benchmarks for multi-GPU execution. However, the use of these arguments should be very careful and for most of the time, only 0 should be passed to these fields.

## Prepare HSACO from Assembly

TODO

## Wrap the HSACO with `go-bindata`

As you have the HSACO binary, you can read the file and load the content into the memory. However, loading the file requires the user to make sure that the HSACO to be located at a certain path. To avoid this inconvenience, we use `go-bindata` to wrap the HSACO binary directly in the source code.

`go-bindata` provides a command line tool to wrap static files into go code. You need to install `go-bindata` with the command `go get -u github.com/jteeuwen/go-bindata/...`. Then, assuming your HSACO file is `kernels.hsaco`, running command `go-bindata kernels.hsaco` generates a `bindata.go` file.

The generated file has a package declaration of `main`. Since the benchmark you are preparing will eventually become a Go library, you would need to replace the package declaration the same as the name of the benchmark.

Users may notice that `go-bindata` has been deprecated by the original author. Replacing `go-bindata` with other similar tools is on our roadmap.

## A Benchmark Struct

For each benchmark, we need a Go program that serves as the host program that controls the GPU execution. A Benchmark is prepared as a struct. Here is an example of the struct definition of the FIR benchmark:

```go
type Benchmark struct {
    driver *driver.Driver
    hsaco  *insts.HsaCo

    Length       int
    numTaps      int
    inputData    []float32
    filterData   []float32
    gFilterData  driver.GPUPtr
    gHistoryData driver.GPUPtr
    gInputData   driver.GPUPtr
    gOutputData  driver.GPUPtr
}
```

The first argument is the driver of the benchmark. Driver serves as the API between the benchmark and the GPU simulator. The benchmark interacts with the driver to control the GPUs. The field `hsaco` is the HSACO binary and we will see how we can load it from the `go-bindata` wrapped code. The third field `Length` is for the benchmark runner to configure this benchmark. The rest of the fields are benchmark specific data. We will see how the fields are used later.

## Load HSACO

We usually load the HSACO while the benchmark is initiated. Continuing with the FIR example, we see that the "constructor" function of the benchmark struct is like this:

```go
func NewBenchmark(driver *driver.Driver) *Benchmark {
    b := new(Benchmark)

    b.driver = driver

    hsacoBytes, err := Asset("kernels.hsaco")
    if err != nil {
        log.Panic(err)
    }
    b.hsaco = kernels.LoadProgramFromMemory(hsacoBytes, "FIR")

    return b
}
```

The `NewBenchmark` function takes 1 argument, injecting the dependency of the GPU driver. After setting the driver, the benchmark loads the hsaco file with `Asset` function. The `Asset` function requires the file name the same as the HSACO file. The `Asset` function is equivalent to loading all bytes of the file into a buffer. Finally, we extract the kernel binary with `kernels.LoadProgramFromMemory` function. The first argument is the raw HSACO buffer and the second argument is the name of the kernel. Note that an HSACO file may include multiple kernels compiled. Using `LoadProgramFromMemory` function allows you to extract each individual function. However, for the kernels compiled from assembly, the kernel name information is not packed into the HSACO file. You can set the kernel name argument to be an empty string.

All the `Benchmark` struct must provide a `Run` function so that the runner can start the simulation. Usually, `Run` is composed of two steps, initializing the GPU memory and run the kernel.

```go
func (b *Benchmark) Run() {
    b.initMem()
    b.exec()
}
```

## Initialize GPU Memory

Before we run the GPU kernel, we need to send data to the GPU. Now, you will need to interact with the GPU driver in the `initMem` function. Connecting the code snippets in this section is the whole `initMem` function implementation.

```go
func (b *Benchmark) initMem() {
    b.numTaps = 16
```

The first step is to allocate memory on GPU using the `AllocateMemory` function. The `AllocateMemory` function takes the number of bytes to be allocated as an argument.

```go
    b.gFilterData = b.driver.AllocateMemory(uint64(b.numTaps * 4))
    b.gHistoryData = b.driver.AllocateMemory(uint64(b.numTaps * 4))
    b.gInputData = b.driver.AllocateMemory(uint64(b.Length * 4))
    b.gOutputData = b.driver.AllocateMemory(uint64(b.Length * 4))
```

Initializing the CPU data is in native Go style:

```go
    b.filterData = make([]float32, b.numTaps)
    for i := 0; i < b.numTaps; i++ {
        b.filterData[i] = float32(i)
    }

    b.inputData = make([]float32, b.Length)
    for i := 0; i < b.Length; i++ {
        b.inputData[i] = float32(i)
    }
```

Copying the data to the GPU is also as simple as follows:

```go
    b.driver.MemCopyH2D(b.gFilterData, b.filterData)
    b.driver.MemCopyH2D(b.gInputData, b.inputData)
```

In case you want to copy the data back from the GPU to the CPU, you simply need to replace the function name as `MemCopyD2H` and invert the argument order, putting the destination in front of the source.

```go
}
```

Note that when you run the `MemCopyH2D` function, the simulator already started detailed timing simulation and the memory copy time is calculated to the total execution time.

## Run a Kernel

Finally, we can run kernels on the GPU simulator. But before we launch the kernel, we need to formally define the kernel arguments as a struct. For example, the OpenCL kernel signature of the FIR kernel is as follows:

```opencl
__kernel void FIR(
    __global float* output,
    __global float* coeff,
    __global float* input,
    __global float* history,
    uint num_tap
)
```

Then, we can convert the arguments as a Go struct:

```go
type KernelArgs struct {
    Output              driver.GPUPtr
    Filter              driver.GPUPtr
    Input               driver.GPUPtr
    History             driver.GPUPtr
    NumTaps             uint32
    Padding             uint32
    HiddenGlobalOffsetX int64
    HiddenGlobalOffsetY int64
    HiddenGlobalOffsetZ int64
}
```

For global pointers, we convert the type to driver.GPUPtr. Each pointer is 8B long. For scalar arguments, we can simply set the corresponding type in Go. Note that in the Go struct, you need to avoid types like `int`. Such types may have various sizes on different platform and they make the serializer not working properly. Finally, we also append the added 3 hidden offsets fields with type int64. We need to add a 4-byte padding field before `HiddenGlobalOffsetX`. The rule is that if the field is 8 bytes in size, the offset of the field relative to the beginning of the kernel argument struct must be a multiple of 8. The names of the arguments do have to match the OpenCL kernel signature, but all of them have to be public struct fields (capitalized first letter).

Running the benchmark is as easy as follows:

```go
func (b *Benchmark) exec() {
    kernArg := KernelArgs{
        b.gOutputData,
        b.gFilterData,
        b.gInputData,
        b.gHistoryData,
        uint32(b.numTaps),
        0,
        0, 0, 0,
    }

    b.driver.LaunchKernel(
        b.hsaco,
        [3]uint32{uint32(b.Length), 1, 1},
        [3]uint16{256, 1, 1},
        &kernArg,
    )
}
```

In the code above, we first set the fields of the kernel arguments. Then we launch the kernel with `LaunchKernel` API. The `LaunchKernel` API takes the kernel HSACO as the first argument. The global grid size (in the unit of the number of work-items) and the work-group size as the second argument. The last argument is the pointer to the kernel arguments. The `LaunchKernel` function runs the kernel on the simulator and it will return when the kernel simulation is completed. Therefore, this function may run for a very long time.

## Verification

Verification is optional but strongly recommended. With a CPU verification that compares the output with the GPU output, a user would know that the simulator is at least functionally correct.