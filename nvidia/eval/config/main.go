package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type EvalConfig struct {
	ScriptPath string      `json:"script_path"`
	Benchmarks []Benchmark `json:"benchmarks"`
}

type Benchmark struct {
	Title string      `json:"title"`
	Suite string      `json:"suite"`
	Args  []ArgConfig `json:"args"`
}

type ArgConfig map[string]interface{}

type GroundTruthEntry struct {
	Suite     string
	Benchmark string
	Args      map[string]string
	Cycles    float64
	Line      int
}

func main() {
	// Example usage:
	folder := "../../../mnt-collector/etc/"
	files := []string{
		"polybench/2dconv-simulations.yaml",
		"polybench/2mm-simulations.yaml",
		"polybench/3dconv-simulations.yaml",
		"polybench/3mm-simulations.yaml",
		"polybench/atax-simulations.yaml",
		"polybench/bicg-simulations.yaml",
		"polybench/gemm-simulations.yaml",
		"polybench/gesummv-simulations.yaml",
		"polybench/mvt-simulations.yaml",
		"polybench/syrk-simulations.yaml",
		"cuda-sdk/fastwalshtransform-simulations.yaml",
		"cuda-sdk/mergesort-simulations.yaml",
		"cuda-sdk/scalarprod-simulations.yaml",
		"cuda-sdk/scan-long-simulations.yaml",
		"cuda-sdk/scan-short-simulations.yaml",
		"cuda-sdk/sortingnetworks-bitonic-simulations.yaml",
		"cuda-sdk/sortingnetworks-oddeven-simulations.yaml",
		"cuda-sdk/transpose-simulations.yaml",
		"cuda-sdk/vectoradd-simulations.yaml",
		// "rodinia/b+tree-simulations.yaml",
	}
	groundTruthPath := "./ground_truth.txt"
	outputPathRelease := "../eval_config_release.json"
	outputPathDev := "../eval_config_dev.json"

	groundTruth, err := parseGroundTruth(groundTruthPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing ground truth: %v\n", err)
		os.Exit(1)
	}

	benchmarks := []Benchmark{}
	missingTruth := []string{}

	for _, relPath := range files {
		fullPath := filepath.Join(folder, relPath)
		suite, benchmark := parseSuiteBenchmark(relPath)
		args, missing := parseSimYaml(fullPath, suite, benchmark, groundTruth)
		benchmarks = append(benchmarks, Benchmark{
			Title: benchmark,
			Suite: suite,
			Args:  args,
		})
		missingTruth = append(missingTruth, missing...)
	}

	if len(missingTruth) > 0 {
		fmt.Println("Missing ground truth entries for:")
		for _, m := range missingTruth {
			fmt.Println(m)
		}
	}

	// Write eval_release.json as before
	releaseConfig := EvalConfig{
		ScriptPath: "./mnt-collector",
		Benchmarks: benchmarks,
	}
	releaseCount := countArgs(benchmarks)                                      // updated
	writeEvalConfig(outputPathRelease, releaseConfig)                          // updated
	fmt.Printf("Wrote %s: %d arg settings\n", outputPathRelease, releaseCount) // updated

	// Generate eval_dev.json (keep only 3 greatest arg settings per benchmark)
	devBenchmarks := []Benchmark{}     // updated
	for _, bench := range benchmarks { // updated
		devArgs := selectMiddleArgsForDev(bench.Args) // updated
		devBenchmarks = append(devBenchmarks, Benchmark{
			Title: bench.Title,
			Suite: bench.Suite,
			Args:  devArgs,
		})
	}
	devConfig := EvalConfig{
		ScriptPath: "./mnt-collector",
		Benchmarks: devBenchmarks,
	}
	devCount := countArgs(devBenchmarks)                               // updated
	writeEvalConfig(outputPathDev, devConfig)                          // updated
	fmt.Printf("Wrote %s: %d arg settings\n", outputPathDev, devCount) // updated
}

