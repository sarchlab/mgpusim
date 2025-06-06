package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

// EvalConfig is for JSON parsing
type EvalConfig struct {
	ScriptPath string      `json:"script_path"`
	Benchmarks []Benchmark `json:"benchmarks"`
}

type Benchmark struct {
	// ID    int         `json:"id"`
	Title string      `json:"title"`
	Suite string      `json:"suite"`
	Args  []ArgConfig `json:"args"`
}

type ArgConfig map[string]interface{}

func main() {
	var configType string
	flag.StringVar(&configType, "config", "dev", "config type: dev, test or release") // updated
	flag.Parse()                                                                      // updated

	var configPath string // updated
	switch configType {   // updated
	case "release": // updated
		configPath = "nvidia/eval/eval_config_release.json"
	case "test": // updated
		configPath = "nvidia/eval/eval_config_test.json"
	default: // updated
		configPath = "nvidia/eval/eval_config_dev.json"
	} // updated

	config := mustReadConfig(configPath)
	printEvalStats(config.Benchmarks)
	avgSEs, names := processBenchmarks(config) // updated
	printAvgSEs(avgSEs, names)                 // updated
}

func mustReadConfig(path string) EvalConfig {
	configFile, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open %s: %v\n", path, err)
		os.Exit(1)
	}
	defer configFile.Close()

	var config EvalConfig
	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to decode %s: %v\n", path, err)
		os.Exit(1)
	}
	return config
}

func processBenchmarks(config EvalConfig) ([]float64, []string) { // updated
	var avgSEs []float64
	var names []string // updated
	for _, bench := range config.Benchmarks {
		seList := processArgs(bench, config.ScriptPath)
		if len(seList) > 0 {
			avgSEs = append(avgSEs, average(seList))
			names = append(names, fmt.Sprintf("%s/%s", bench.Suite, bench.Title)) // updated
		}
	}
	return avgSEs, names // updated
}

func processArgs(bench Benchmark, scriptPath string) []float64 {
	seList := make([]float64, 0, len(bench.Args)) // pre-allocate
	for _, arg := range bench.Args {
		traceID, ok := arg["trace-id"].(string)
		if !ok {
			fmt.Fprintf(os.Stderr, "trace-id missing or not a string\n")
			continue
		}
		truthCycles := getTruthCycles(arg)
		tmpYamlPath := filepath.Join("nvidia/eval/", "tmp.yaml")
		if err := writeTmpYaml(tmpYamlPath, traceID); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write nvidia/eval/tmp.yaml: %v\n", err)
			continue
		}
		simResult := runSimulation(scriptPath, tmpYamlPath)
		if simResult == 0 || simResult == 1 {
			fmt.Fprintf(os.Stderr, "Error: simResult is %v (likely due to process exit code)\n", simResult)
			continue
		}
		seList = append(seList, symmetricError(truthCycles, simResult))
	}
	return seList
}

func printEvalStats(benchmarks []Benchmark) {
	suiteSet := make(map[string]struct{})
	numBenchmark := len(benchmarks)
	numTrace := 0

	for _, bench := range benchmarks {
		suiteSet[bench.Suite] = struct{}{}
		numTrace += len(bench.Args)
	}

	// fmt.Printf("num_suite: %d\n", len(suiteSet))
	// fmt.Printf("num_benchmark: %d\n", numBenchmark)
	// fmt.Printf("num_trace: %d\n", numTrace)

	fmt.Printf("%d\n", len(suiteSet))
	fmt.Printf("%d\n", numBenchmark)
	fmt.Printf("%d\n", numTrace)
}

func getTruthCycles(arg ArgConfig) float64 {
	if truth, ok := arg["truth"].(map[string]interface{}); ok {
		if cycles, ok := truth["cycles"].(float64); ok {
			return cycles
		}
	}
	return 0.0
}

