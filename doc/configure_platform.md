# Configure Platforms

When performing a new experiment, you probably want to simulate a modified platform rather than the standard one. In this tutorial, we introduce how the platform is built and how you can change it. For many detailed configurations of each individual component, you may need to refer to chapters describing those components.

When you perform a new experiment and need to reconfigure the platform, It is recommended to write a new platform builder in your experiment repository. In case you want to change the configuration of GPU internal, you may also want to provide your own GPU builder in your repo. We introduce platform builders and GPU builders in the following sections.

## Platform Builder

In the [Prepare Experiments](prepare_experiments.md) chapter, we see that the platform is built with `BuildNR9NanoPlatform` function. This function serves as the configuration file of the platform. We do not use configuration files in the traditional sense for the following reasons:

First, configuration files are verbose. A configuration file, if all the aspects of the platform are defined, can be thousands of lines long. The long format makes users really hard to find the information to look at. Also, since the configuration is long, it is common to use a Python script to generate the configuration file. If a script is needed anyway, why not directly use the same language that the simulator uses?

Second, configuration files are error-prone. You would be lucky if the simulator warns about a configuration that does not make sense. If there is some configuration that is not desired but still works, it would be really hard to find the error. With the configuration written in Go code, you can use loops and function calls to significantly reduce the line of the code in the configuration and you would also be able to use a debugger to find errors.

Third, using code as configuration file gives us more flexibility. Suppose you want to dump traces from core 0-3 into one file and dump traces from core 4-7 into another file, it would be really difficult to use configuration format to express this. Using code, it is much easier by opening files in the builder and pass the file handle to the corresponding core struct.

Finally, by integrating all the code into one binary improves the repeatability of the program. In case someone wants to repeat your experiment, they do not need to find the updated simulator code, the benchmarks, and the configuration file separately.

So next, let's take a look at how to write the function that builds a platform, starting from the function signature. We directly use the example of building a multi-GPU system to demonstrate the power of the Akita GCN3's configuration system.

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

For the platform under simulation, we need to define the parts that are shared by the multiple GPUs. We define a Memory Management Unit (MMU) that can perform the virtual address to physical address translation. In real hardware, the MMUs that serves the GPUs are usually called IOMMU and is a component located on the CPU chip. We also create a GPU driver. In a platform, there should be only one GPU driver that controls all the GPUs. Also, we create a fixed-bandwidth connection (models the PCIe bus) that connects the CPU and the GPUs.

```go
    mmu := vm.NewMMU("MMU", engine, &vm.DefaultPageTableFactory{})
    mmu.Latency = 100
    gpuDriver := driver.NewDriver(engine, mmu)
    connection := noc.NewFixedBandwidthConnection(32, engine, 1*akita.GHz)
```

Next, we create the GPUs. Since each GPU are very complex internally, we do not directly create them in the platform-building code, but we employ a GPUBuilder to create the GPUs.

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

This piece of code is a little bit complex, but let's dissect the code. First, we create a GPU Builder with some arguments. Then in the second half, we create GPUs in a for loop. The key line is `gpu := gpuBuilder.Build()`. We register the GPU in the driver with `gpuDriver.RegisterGPU` so that the driver can send commands to the GPU. The `RegisterGPU` function takes two arguments as the GPU itself and the size of the memory that the GPU has. We also link the driver with the GPU with `gpu.Driver = gpuDriver.ToGPUs`, allowing the GPU to send requests to the driver.

The rest of the code is related multi-GPU RDMA. We first create an RDMA address table, so that the RDMA unit knows which GPU owns a certain memory address range. Since each GPU has 4GB memory, we set the bank size to be 4GB. In the loop, after creating the GPU, we set the RDMA table to the RDMA engine. the RDMA engine itself needs to register to the RDMA address table. After the loop, we have the address table as follows:

| Addr Lo | Addr Hi |  GPU  |
| :-----: | :-----: | :---: |
|  0 GB   |  4 GB   | GPU 0 |
|  4 GB   |  8 GB   | GPU 1 |
|  8 GB   |  12 GB  | GPU 2 |
|  12 GB  |  16 GB  | GPU 3 |

Finally, we need to connect the components together. As you may see, we have already been connecting the RDMA engines using `connection.PlugIn(gpu.RDMAEngine.ToOutside)`. We also need the following code to connect the driver and the MMU.

```go
    connection.PlugIn(gpuDriver.ToGPUs)
    connection.PlugIn(mmu.ToTop)

    return engine, gpuDriver
}
```

In the end, we return the engine and the GPU Driver. Since the benchmark only communicates with the driver, we do not need to return the GPUs and any other components out. This guarantees a full decoupling of the platform under simulation and the benchmark, making the benchmark can run on any platform.

## GPU Builder

When we build the platform, we use the GPU builder to hide the complexity of the internal of GPUs. In this section, we discuss how the GPU builder is implemented. A GPU builder is rather complex and involves details of different components. Therefore, you may use this section as a reference and only read the parts you care about.

The main `Build` function serve well as a table of content for this section.

