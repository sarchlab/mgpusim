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
var onlyTiming = flag.Bool("only-timing", false,
	`Only run the timing benchmark cases.`)
var noTiming = flag.Bool("no-timing", false,
	`Skip the timing benchmark cases.`)


type benchmark struct {
	benchmarkPath  string
	executablePath string
	executable     string
	sizeArgs       []string
	cases          []benchmarkCase
}

type benchmarkCase struct {
	gpus     []int
	timing   bool
	parallel bool
	gpuType  string // GPU model: "mi300a" (default)
}

// generateCases returns the standard test matrix: all combinations of
// {timing} x {parallel} x {gpus}.
func generateCases() []benchmarkCase {
	var cases []benchmarkCase
	gpuSets := [][]int{{1}, {1, 2}, {1, 2, 3, 4}}
	for _, timing := range []bool{false, true} {
		for _, parallel := range []bool{false, true} {
			for _, gpus := range gpuSets {
				cases = append(cases, benchmarkCase{
					gpus:     gpus,
					timing:   timing,
					parallel: parallel,
				})
			}
		}
	}
	return cases
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
	stdout, err := os.Create(b.executablePath + "/stdout.debug")
	if err != nil {
		return err
	}
	defer stdout.Close()

	stderr, err := os.Create(b.executablePath + "/stderr.debug")
	if err != nil {
		return err
	}
	defer stderr.Close()

	args := b.populateArgs(c)

	cmd := &exec.Cmd{
		Path:   b.executable,
		Dir:    b.executablePath,
		Args:   args,
		Stdout: stdout,
		Stderr: stderr,
	}

	fmt.Print(cmd.String())

	if execErr := cmd.Run(); execErr != nil {
		color.Red("\tFailed\n")

		fmt.Printf("\nError: %v\n", execErr)

		// Print stderr content for debugging
		stderr.Seek(0, io.SeekStart)
		stderrContent, _ := io.ReadAll(stderr)
		if len(stderrContent) > 0 {
			fmt.Printf("\n--- stderr ---\n%s\n--- end stderr ---\n", string(stderrContent))
		}

		return execErr
	}

	// stdout content must be empty
	stdout.Seek(0, io.SeekStart)
	stdoutContent, err := io.ReadAll(stdout)
	if err != nil {
		return err
	}

	if len(stdoutContent) != 0 {
		color.Red("\tFailed, stdout is not empty\n")
		return fmt.Errorf("stdout is not empty")
	}

	color.Green("\tSucceed\n")
	return nil
}

func (b benchmark) populateArgs(c benchmarkCase) []string {
	args := append(
		[]string{b.executable, "-verify", "--report-all"},
		b.sizeArgs...,
	)

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

	if c.gpuType != "" {
		args = append(args, "-gpu="+c.gpuType)
	}

	return args
}

func (b benchmark) populateGPUArgs(c benchmarkCase) string {
	gpuArg := "-gpus="

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

	if *onlyTiming && !c.timing {
		return false
	}

	if *noTiming && c.timing {
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

		cases := b.cases
		if len(cases) == 0 {
			cases = generateCases()
		}
		for _, c := range cases {
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