// func main() {
// 	// Step 1: Read eval_config.json
// 	configFile, err := os.Open("nvidia/eval/eval_config.json")
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "Failed to open nvidia/eval/eval_config.json: %v\n", err)
// 		os.Exit(1)
// 	}
// 	defer configFile.Close()

// 	var config EvalConfig
// 	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
// 		fmt.Fprintf(os.Stderr, "Failed to decode nvidia/eval/eval_config.json: %v\n", err)
// 		os.Exit(1)
// 	}

// 	avgSEs := []float64{}

// 	// Step 2: For each benchmark and each arg setting
// 	for _, bench := range config.Benchmarks {
// 		seList := []float64{}
// 		for _, arg := range bench.Args {
// 			traceID, ok := arg["trace-id"].(string)
// 			if !ok {
// 				fmt.Fprintf(os.Stderr, "trace-id missing or not a string\n")
// 				continue
// 			}
// 			truthCycles := 0.0
// 			if truth, ok := arg["truth"].(map[string]interface{}); ok {
// 				if cycles, ok := truth["cycles"].(float64); ok {
// 					truthCycles = cycles
// 				}
// 			}

// 			// Step 2.1: Create eval/tmp.yaml
// 			tmpYamlPath := filepath.Join("nvidia/eval/", "tmp.yaml")
// 			err := writeTmpYaml(tmpYamlPath, traceID)
// 			if err != nil {
// 				fmt.Fprintf(os.Stderr, "Failed to write nvidia/eval/tmp.yaml: %v\n", err)
// 				continue
// 			}

// 			// Step 2.2: Run the external script
// 			cmd := exec.Command(
// 				"./mnt-collector", //config.ScriptPath, // "./mnt-collector"
// 				"simulations", "--collect", "../nvidia/eval/tmp.yaml",
// 			)
// 			cmd.Dir = "mnt-collector" // run from eval directory
// 			stdout, err := cmd.StdoutPipe()
// 			if err != nil {
// 				fmt.Fprintf(os.Stderr, "Failed to get stdout: %v\n", err)
// 				continue
// 			}
// 			cmd.Stderr = os.Stderr
// 			if err := cmd.Start(); err != nil {
// 				fmt.Println(stdout)
// 				fmt.Fprintf(os.Stderr, "Failed to start command: %v\n", err)
// 				continue
// 			}

// 			// Step 2.3: Parse the last float from the output
// 			simResult := parseLastFloat(stdout)
// 			cmd.Wait()

// 			// Check for invalid simResult
// 			if simResult == 0 || simResult == 1 {
// 				fmt.Fprintf(os.Stderr, "Error: simResult is %v (likely due to process exit code)\n", simResult)
// 				continue
// 			}

// 			// // Step 2.4: Print the result
// 			// argsStr := buildArgsString(arg)
// 			// fmt.Printf("(%v, %v) suite: '%s', title: '%s', args: %s\n",
// 			// 	truthCycles, simResult, bench.Suite, bench.Title, argsStr)

// 			// Step 2.5: Calculate symmetric error for this arg
// 			minVal := simResult
// 			if truthCycles < simResult {
// 				minVal = truthCycles
// 			}
// 			se := 999999.0
// 			if minVal > 0 {
// 				se = abs(simResult-truthCycles) / minVal
// 			}
// 			seList = append(seList, se)
// 		}
// 		// Average SE for this benchmark
// 		if len(seList) > 0 {
// 			sum := 0.0
// 			for _, v := range seList {
// 				sum += v
// 			}
// 			avg := sum / float64(len(seList))
// 			avgSEs = append(avgSEs, avg)
// 		}
// 	}
// 	// // Print the list of avg SEs
// 	// fmt.Print("[")
// 	// for i, v := range avgSEs {
// 	// 	if i > 0 {
// 	// 		fmt.Print(",")
// 	// 	}
// 	// 	fmt.Printf("%.6f", v)
// 	// }
// 	// fmt.Println("]")