```go
func (b *R9NanoGPUBuilder) Build() *gcn3.GPU {
    b.Freq = 1000 * akita.MHz
    b.InternalConn = akita.NewDirectConnection(b.Engine)

    b.GPU = gcn3.NewGPU(b.GPUName, b.Engine)

    b.buildCP()
    b.buildMemSystem()
    b.buildDMAEngine()
    b.buildRDMAEngine()
    b.buildCUs()

    b.InternalConn.PlugIn(b.GPU.ToCommandProcessor)
    b.InternalConn.PlugIn(b.DMAEngine.ToCP)
    b.InternalConn.PlugIn(b.DMAEngine.ToMem)
    b.ExternalConn.PlugIn(b.GPU.ToDriver)

    b.GPU.InternalConnection = b.InternalConn

    return b.GPU
}
```

In the beginning, we set the frequency of the GPU to be 1 GHz. We also initialize the internal connection that connects every component inside the GPU. Here, we use a simple direct connection, since inside the GPU, components are usually connected by wires directly and direct connection is good enough to model wires. We create the GPU object and all the subsystems next. Finally, we connect a few critical components to the GPU internal connection.

### Command Processor

We build the Command Processor (CP) and Asynchronous Compute Engine (ACE) first:

```go
func (b *R9NanoGPUBuilder) buildCP() {
    b.CP = gcn3.NewCommandProcessor(b.GPUName+".CommandProcessor", b.Engine)
    b.CP.Driver = b.GPU.ToCommandProcessor
    b.GPU.CommandProcessor = b.CP.ToDriver

    b.ACE = gcn3.NewDispatcher(b.GPUName+"Dispatcher", b.Engine,
        new(kernels.GridBuilderImpl))
    b.ACE.Freq = b.Freq
    b.CP.Dispatcher = b.ACE.ToCommandProcessor

    b.InternalConn.PlugIn(b.CP.ToDriver)
    b.InternalConn.PlugIn(b.CP.ToDispatcher)
    b.InternalConn.PlugIn(b.ACE.ToCommandProcessor)
    b.InternalConn.PlugIn(b.ACE.ToCUs)
}
```

Here, we simply instantiate the CP object and the ACE object here and set some fields here. We also connect all the ports of the CP and the ACE to the internal connection.

One thing needs to clarify is the relationship between the GPU and the CP. In real GPUs, the CP is the gateway that processes all the commands from the CPU side. It directly communicates with the PCIe bus. In the simulator, we abstract everything into a GPU. The GPU component serves as a facade and does not process any commands from the driver. The GPU component simply forwards the driver command to the CP to process.

### Memory System

The memory system is relatively complex. It includes memory controllers, cache units, and TLBs. If you are interested in each part, you can directly read the code and we will skip the details in this tutorial. Also, DMA and RDMA components are closely related to the memory system, and we will skip them too.

```go
func (b *R9NanoGPUBuilder) buildMemSystem() {
    b.buildMemControllers()
    b.buildTLBs()
    b.buildL2Caches()
    b.buildL1VCaches()
    b.buildL1SCaches()
    b.buildL1ICaches()
}
```

### Compute Units

The core parts of a GPU are the compute units (CU). Since CUs are complex internally, similar to GPU builders, we use CU builders to hide the complexity.

```go
func (b *R9NanoGPUBuilder) buildCUs() {
    cuBuilder := timing.NewBuilder()
    cuBuilder.Engine = b.Engine
    cuBuilder.Freq = b.Freq
    cuBuilder.Decoder = insts.NewDisassembler()
    cuBuilder.ConnToInstMem = b.InternalConn
    cuBuilder.ConnToScalarMem = b.InternalConn
    cuBuilder.ConnToVectorMem = b.InternalConn

    for i := 0; i < 64; i++ {
        cuBuilder.CUName = fmt.Sprintf("%s.CU%02d", b.GPUName, i)
        cuBuilder.InstMem = b.L1ICaches[i/4].ToCU
        cuBuilder.ScalarMem = b.L1SCaches[i/4].ToCU

        lowModuleFinderForCU := new(cache.SingleLowModuleFinder)
        lowModuleFinderForCU.LowModule = b.L1VCaches[i].ToCU
        cuBuilder.VectorMemModules = lowModuleFinderForCU

        cu := cuBuilder.Build()
        b.GPU.CUs = append(b.GPU.CUs, cu)
        b.ACE.RegisterCU(cu.ToACE)

        b.InternalConn.PlugIn(cu.ToACE)
    }
}
```

Before the for loop, we create the CU builder and set up the arguments. We initialize all the CUs with a for loop. Since there are 64 CUs in the R9 Nano GPU, we hardcode the number of CUs in the loop header. With these lines

```go
        cuBuilder.InstMem = b.L1ICaches[i/4].ToCU
        cuBuilder.ScalarMem = b.L1SCaches[i/4].ToCU
        lowModuleFinderForCU := new(cache.SingleLowModuleFinder)
        lowModuleFinderForCU.LowModule = b.L1VCaches[i].ToCU
        cuBuilder.VectorMemModules = lowModuleFinderForCU
```

we set the instruction cache, scalar cache, and vector cache that are associated with the CU. We build the CU with `cuBuilder.Build` and register the CU to the ACE and the GPU. Finally, we connect the CU's `ToACE` port with the internal connection.
