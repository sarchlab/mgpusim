package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/sarchlab/mgpusim/v3/timing/rob"
	"github.com/sarchlab/mgpusim/v3/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
)

var numData = flag.Int("length", 4096, "The number of samples to filter.")

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	benchmark := fir.NewBenchmark(runner.Driver())
	benchmark.Length = *numData

	runner.AddBenchmark(benchmark)

	runner.Run()
	err := rob.GlobalMilestoneManager.ExportMilestonesToCSV("../samples/fir/milestones.csv")
    if err != nil {
        log.Fatalf("Error exporting milestones: %v", err)
    }
    fmt.Println("Milestones exported successfully")
}