package platform_test

import (
	"fmt"
	"io"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/benchmark"
	"github.com/sarchlab/mgpusim/v4/nvidia/platform"
	"github.com/sarchlab/mgpusim/v4/nvidia/runner"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

var logFile = "testA100.log"

func TestA100PlatformWithActualData(t *testing.T) {
	setTestLogFile()

	benchmark := new(benchmark.BenchmarkBuilder).
		// WithTraceDirectory("../data/bfs-rodinia-2.0-ft").
		WithTraceDirectory("../data/simple-trace-example").
		Build()
	platform := new(platform.A100PlatformBuilder).
		WithFreq(1 * sim.GHz).
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

// func TestA100PlatformWithMockData(t *testing.T) {
// 	setTestLogFile()

// 	benchmark := generateMockBenchmark()
// 	platform := new(platform.A100PlatformBuilder).
// 		WithFreq(1 * sim.GHz).
// 		Build()
// 	runner := new(runner.RunnerBuilder).
// 		WithPlatform(platform).
// 		Build()
// 	runner.AddBenchmark(benchmark)
// 	runner.Run()

// 	theoreticalTotalInsts := calcTheoreticalTotalInsts(benchmark)
// 	actualTotalInsts := calcActualTotalInsts(platform)
// 	if theoreticalTotalInsts != actualTotalInsts {
// 		t.Errorf("Expected %d insts, got %d", theoreticalTotalInsts, actualTotalInsts)
// 	}
// }

func generateMockBenchmark() *benchmark.Benchmark {
	bm := new(benchmark.Benchmark)

	kernelCount := 10
	threadblockCount := 10
	warpCount := 10
	instructionsCount := 10

	inst := new(trace.InstructionTrace)
	warp := new(trace.WarpTrace)
	for i := 0; i < instructionsCount; i++ {
		// warp.InstructionsCount++
		warp.Instructions = append(warp.Instructions, inst)
	}
	tb := new(trace.ThreadblockTrace)
	for i := 0; i < warpCount; i++ {
		// tb.WarpsCount++
		tb.Warps = append(tb.Warps, warp)
	}
	kernel := new(trace.KernelTrace)
	for i := 0; i < threadblockCount; i++ {
		// kernel.ThreadblocksCount++
		kernel.Threadblocks = append(kernel.Threadblocks, tb)
	}
	exec := new(benchmark.ExecKernel)
	exec.SetKernel(*kernel)
	for i := 0; i < kernelCount; i++ {
		bm.TraceExecs = append(bm.TraceExecs, exec)
	}

	return bm
}

func setTestLogFile() {
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("Failed to open log file:", err)
	}
	multiWriter := io.MultiWriter(file, os.Stdout)

	log.SetOutput(multiWriter)
	log.SetLevel(log.DebugLevel)
}

func calcTheoreticalTotalInsts(bm *benchmark.Benchmark) uint64 {
	totalInstsCount := uint64(0)
	for _, exec := range bm.TraceExecs {
		if exec.ExecType() == trace.ExecKernel {
			r, ok := exec.(*benchmark.ExecKernel)
			if !ok {
				log.Error("cannot cast to ExecKernel")
				return 0
			}
			kernel := r.GetKernel()
			for _, tb := range kernel.Threadblocks {
				for _, warp := range tb.Warps {
					totalInstsCount += warp.InstructionsCount()
				}
			}
		}
	}
	return totalInstsCount
}

func calcTheoreticalTotalWarps(bm *benchmark.Benchmark) uint64 {
	totalWarpsCount := uint64(0)
	for _, exec := range bm.TraceExecs {
		if exec.ExecType() == trace.ExecKernel {
			r, ok := exec.(*benchmark.ExecKernel)
			if !ok {
				log.Error("cannot cast to ExecKernel")
				return 0
			}
			kernel := r.GetKernel()
			for _, tb := range kernel.Threadblocks {
				totalWarpsCount += tb.WarpsCount()
			}
		}
	}
	return totalWarpsCount
}

func calcActualTotalInsts(pf *platform.Platform) uint64 {
	totalInstsCount := uint64(0)
	fmt.Println("totalInstsCount := uint64(0)")
	for _, gpu := range pf.Devices {
		for _, sm := range gpu.SMs {
			for _, smsp := range sm.SMSPs {
				totalInstsCount += smsp.GetTotalInstsCount()
				// log.WithFields(log.Fields{
				// 	"gpu_id":  gpu.ID,
				// 	"sm_id":   sm.ID,
				// 	"smsp_id": smsp.ID,
				// 	"total":   smsp.GetTotalInstsCount(),
				// }).Warning("smsp status")
			}
			// totalInstsCount += sm.GetTotalInstsCount()
		}
	}

	return totalInstsCount
}

func calcActualTotalWarps(pf *platform.Platform) uint64 {
	totalWarpsCount := uint64(0)

	for _, gpu := range pf.Devices {
		for _, sm := range gpu.SMs {
			totalWarpsCount += sm.GetTotalWarpsCount()
		}
	}

	return totalWarpsCount
}
