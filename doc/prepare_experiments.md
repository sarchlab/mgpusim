# Prepare Experiment

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

After creating and initializing the runner, we instantiate the benchmark. For how to create a benchmark, please refer to [Creating Benchmarks](./create_benchmarks.md). We set the argument of the benchmark with `benchmark.Length = 4096` before we assocate the benchmark with the runner, using `runner.Benchmark = benchmark`.
