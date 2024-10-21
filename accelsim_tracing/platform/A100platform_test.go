package platform_test

import (
	"log"
	"os"
	"testing"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/benchmark"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/platform"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/runner"
)

var logFile = "testA100.log"

func TestA100PlatformWithActualData(t *testing.T) {
	setTestLogFile()

	benchmark := new(benchmark.BenchmarkBuilder).
		// WithTraceDirectory("../data/bfs-rodinia-2.0-ft").
		WithTraceDirectory("../data/simple-trace-example").
		Build()
	platform := new(platform.A100PlatformBuilder).
		WithFreq(1 * sim.Hz).
		Build()
	runner := new(runner.RunnerBuilder).
		WithPlatform(platform).
		Build()
	runner.AddBenchmark(benchmark)
	runner.Run()

	theoreticalTotalWarps := calcTheoreticalTotalWarps(benchmark)
	actualTotalWarps := calcActualTotalWarps(platform)
	if theoreticalTotalWarps != actualTotalWarps {
		t.Errorf("Expected %d warps, got %d", theoreticalTotalWarps, actualTotalWarps)
	}

	theoreticalTotalInsts := calcTheoreticalTotalInsts(benchmark)
	actualTotalInsts := calcActualTotalInsts(platform)
	if theoreticalTotalInsts != actualTotalInsts {
		t.Errorf("Expected %d insts, got %d", theoreticalTotalInsts, actualTotalInsts)
	}
}

func TestA100PlatformWithMockData(t *testing.T) {
	setTestLogFile()

	benchmark := generateMockBenchmark()
	platform := new(platform.A100PlatformBuilder).
		WithFreq(1 * sim.Hz).
		Build()
	runner := new(runner.RunnerBuilder).
		WithPlatform(platform).
		Build()
	runner.AddBenchmark(benchmark)
	runner.Run()

	theoreticalTotalInsts := calcTheoreticalTotalInsts(benchmark)
	actualTotalInsts := calcActualTotalInsts(platform)
	if theoreticalTotalInsts != actualTotalInsts {
		t.Errorf("Expected %d insts, got %d", theoreticalTotalInsts, actualTotalInsts)
	}
}

func generateMockBenchmark() *benchmark.Benchmark {
	bm := new(benchmark.Benchmark)

	kernelCount := 10
	threadblockCount := 10
	warpCount := 10
	instructionsCount := 10

	inst := new(nvidia.Instruction)
	warp := new(nvidia.Warp)
	for i := 0; i < instructionsCount; i++ {
		warp.InstructionsCount++
		warp.Instructions = append(warp.Instructions, *inst)
	}
	tb := new(nvidia.Threadblock)
	for i := 0; i < warpCount; i++ {
		tb.WarpsCount++
		tb.Warps = append(tb.Warps, *warp)
	}
	kernel := new(nvidia.Kernel)
	for i := 0; i < threadblockCount; i++ {
		kernel.ThreadblocksCount++
		kernel.Threadblocks = append(kernel.Threadblocks, *tb)
	}
	exec := new(benchmark.ExecKernel)
	exec.SetKernel(*kernel)
	for i := 0; i < kernelCount; i++ {
		bm.TraceExecs = append(bm.TraceExecs, exec)
	}

	return bm
}

func setTestLogFile() {
	file, err := os.Create(logFile)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	log.SetOutput(file)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func calcTheoreticalTotalInsts(bm *benchmark.Benchmark) int64 {
	totalInstsCount := int64(0)
	for _, exec := range bm.TraceExecs {
		if exec.ExecType() == nvidia.ExecKernel {
			r, ok := exec.(*benchmark.ExecKernel)
			if !ok {
				log.Printf("cannot cast to ExecKernel")
				return 0
			}
			kernel := r.GetKernel()
			for _, tb := range kernel.Threadblocks {
				for _, warp := range tb.Warps {
					totalInstsCount += warp.InstructionsCount
				}
			}
		}
	}
	return totalInstsCount
}

func calcTheoreticalTotalWarps(bm *benchmark.Benchmark) int64 {
	totalWarpsCount := int64(0)
	for _, exec := range bm.TraceExecs {
		if exec.ExecType() == nvidia.ExecKernel {
			r, ok := exec.(*benchmark.ExecKernel)
			if !ok {
				log.Printf("cannot cast to ExecKernel")
				return 0
			}
			kernel := r.GetKernel()
			for _, tb := range kernel.Threadblocks {
				totalWarpsCount += tb.WarpsCount
			}
		}
	}
	return totalWarpsCount
}

func calcActualTotalInsts(pf *platform.Platform) int64 {
	totalInstsCount := int64(0)

	for _, gpu := range pf.Devices {
		for _, sm := range gpu.SMs {
			for _, subcore := range sm.Subcores {
				totalInstsCount += subcore.GetTotalInstsCount()
			}
			// totalInstsCount += sm.GetTotalInstsCount()
		}
	}

	return totalInstsCount
}

func calcActualTotalWarps(pf *platform.Platform) int64 {
	totalWarpsCount := int64(0)
	// totalSubcoresCount := 0

	for _, gpu := range pf.Devices {
		for _, sm := range gpu.SMs {
			totalWarpsCount += sm.GetTotalWarpsCount()
			// totalSubcoresCount += len(sm.Subcores)
		}
	}

	// fmt.Println("totalSubcoresCount: ", totalSubcoresCount)

	return totalWarpsCount
}
