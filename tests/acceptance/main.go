package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"

	"github.com/fatih/color"
)

var benchmarkFilter = flag.String("benchmark", "",
	`Regular expression for the benchmarks to run. Leaving it empty will run 
all the benchmarks.`)
var numGPU = flag.Int("num-gpu", 0,
	`Only run the benchmark cases with certain number of GPUs.`)
var onlyParallel = flag.Bool("only-parallel", false,
	`Only run the parallel benchmark cases.`)
var noParallel = flag.Bool("no-parallel", false,
	`Skip the parallel benchmark cases.`)
var onlyUnifiedMemory = flag.Bool("only-unified-memory", false,
	`Only run the unified memory benchmark cases.`)
var noUnifiedMemory = flag.Bool("no-unified-memory", false,
	`Skip the unified memory benchmark cases.`)
var onlyUnifiedGPU = flag.Bool("only-unified-gpu", false,
	`Only run the unified GPU benchmark cases.`)
var noUnifiedGPU = flag.Bool("no-unified-gpu", false,
	`Skip the unified GPU benchmark cases.`)

type benchmark struct {
	benchmarkPath  string
	executablePath string
	executable     string
	sizeArgs       []string
	cases          []benchmarkCase
}

type benchmarkCase struct {
	gpus          []int
	timing        bool
	unifiedGPU    bool
	unifiedMemory bool
	parallel      bool
}

func (b benchmark) compile() error {
	goExecutable, err := exec.LookPath("go")
	if err != nil {
		return err
	}

	cmd := &exec.Cmd{
		Path:   goExecutable,
		Dir:    b.executablePath,
		Args:   []string{"go", "build"},
		Stdout: os.Stdout,
		Stdin:  os.Stdin,
	}

	fmt.Print(cmd.String())

	if err := cmd.Run(); err != nil {
		color.Red("\tFailed")
		return err
	}

	color.Green("\tSucceed")
	return nil
}

func (b benchmark) runCase(c benchmarkCase) error {
	out, err := os.Create(b.executablePath + "/out.debug")
	if err != nil {
		return err
	}
	defer out.Close()

	args := b.populateArgs(c)

	cmd := &exec.Cmd{
		Path:   b.executable,
		Dir:    b.executablePath,
		Args:   args,
		Stdout: out,
		Stderr: out,
	}

	fmt.Print(cmd.String())

	if err := cmd.Run(); err != nil {
		color.Red("\tFailed\n")

		output, err := io.ReadAll(out)
		if err != nil {
			panic(err)
		}

		_, err = os.Stdout.Write(output)
		if err != nil {
			panic(err)
		}

		return err
	}

	color.Green("\tSucceed\n")
	return nil
}

func (b benchmark) populateArgs(c benchmarkCase) []string {
	args := append([]string{b.executable, "-verify"}, b.sizeArgs...)

	gpuArg := b.populateGPUArgs(c)
	args = append(args, gpuArg)

	if c.timing {
		args = append(args, "-timing=true")
	} else {
		args = append(args, "-timing=false")
	}

	if c.parallel {
		args = append(args, "-parallel=true")
	} else {
		args = append(args, "-parallel=false")
	}

	if c.unifiedMemory {
		args = append(args, "-use-unified-memory=true")
	} else {
		args = append(args, "-use-unified-memory=false")
	}

	return args
}

func (b benchmark) populateGPUArgs(c benchmarkCase) string {
	gpuArg := ""
	if c.unifiedGPU {
		gpuArg = "-unified-gpus="
	} else {
		gpuArg = "-gpus="
	}

	for i, g := range c.gpus {
		if i != 0 {
			gpuArg += ","
		}
		gpuArg += fmt.Sprint(g)
	}

	return gpuArg
}

func shouldRunBenchmark(b benchmark) bool {
	if *benchmarkFilter == "" {
		return true
	}

	re := regexp.MustCompile(*benchmarkFilter)

	return re.MatchString(b.executable)
}

//nolint:gocyclo
func shouldRunBenchmarkCase(b benchmark, c benchmarkCase) bool {
	if *numGPU != 0 && len(c.gpus) != *numGPU {
		return false
	}

	if *onlyParallel && !c.parallel {
		return false
	}

	if *noParallel && c.parallel {
		return false
	}

	if *onlyUnifiedGPU && !c.unifiedGPU {
		return false
	}

	if *noUnifiedGPU && c.unifiedGPU {
		return false
	}

	if *onlyUnifiedMemory && !c.unifiedMemory {
		return false
	}

	if *noUnifiedMemory && c.unifiedMemory {
		return false
	}

	return true
}

func run() {
	failed := false

	for _, b := range benchmarks {
		if !shouldRunBenchmark(b) {
			continue
		}

		err := b.compile()
		if err != nil {
			fmt.Println(err)
			failed = true
			continue
		}

		for _, c := range b.cases {
			if !shouldRunBenchmarkCase(b, c) {
				continue
			}

			err := b.runCase(c)
			if err != nil {
				fmt.Println(err)
				failed = true
			}
		}
	}

	if failed {
		os.Exit(2)
	}
}

func main() {
	flag.Parse()
	run()
}
