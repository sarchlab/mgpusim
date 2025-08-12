package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"

	"gonum.org/v1/gonum/stat"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
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

type Record struct {
	Suite        string            `json:"suite"`
	Benchmark    string            `json:"benchmark"`
	Param        map[string]string `json:"param"`
	HasTrace     bool              `json:"has_trace"`
	TraceID      string            `json:"trace_id"`
	HasProfile   bool              `json:"has_profile"`
	Frequency    interface{}       `json:"frequency"`
	AvgNanoSec   interface{}       `json:"avg_nano_sec"`
	HasSim       bool              `json:"has_sim"`
	PredictCycle float64           `json:"predict_cycle"`
}

func main() {
	var configType string
	var sha string
	flag.StringVar(&configType, "config", "dev", "config type: dev, test or release")
	flag.StringVar(&sha, "sha", "unknownSHA", "git commit SHA")
	flag.Parse()

	var configPath string
	switch configType {
	case "release":
		configPath = "nvidia/eval/eval_config_release.json"
	case "test":
		configPath = "nvidia/eval/eval_config_test.json"
	default:
		configPath = "nvidia/eval/eval_config_dev.json"
	}
	config := mustReadConfigAndAfterTurning(configPath)
	var allRecords []Record
	avgSEs, names, records := processBenchmarks(config)
	allRecords = append(allRecords, records...)
	printEvalStats(config.Benchmarks)
	printAvgSEs(avgSEs, names)

	rsqDir := "nvidia/eval/metrics"
	if err := os.MkdirAll(rsqDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create metrics dir: %v\n", err)
		return
	}
	truths, preds := extractCorrelationData(allRecords)
	r2 := handleCoefficientOfDetermination(truths, preds)
	pearson := handlePearsonCorrelationCoefficient(truths, preds)
	spearman := handleSpearmanRankCorrelation(truths, preds)

	fmt.Printf("%.6f\n", r2)
	fmt.Printf("%.6f\n", pearson)
	fmt.Printf("%.6f\n", spearman)
	shortSha := sha
	if sha != "unknownSHA" && len(sha) >= 7 {
		shortSha = sha[:7]
	}
	title := fmt.Sprintf("Evaluation Metrics of Commit %s (%d Pts)\n Coefficient of Determination R²=%.6f\nPearson Correlation Coefficient r=%.6f\nSpearman Rank Correlation Coefficient ρ=%.6f", shortSha, len(truths), r2, pearson, spearman)
	plotCorrelation(truths, preds, filepath.Join(rsqDir, sha+".png"), plotutil.Color(2), title)

	// Save records as JSON
	outDir := "nvidia/eval/records"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output dir: %v\n", err)
		os.Exit(1)
	}
	outPath := filepath.Join(outDir, sha+".json")
	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create %s: %v\n", outPath, err)
		os.Exit(1)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(allRecords); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write JSON: %v\n", err)
		os.Exit(1)
	}
	// fmt.Printf("Saved records to %s\n", outPath)
}

func extractCorrelationData(allRecords []Record) (truths, preds []float64) {
	for _, rec := range allRecords {
		// Extract AvgNanoSec and convert to float64
		avgNanoSec, ok := rec.AvgNanoSec.(float64)
		if !ok {
			if tInt, ok := rec.AvgNanoSec.(int); ok {
				avgNanoSec = float64(tInt)
			} else {
				continue
			}
		}

		// Extract Frequency and convert to float64
		frequency, ok := rec.Frequency.(float64)
		if !ok {
			if fInt, ok := rec.Frequency.(int); ok {
				frequency = float64(fInt)
			} else {
				continue
			}
		}

		// Calculate truth as (AvgNanoSec * 1e-3) * Frequency
		truth := (avgNanoSec * 1e-3) * frequency

		// Extract PredictCycle
		pred := rec.PredictCycle

		// Append to results
		truths = append(truths, truth)
		preds = append(preds, pred)
	}
	return truths, preds
}

func handleCoefficientOfDetermination(truths, preds []float64) (coeff float64) {
	r2 := rsquared(truths, preds)
	return r2
	// plotCorrelation(truths, preds, sha, r2, filepath.Join(rsqDir, sha+"_CoD.png"), plotutil.Color(2), "R²")
}

