# Configure Platforms

When performing a new experiment, you probably want to simulate a modified platform rather than the standard one. In this tutorial, we introduce how the plaform is built and how you can change it. For many detailed configuration of each individual component, you may need to refer to chapters describing those components.

## Platform Builder

In the [Prepare Experiments](prepare_experiments.md) chapter, we see that the platform is built with `BuildNR9NanoPlatform` function. This function servers as the configuration file of the platform. We do not use configuration files in the traditional sense for the following reasons:

First, configuration files are verbose. A configuration file, if all the aspects of the platform are defined, can be thousands of lines long. The long format makes users really hard to find the information to look at. Also, since the configuration is long, it is common to use a Python script to generate the configuration file. If a script is needed anyway, why not directly use the same language that the simulator uses?

Second, configuration files are error prone. You would be lucky if the simulator warns about a configuration that does not make sense. If there is some configuration that is not desired but still works, it would be really hard to find the error. With the configuration written in Go code, you can use loops and function calls to significantly reduce the line of the code in the configuation and you would also be able use a debugger to find errors.

Third, using code as configuration file gives us more flexibility. Suppose you want to dump traces from core 0-3 into one file and dump traces from core 4-7 into another file, it would be really difficult to use configuration format to express this. Using code, it is much easier by opening files in the builder and pass the file handle to the corresponding core struct.

Finally, by integrating all the code into one binary imrpoves the recreatability of the program. In case someone want to repeat your experiment, they do not need to find the undated simulator code, the benchmarks, and the configuration file separately.

So next, lets take a look at how to write the function that builds a platform, starting from the function signature. We directly use the example of building a multi-GPU system to demonstrate the power of the Akita GCN3's configuration system.

```go
func BuildNR9NanoPlatform(numGPUs int) (
    akita.Engine,
    *driver.Driver,
) {
```

The build function takes 1 argument as the number of GPUs to create and return 2 arguments as the event-driven simulation engine and the GPU driver. There is no requirement for the input and output argument for such build function and the user can define this function as desired.

Next, we create the event-driven simulation engine.

```go
    var engine akita.Engine
    engine = akita.NewSerialEngine()
```

For the platform under simulation, we need to define the parts that are shared by the multiple GPUs. We define an Memory Management Unit (MMU) that can perform the virtual address to physical address translation. In real hardware, the MMUs that serves the GPUs are usually called IOMMU and is a component located on the CPU chip. We also create a GPU driver. In a platform, there should be only one GPU driver that controlls all the GPUs. Also, we create a fixed-bandwidth connection (models the PCIe bus) that connects the CPU and the GPUs.

```go
    mmu := vm.NewMMU("MMU", engine, &vm.DefaultPageTableFactory{})
    mmu.Latency = 100
    gpuDriver := driver.NewDriver(engine, mmu)
    connection := noc.NewFixedBandwidthConnection(32, engine, 1*akita.GHz)
```

Next, we create the GPUs. Since each GPU are very complex internally, we do not directly creates them in the platform-building code, but we employ a GPUBuilder to create the GPUs.

```go
    gpuBuilder := gpubuilder.R9NanoGPUBuilder{
        GPUName:           "GPU",
        Engine:            engine,
        Driver:            gpuDriver,
        MMU:               mmu,
        ExternalConn:      connection,
    }

    rdmaAddressTable := new(cache.BankedLowModuleFinder)
    rdmaAddressTable.BankSize = 4 * mem.GB
    for i := 0; i < numGPUs; i++ {
        gpuBuilder.GPUName = fmt.Sprintf("GPU_%d", i)
        gpuBuilder.GPUMemAddrOffset = uint64(i) * 4 * mem.GB
        gpu := gpuBuilder.Build()
        gpuDriver.RegisterGPU(gpu, 4*mem.GB)
        gpu.Driver = gpuDriver.ToGPUs

        gpu.RDMAEngine.RemoteRDMAAddressTable = rdmaAddressTable
        rdmaAddressTable.LowModules = append(
            rdmaAddressTable.LowModules,
            gpu.RDMAEngine.ToOutside)
        connection.PlugIn(gpu.RDMAEngine.ToOutside)
    }
```

This piece of code is a little bit complex, but lets dissect the code. First, we create a GPU Builder with some arguments. Then in the second half, we create GPUs in a for loop. The key line is `gpu := gpuBuilder.Build()`. We register the GPU in the driver with `gpuDriver.RegisterGPU` so that the driver can send command to the GPU. The `RegisterGPU` function takes two arguments as the GPU itself and the size of the memory that the GPU has. We also link the driver with the GPU with `gpu.Driver = gpuDRiver.ToGPUs`, allowing the GPU to send request to the driver.

The rest of the code is related multi-GPU RDMA. We first create an RDMA address table, so that the RDMA unit knows which GPU owns a certain memory address range. Since each GPU has 4GB memory, we set the bank size to be 4GB. In the loop, after creating the GPU, we set the RDMA table to the RDMA engine. the RDMA engine itself needs to register to the RDMA address table. After the loop, we have the address table as follows:

| Addr Lo | Addr Hi |  GPU  |
| :-----: | :-----: | :---: |
|  0 GB   |  4 GB   | GPU 0 |
|  4 GB   |  8 GB   | GPU 1 |
|  8 GB   |  12 GB  | GPU 2 |
|  12 GB  |  16 GB  | GPU 3 |

Finally, we need to connect the components together. As you may see, we have already been connecting the RDMA engines using `connection.PlugIn(gpu.RDMAEngine.ToOutside)`. We also need the following code to connect the driver and the mmu.

```go
    connection.PlugIn(gpuDriver.ToGPUs)
    connection.PlugIn(mmu.ToTop)

    return engine, gpuDriver
}
```

At the end, we return the engine and the GPU Driver. Since the benchmark only communicates with the driver, we do not need to return the GPUs and any other components out. This guarantees a full decouple of the platform under simulation and the benchmark, making the benchmark can run on any platform.

## GPU Builder