// --- Helper functions ---

// Helper to write config to file // updated
func writeEvalConfig(path string, config EvalConfig) { // updated
	outf, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output: %v\n", err)
		os.Exit(1)
	}
	defer outf.Close()
	enc := json.NewEncoder(outf)
	enc.SetIndent("", "    ")
	if err := enc.Encode(config); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
}

// Helper to count total arg settings // updated
func countArgs(benches []Benchmark) int { // updated
	count := 0
	for _, b := range benches {
		count += len(b.Args)
	}
	return count
}

func parseSuiteBenchmark(path string) (suite, benchmark string) {
	parts := strings.Split(path, string(os.PathSeparator))
	if len(parts) < 2 {
		return "unknown", "unknown"
	}
	suite = parts[len(parts)-2]
	base := filepath.Base(path)
	benchmark = strings.TrimSuffix(base, "-simulations.yaml")
	return suite, benchmark
}

func parseSimYaml(path, suite, benchmark string, gt map[string]GroundTruthEntry) ([]ArgConfig, []string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open %s: %v\n", path, err)
		return nil, nil
	}
	defer f.Close()

	type traceEntry struct {
		TraceID string
		Comment string
	}
	var traces []traceEntry

	scanner := bufio.NewScanner(f)
	traceLine := regexp.MustCompile(`^\s*-\s*([a-f0-9]+)\s*#\s*(.*)$`)
	inTrace := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "trace-id:") {
			inTrace = true
			continue
		}
		if inTrace {
			if strings.TrimSpace(line) == "" || !strings.HasPrefix(strings.TrimSpace(line), "-") {
				break
			}
			m := traceLine.FindStringSubmatch(line)
			if m != nil {
				traces = append(traces, traceEntry{
					TraceID: m[1],
					Comment: m[2],
				})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
	}

	var args []ArgConfig
	var missing []string
	for _, t := range traces {
		argMap := parseArgsFromComment(t.Comment)
		gtKey := groundTruthKey(suite, benchmark, argMap)
		entry, ok := gt[gtKey]
		if !ok {
			missing = append(missing, fmt.Sprintf("%s/%s %v", suite, benchmark, argMap))
		}
		arg := ArgConfig{}
		for k, v := range argMap {
			// try to convert to int if possible
			if ival, err := parseMaybeInt(v); err == nil {
				arg[k] = ival
			} else {
				arg[k] = v
			}
		}
		arg["trace-id"] = t.TraceID
		if ok {
			arg["truth"] = map[string]interface{}{"cycles": entry.Cycles}
		}
		args = append(args, arg)
	}
	return args, missing
}

func parseArgsFromComment(comment string) map[string]string {
	// comment example: "polybench / 2mm / -size: 32, -blockDimX: 8"
	argMap := make(map[string]string)
	parts := strings.Split(comment, "/")
	if len(parts) < 3 {
		return argMap
	}
	argStr := strings.TrimSpace(parts[len(parts)-1])
	argPairs := strings.Split(argStr, ",")
	for _, pair := range argPairs {
		pair = strings.TrimSpace(pair)
		if strings.HasPrefix(pair, "-") {
			kv := strings.SplitN(pair[1:], ":", 2)
			if len(kv) == 2 {
				argMap[kv[0]] = strings.TrimSpace(kv[1])
			}
		}
	}
	return argMap
}

func parseMaybeInt(s string) (interface{}, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	if err == nil {
		return i, nil
	}
	var f float64
	_, err = fmt.Sscanf(s, "%f", &f)
	if err == nil {
		return f, nil
	}
	return s, errors.New("not a number")
}