// Call this after saving allRecords
func handlePearsonCorrelationCoefficient(truths, preds []float64) (coeff float64) {
	pearson := stat.Correlation(truths, preds, nil)
	return pearson
	// plotCorrelation(truths, preds, sha, pearson, filepath.Join(rsqDir, sha+"_PCC.png"), plotutil.Color(3), "Pearson r")
}

func handleSpearmanRankCorrelation(truths, preds []float64) (coeff float64) {
	spearman := spearmanRank(truths, preds)
	return spearman
	// plotCorrelation(truths, preds, sha, spearman, filepath.Join(rsqDir, sha+"_SRC.png"), plotutil.Color(4), "Spearman ρ")
}

// Helper for plotting correlation
func plotCorrelation(truths, preds []float64, outPath string, scatterColor color.Color, title string) {
	p := plot.New()
	p.Title.Text = fmt.Sprintf("%s", title)
	// fmt.Printf("%s=%.6f\n", coeffName, coeff)
	p.X.Label.Text = "Truth Cycles (avg_nano_sec * frequency_nano)"
	p.Y.Label.Text = "Prediction Cycles (predict_cycle)"
	p.X.Min = 0
	p.Y.Min = 0

	pts := make(plotter.XYs, len(truths))
	for i := range truths {
		pts[i].X = truths[i]
		pts[i].Y = preds[i]
	}
	scatter, err := plotter.NewScatter(pts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create scatter: %v\n", err)
		return
	}
	r, g, b, a := scatterColor.RGBA()
	scatterColorAlpha := color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 9)}
	// scatterColorAlpha := color.RGBA{R: 0, G: 0, B: 255, A: uint8(128)}
	scatter.Color = scatterColorAlpha
	scatter.Radius = vg.Points(5)
	scatter.Shape = draw.CircleGlyph{}
	p.Add(scatter)

	// Add dashed y=x line
	minVal, maxVal := 0.0, 0.0
	for i, v := range truths {
		if i == 0 || v > maxVal {
			maxVal = v
		}
	}
	for _, v := range preds {
		if v > maxVal {
			maxVal = v
		}
	}
	linePts := plotter.XYs{
		{X: minVal, Y: minVal},
		{X: maxVal, Y: maxVal},
	}
	line, err := plotter.NewLine(linePts)
	if err == nil {
		line.LineStyle.Dashes = []vg.Length{vg.Points(5), vg.Points(5)}
		line.LineStyle.Width = vg.Points(1)
		line.LineStyle.Color = plotter.DefaultLineStyle.Color
		p.Add(line)
	}

	if err := p.Save(10*vg.Inch, 10*vg.Inch, outPath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save plot: %v\n", err)
	}
}

// R² calculation
func rsquared(truths, preds []float64) float64 {
	if len(truths) == 0 || len(truths) != len(preds) {
		return math.NaN()
	}
	mean := 0.0
	for _, t := range truths {
		mean += t
	}
	mean /= float64(len(truths))
	ssTot := 0.0
	ssRes := 0.0
	for i := range truths {
		ssTot += (truths[i] - mean) * (truths[i] - mean)
		ssRes += (truths[i] - preds[i]) * (truths[i] - preds[i])
	}
	if ssTot == 0 {
		return math.NaN()
	}
	return 1 - ssRes/ssTot
}

// Spearman rank correlation calculation
func spearmanRank(x, y []float64) float64 {
	if len(x) != len(y) || len(x) == 0 {
		return math.NaN()
	}
	rx := rank(x)
	ry := rank(y)
	return stat.Correlation(rx, ry, nil)
}

func rank(data []float64) []float64 {
	type kv struct {
		Value float64
		Index int
	}
	n := len(data)
	sorted := make([]kv, n)
	for i, v := range data {
		sorted[i] = kv{v, i}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Value < sorted[j].Value })
	ranks := make([]float64, n)
	for rank, kv := range sorted {
		ranks[kv.Index] = float64(rank + 1)
	}
	return ranks
}

func mustReadConfigAndAfterTurning(path string) EvalConfig {
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

	// Filter args for each benchmark: keep only beforeTurning == 0 if available
	for bi, bench := range config.Benchmarks {
		var zeroArgs []ArgConfig
		for _, arg := range bench.Args {
			if truth, ok := arg["truth"].(map[string]interface{}); ok {
				// Accept int or float
				if v, ok := truth["beforeTurning"]; ok {
					switch vv := v.(type) {
					case int:
						if vv == 0 {
							zeroArgs = append(zeroArgs, arg)
						}
					case float64:
						if int(vv) == 0 {
							zeroArgs = append(zeroArgs, arg)
						}
					}
				}
			}
		}
		config.Benchmarks[bi].Args = zeroArgs
	}
	return config
}

