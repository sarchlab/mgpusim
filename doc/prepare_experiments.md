# Prepare Experiments

This tutorial introduce how to prepare an experiment running on Akita GCN3.

## Create a New Repo

Suppose you want to perform a new set of experiment with Akita GCN3, you may probably want to create another git repository and create your experiment source code here. Using separate repositories has 2 advantages. First, we can have a clear boundry of what is specific to the experiment and what is general to the simulator that everyone should use. Second, this approach creates more repeatable experiments as all the platform-related and benchmark-related parameters are defined in your program. You can even use "go vendor" and "git submodule" to create fully recreatable experiment by embedding the whole simulator in your repository.

## The Experiment Code

Here is full list of the example that runs the fir benchmark.

```go
package main

import (
    "flag"

    "gitlab.com/akita/gcn3/benchmarks/heteromark/fir"
    "gitlab.com/akita/gcn3/samples/runner"
)

func main() {
    runner := runner.Runner{}
    runner.Init()

    benchmark := fir.NewBenchmark(runner.GPUDriver)
    benchmark.Length = 4096
    runner.Benchmark = benchmark
    runner.Run()
}

```

As you may see, at the beginning of the `main` function, we create a runner that runs the simulator. As in the official examples, we use the same logic to run all the benchmarks, we abstract the logic in `Runner`. We will discuss the detail of the runner soon.

After creating and initializing the runner, we instantiate the benchmark. For how to create a benchmark, please refer to [Prepare Benchmarks](./create_benchmarks.md). We set the argument of the benchmark with `benchmark.Length = 4096` before we assocate the benchmark with the runner, using `runner.Benchmark = benchmark`.

## Runner

Next, let's take a look at how the `Runner` is implemented. In general, if you want to use the same way to run multiple benchmarks, writing a runner is recommended. Here is a simplified version of a runner:

```go
type Runner struct {
    engine            akita.Engine
    GPUDriver         *driver.Driver
    Benchmark         benchmarks.Benchmark
}

func (r *Runner) Init() {
    r.engine, r.GPUDriver = platform.BuildNR9NanoPlatform(1)
}

func (r *Runner) Run() {
    r.Benchmark.Run()
    r.engine.Finished()
}
```

As you may observed, the `Runner` code is also extremely simple. It maintains 3 fields including the Akita event-driven simulation engine, the GPU driver, and the benchmark to run.

In the `Init` function, an engine and a GPU driver is created, using the `BuildNR9NanoPlatform` function. This function creates a certain number (in this example, only one) of R9 Nano GPUs under the hood, and returns the GPU driver that can control the GPUs. The benchmark does not directly communicate with the GPUs, but only communicate with the driver, similar to how you would write a benchmark for a real GPU platform.

In the `Run` function, it runs the whole benchmark and calls `engine.Finished` to process some end-simulation actions.