// 	// Print the overall average
// 	if len(avgSEs) > 0 {
// 		sum := 0.0
// 		for _, v := range avgSEs {
// 			sum += v
// 		}
// 		fmt.Printf("%.6f\n", sum/float64(len(avgSEs)))
// 	}
// 	// println(99999)
// 	// os.Exit(0)
// }

// Write tmp.yaml based on the example, but with the correct trace-id
func writeTmpYaml(path, traceID string) error {
	content := `upload-to-server: false
experiment:
  version: "1.0"
  message: "base model"
  runfile: ../nvidia/nvidia

trace-id:
- %s
`
	return os.WriteFile(path, []byte(fmt.Sprintf(content, traceID)), 0644)
}

func runSimulation(scriptPath, tmpYamlPath string) float64 {
	cmd := exec.Command(
		"./mnt-collector",
		"simulations", "--collect", "../nvidia/eval/tmp.yaml",
	)
	cmd.Dir = "mnt-collector"

	// Print the full command for debugging
	// fmt.Printf("[mnt-collector] full command: '%s'\n", cmd.String())

	// Print the content of tmp.yaml for debugging
	// fmt.Printf("[mnt-collector] cat the tmp.yaml:\n")
	// content, err := os.ReadFile(tmpYamlPath)
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, "Failed to read %s: %v\n", tmpYamlPath, err)
	// } else {
	// 	fmt.Println(string(content))
	// }

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get stdout: %v\n", err)
		return 0
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start command: %v\n", err)
		return 0
	}
	result := parseLastFloat(stdout)
	cmd.Wait()
	return result
}

// Parse the last float from the command output
func parseLastFloat(r io.Reader) float64 {
	scanner := bufio.NewScanner(r)
	re := regexp.MustCompile(`[-+]?\d*\.\d+|\d+`)
	var lastFloat float64
	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindAllString(line, -1)
		if len(matches) > 0 {
			// Try to parse the last match as float
			for i := len(matches) - 1; i >= 0; i-- {
				var f float64
				_, err := fmt.Sscanf(matches[i], "%f", &f)
				if err == nil {
					lastFloat = f
					break
				}
			}
		}
	}
	return lastFloat
}

// // Build args string, excluding "trace-id" and "truth"
// func buildArgsString(arg ArgConfig) string {
// 	var sb strings.Builder
// 	sb.WriteString("{")
// 	first := true
// 	for k, v := range arg {
// 		if k == "trace-id" || k == "truth" {
// 			continue
// 		}
// 		if !first {
// 			sb.WriteString(", ")
// 		}
// 		first = false
// 		switch val := v.(type) {
// 		case float64:
// 			if val == float64(int(val)) {
// 				fmt.Fprintf(&sb, "'%s': %d", k, int(val))
// 			} else {
// 				fmt.Fprintf(&sb, "'%s': %v", k, val)
// 			}
// 		default:
// 			fmt.Fprintf(&sb, "'%s': %v", k, val)
// 		}
// 	}
// 	sb.WriteString("}")
// 	return sb.String()
// }

// Add this helper function:

func symmetricError(truth, sim float64) float64 {
	minVal := sim
	if truth < sim {
		minVal = truth
	}
	if minVal > 0 {
		return (sim - truth) / minVal
	}
	return 999999.0
}

func average(list []float64) float64 {
	sum := 0.0
	for _, v := range list {
		sum += v
	}
	return sum / float64(len(list))
}

func printAvgSEs(avgSEs []float64, names []string) { // updated
	// // Print the list of avg SEs
	// fmt.Print("[")
	// for i, v := range avgSEs {
	// 	if i > 0 {
	// 		fmt.Print(",")
	// 	}
	// 	fmt.Printf("%.6f", v)
	// }
	// fmt.Println("]")
	// Print the mapping from suite/benchmark to SE
	fmt.Print("{")             // updated
	for i, v := range avgSEs { // updated
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("\"%s\": %.6f", names[i], v)
	}
	fmt.Println("}") // updated
	// Print the overall average
	if len(avgSEs) > 0 {
		fmt.Printf("%.6f\n", average(avgSEs))
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