func processBenchmarks(config EvalConfig) ([]float64, []string, []Record) {
	var avgSEs []float64
	var names []string
	var allRecords []Record
	for _, bench := range config.Benchmarks {
		seList, records := processArgs(bench, config.ScriptPath)
		allRecords = append(allRecords, records...)
		if len(seList) > 0 {
			avgSEs = append(avgSEs, average(seList))
			names = append(names, fmt.Sprintf("%s/%s", bench.Suite, bench.Title))
		}
	}
	return avgSEs, names, allRecords
}

func processArgs(bench Benchmark, scriptPath string) ([]float64, []Record) {
	seList := make([]float64, 0, len(bench.Args)) // pre-allocate
	records := make([]Record, 0, len(bench.Args))
	for _, arg := range bench.Args {
		traceID, ok := arg["trace-id"].(string)
		if !ok {
			fmt.Fprintf(os.Stderr, "trace-id missing or not a string\n")
			continue
		}
		// Build arg setting string (excluding "trace-id" and "truth")
		param := make(map[string]string)
		argStr := "{"
		first := true
		for k, v := range arg {
			if k == "trace-id" || k == "truth" || k == "frequency" {
				continue
			}
			if !first {
				argStr += ", "
			}
			first = false
			argStr += fmt.Sprintf("\"%s\": %v", k, v)
			param[k] = fmt.Sprintf("%v", v)
		}
		argStr += "}"
		frequency := arg["frequency"]
		fmt.Printf("trace-id: %s, suite: %s, frequency: %v, benchmark: %s, arg setting: %s\n", traceID, bench.Suite, frequency, bench.Title, argStr)

		truthCycles := getTruthCycles(arg)
		tmpYamlPath := filepath.Join("nvidia/eval/", "tmp.yaml")
		if err := writeTmpYaml(tmpYamlPath, traceID); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write nvidia/eval/tmp.yaml: %v\n", err)
			continue
		}
		tmpDir := "mnt-collector/tmp"
		if _, err := os.Stat(tmpDir); err == nil {
			err := os.RemoveAll(tmpDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", tmpDir, err)
			}
		}
		matches, err := filepath.Glob("mnt-collector/logfile*")
		if err == nil {
			for _, f := range matches {
				err := os.Remove(f)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", f, err)
				}
			}
		}
		simResult, err := runSimulation(scriptPath, tmpYamlPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error during simulation: %v\n", err)
			// fmt.Printf("Skipping trace-id %s due to error\n", traceID)
			continue // Skip this iteration if an error occurs
		}
		if simResult == 0 || simResult == 1 {
			fmt.Fprintf(os.Stderr, "Error: simResult is %v (likely due to process exit code)\n", simResult)
			continue
		}
		seList = append(seList, symmetricError(truthCycles, simResult))

		rec := Record{
			Suite:        bench.Suite,
			Benchmark:    bench.Title,
			Param:        param,
			HasTrace:     true,
			TraceID:      traceID,
			HasProfile:   true,
			Frequency:    frequency,
			AvgNanoSec:   truthCycles,
			HasSim:       true,
			PredictCycle: simResult,
		}
		records = append(records, rec)
	}
	return seList, records
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
	fmt.Printf("The following lines mean: (1) #suite; (2) #benchmark; (3) #trace; (4) SE distribution (5) avgSEs (6) Coefficient of Determination R² (7) Pearson r (8) Spearman ρ\n")

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
	// fmt.Printf("trace-id: %s\n", traceID)
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

func runSimulation(scriptPath, tmpYamlPath string) (float64, error) {
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
		return 0, fmt.Errorf("failed to get stdout: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start command: %v\n", err)
		return 0, fmt.Errorf("failed to start command: %v", err)
	}
	result := parseLastFloat(stdout)
	cmd.Wait()
	return result, nil
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

func printAvgSEs(avgSEs []float64, names []string) {
	fmt.Print("{")
	for i, v := range avgSEs {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("\"%s\": %.6f", names[i], v)
	}
	fmt.Println("}")
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