func parseGroundTruth(path string) (map[string]GroundTruthEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gt := make(map[string]GroundTruthEntry)
	scanner := bufio.NewScanner(f)
	lineNum := 0
	conflicts := []string{}
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 4 {
			continue
		}
		suite := strings.TrimSpace(strings.Split(parts[0], "=")[1])
		benchmark := strings.TrimSpace(strings.Split(parts[1], "=")[1])
		args := make(map[string]string)
		for _, p := range parts[2 : len(parts)-1] {
			kv := strings.SplitN(p, "=", 2)
			if len(kv) == 2 {
				args[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
		cyclesStr := strings.TrimSpace(strings.Split(parts[len(parts)-1], "=")[1])
		var cycles float64
		fmt.Sscanf(cyclesStr, "%f", &cycles)
		key := groundTruthKey(suite, benchmark, args)
		if _, exists := gt[key]; exists {
			conflicts = append(conflicts, fmt.Sprintf("Duplicate at line %d: %s", lineNum, line))
		}
		gt[key] = GroundTruthEntry{
			Suite:     suite,
			Benchmark: benchmark,
			Args:      args,
			Cycles:    cycles,
			Line:      lineNum,
		}
	}
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("conflicts in ground truth:\n%s", strings.Join(conflicts, "\n"))
	}
	return gt, scanner.Err()
}

func groundTruthKey(suite, benchmark string, args map[string]string) string {
	// Key: suite|benchmark|k1=v1|k2=v2|... (sorted by key)
	keys := []string{}
	for k := range args {
		keys = append(keys, k)
	}
	// sort keys for stable key
	sortStrings(keys)
	sb := strings.Builder{}
	sb.WriteString(suite)
	sb.WriteString("|")
	sb.WriteString(benchmark)
	for _, k := range keys {
		sb.WriteString("|")
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(args[k])
	}
	return sb.String()
}

func sortStrings(a []string) {
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			if a[j] < a[i] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

// Select the 3 "middle" arg settings for dev set, with "size" as highest priority if present // updated
func selectMiddleArgsForDev(args []ArgConfig) []ArgConfig { // updated
	if len(args) <= 3 {
		return args
	}
	// Build sort keys: "size" first (if present), then other numeric keys alphabetically
	type argWithSort struct {
		Arg      ArgConfig
		SortKeys []float64
	}
	var argList []argWithSort
	for _, arg := range args {
		var keys []string
		hasSize := false
		for k := range arg {
			if k == "trace-id" || k == "truth" {
				continue
			}
			if k == "size" {
				hasSize = true
			} else {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		finalKeys := []string{}
		if hasSize {
			finalKeys = append(finalKeys, "size")
		}
		finalKeys = append(finalKeys, keys...)

		var sortVec []float64
		for _, k := range finalKeys {
			switch v := arg[k].(type) {
			case int:
				sortVec = append(sortVec, float64(v))
			case float64:
				sortVec = append(sortVec, v)
			default:
				// ignore non-numeric
			}
		}
		argList = append(argList, argWithSort{Arg: arg, SortKeys: sortVec})
	}
	// Sort: lexicographically by sortVec
	sort.Slice(argList, func(i, j int) bool {
		a, b := argList[i].SortKeys, argList[j].SortKeys
		for x := 0; x < len(a) && x < len(b); x++ {
			if a[x] != b[x] {
				return a[x] < b[x] // ascending for middle selection
			}
		}
		return len(a) < len(b)
	})
	// Select the middle 3
	start := (len(argList) - 3) / 2
	if start < 0 {
		start = 0
	}
	end := start + 3
	if end > len(argList) {
		end = len(argList)
	}
	var result []ArgConfig
	for i := start; i < end; i++ {
		result = append(result, argList[i].Arg)
	}
	return result
}

// Strategy explanation (for you): // updated
// For each arg setting, extract all numeric fields (excluding "trace-id" and "truth"), sort keys alphabetically, and build a vector.
// The "greatest" three are those with the lexicographically largest vectors, i.e., prioritize larger values for the first key, then second, etc.
// This is robust for any number of numeric args and is deterministic. // updated
