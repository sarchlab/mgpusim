package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	Suite         string
	Benchmark     string
	Frequency     float64
	BeforeTurning int64
	Args          map[string]string
	Cycles        float64
	Line          int
}

const ratioSkipRelease = 0.35

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
	filesEmptyKernel := []string{
		// "polybench/2dconv-simulations.yaml",
		// "polybench/2mm-simulations.yaml",
		// "polybench/3dconv-simulations.yaml",
		// "polybench/3mm-simulations.yaml",
		// "polybench/atax-simulations.yaml",
		// "polybench/bicg-simulations.yaml",
		// "polybench/gemm-simulations.yaml",
		// "polybench/gesummv-simulations.yaml",
		// "polybench/mvt-simulations.yaml",
		// "polybench/syrk-simulations.yaml",
		// "cuda-sdk/fastwalshtransform-simulations.yaml",
		// "cuda-sdk/mergesort-simulations.yaml",
		// "cuda-sdk/scalarprod-simulations.yaml",
		// "cuda-sdk/scan-long-simulations.yaml",
		// "cuda-sdk/scan-short-simulations.yaml",
		// "cuda-sdk/sortingnetworks-bitonic-simulations.yaml",
		// "cuda-sdk/sortingnetworks-oddeven-simulations.yaml",
		// "cuda-sdk/transpose-simulations.yaml",
		// "cuda-sdk/vectoradd-simulations.yaml",

		"simtune/emptykernel-simulations.yaml",
	}
	groundTruthPath := "./ground_truth_release.txt"
	groundTruthEmptyKernelPath := "./ground_truth_release-emptykernel.txt"
	outputPathRelease := "../eval_config_release.json"
	outputPathReleaseEmptyKernel := "../eval_config_release-emptykernel.json"
	outputPathDev := "../eval_config_dev.json"

	groundTruth, err := parseGroundTruth(groundTruthPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing ground truth: %v\n", err)
		os.Exit(1)
	}

	groundTruthEmptyKernel, err := parseGroundTruth(groundTruthEmptyKernelPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing ground truth (empty kernel): %v\n", err)
		os.Exit(1)
	}

	benchmarks := []Benchmark{}
	missingTruth := []string{}

	for _, relPath := range files {
		fullPath := filepath.Join(folder, relPath)
		suite, benchmark := parseSuiteBenchmark(relPath)
		args, missing := parseSimYaml(fullPath, suite, benchmark, groundTruth, false)
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

	// Write eval_release.json, skipping the greatest nSkipRelease
	releaseBenchmarks := []Benchmark{}
	for _, bench := range benchmarks {
		skipN := int(math.Ceil(ratioSkipRelease * float64(len(bench.Args))))
		releaseArgs := selectArgsSorted(bench.Args, skipN, 0)
		releaseBenchmarks = append(releaseBenchmarks, Benchmark{
			Title: bench.Title,
			Suite: bench.Suite,
			Args:  releaseArgs,
		})
	}
	releaseConfig := EvalConfig{
		ScriptPath: "./mnt-collector",
		Benchmarks: releaseBenchmarks,
	}
	releaseCount := countArgs(releaseBenchmarks)

	releaseBeforeTurningCount := countBeforeTurning(releaseBenchmarks)

	writeEvalConfig(outputPathRelease, releaseConfig)
	fmt.Printf("Wrote %s: %d arg settings (%d/%d kept, %d/%d beforeTurning)\n", outputPathRelease, releaseCount, releaseCount-releaseBeforeTurningCount, releaseCount, releaseBeforeTurningCount, releaseCount)

	// For emptykernel only
	benchmarksEmptyKernel := []Benchmark{}
	missingTruthEmptyKernel := []string{}

	for _, relPath := range filesEmptyKernel {
		fullPath := filepath.Join(folder, relPath)
		suite, benchmark := parseSuiteBenchmark(relPath)
		args, missing := parseSimYaml(fullPath, suite, benchmark, groundTruthEmptyKernel, true)
		benchmarksEmptyKernel = append(benchmarksEmptyKernel, Benchmark{
			Title: benchmark,
			Suite: suite,
			Args:  args,
		})
		missingTruthEmptyKernel = append(missingTruthEmptyKernel, missing...)
	}

	if len(missingTruthEmptyKernel) > 0 {
		fmt.Println("Missing ground truth entries for (empty kernel):")
		for _, m := range missingTruthEmptyKernel {
			fmt.Println(m)
		}
	}

	// Write eval_release-emptykernel.json, keep all args
	releaseBenchmarksEmptyKernel := benchmarksEmptyKernel // no filtering
	releaseConfigEmptyKernel := EvalConfig{
		ScriptPath: "./mnt-collector",
		Benchmarks: releaseBenchmarksEmptyKernel,
	}
	releaseCountEmptyKernel := countArgs(releaseBenchmarksEmptyKernel)
	releaseBeforeTurningCountEmptyKernel := countBeforeTurning(releaseBenchmarksEmptyKernel)

	writeEvalConfig(outputPathReleaseEmptyKernel, releaseConfigEmptyKernel)
	fmt.Printf("Wrote %s: %d arg settings (all kept, %d/%d beforeTurning)\n",
		outputPathReleaseEmptyKernel, releaseCountEmptyKernel,
		releaseBeforeTurningCountEmptyKernel, releaseCountEmptyKernel)

	// Generate eval_dev.json (keep only 3 greatest arg settings per benchmark)
	devBenchmarks := []Benchmark{}     // updated
	for _, bench := range benchmarks { // updated
		devArgs := selectArgsSorted(bench.Args, 0, 3)
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
	devCount := countArgs(devBenchmarks)
	devBeforeTurningCount := countBeforeTurning(devBenchmarks)
	writeEvalConfig(outputPathDev, devConfig)
	fmt.Printf("Wrote %s: %d arg settings (%d/%d kept, %d/%d beforeTurning)\n", outputPathDev, devCount, devCount-devBeforeTurningCount, devCount, devBeforeTurningCount, devCount)
}

func countBeforeTurning(benchmarks []Benchmark) int {
	beforeTurningCount := 0
	for _, bench := range benchmarks {
		for _, arg := range bench.Args {
			if truth, ok := arg["truth"].(map[string]interface{}); ok {
				if v, ok := truth["beforeTurning"]; ok {
					// Accept int or float
					switch vv := v.(type) {
					case int:
						if vv == 1 {
							beforeTurningCount++
						}
					case int64:
						if vv == 1 {
							beforeTurningCount++
						}
					case float64:
						if int(vv) == 1 {
							beforeTurningCount++
						}
					}
				}
			}
		}
	}
	return beforeTurningCount
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

func parseSimYaml(path, suite, benchmark string, gt map[string]GroundTruthEntry, forceBeforeTurningIsZero bool) ([]ArgConfig, []string) {
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
			var beforeTurningVal interface{}
			if forceBeforeTurningIsZero {
				beforeTurningVal = 0
			} else {
				beforeTurningVal = entry.BeforeTurning
			}
			arg["truth"] = map[string]interface{}{
				"cycles":        entry.Cycles,
				"beforeTurning": beforeTurningVal,
			}
			// arg["truth"] = map[string]interface{}{"cycles": entry.Cycles, "beforeTurning": 0 if forceBeforeTurningIsZero else entry.BeforeTurning}
			arg["frequency"] = entry.Frequency
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
		frequency, err := strconv.ParseFloat(strings.TrimSpace(strings.Split(parts[2], "=")[1]), 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse frequency at line %d: %v\n", lineNum, err)
			frequency = -1.0 // default invalid frequency
		}
		args := make(map[string]string)
		for _, p := range parts[3 : len(parts)-2] {
			kv := strings.SplitN(p, "=", 2)
			if len(kv) == 2 {
				args[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
		cyclesStr := strings.TrimSpace(strings.Split(parts[len(parts)-2], "=")[1])
		beforeTurning, err := strconv.ParseInt(strings.TrimSpace(strings.Split(parts[len(parts)-1], "=")[1]), 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse beforeTurning at line %d: %v\n", lineNum, err)
			beforeTurning = -1 // default invalid beforeTurning
		}
		var cycles float64
		fmt.Sscanf(cyclesStr, "%f", &cycles)
		key := groundTruthKey(suite, benchmark, args)
		if _, exists := gt[key]; exists {
			conflicts = append(conflicts, fmt.Sprintf("Duplicate at line %d: %s", lineNum, line))
		}
		gt[key] = GroundTruthEntry{
			Suite:         suite,
			Benchmark:     benchmark,
			Frequency:     frequency,
			BeforeTurning: beforeTurning,
			Args:          args,
			Cycles:        cycles,
			Line:          lineNum,
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

// Generalized selection function
func selectArgsSorted(args []ArgConfig, skipGreatestN int, nSmall int) []ArgConfig {
	if len(args) <= nSmall || (skipGreatestN > 0 && len(args) <= skipGreatestN) {
		return args
	}

	var zeroList []ArgConfig
	if nSmall > 0 {
		// Filter out beforeTurning=1 points
		for _, a := range args {
			if truth, ok := a["truth"].(map[string]interface{}); ok {
				// fmt.Printf("Checking arg: %v\n", truth)
				if v, ok := truth["beforeTurning"]; ok && v == int64(0) {
					zeroList = append(zeroList, a)
				}
			}
		}
		args = zeroList
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

	// For release: skip the greatest n (from the end)
	if skipGreatestN > 0 {
		if len(argList) <= skipGreatestN {
			return []ArgConfig{}
		}
		argList = argList[:len(argList)-skipGreatestN]
	}

	// For dev: select the middle nSmall
	if nSmall > 0 {
		if len(argList) >= nSmall {
			argList = argList[:nSmall]
		} else {
			// If not enough args, just return all
			argList = argList[:]
		}
		// // fmt.Printf("Selecting %d middle args from %d total\n", nSmall, len(argList))
		// if len(argList) > nSmall {
		// 	start := (len(argList) - nSmall) / 2
		// 	end := start + nSmall
		// 	argList = argList[start:end]
		// } else {
		// 	// If not enough args, just return all
		// 	argList = argList[:]
		// }
	}

	var result []ArgConfig
	for _, a := range argList {
		result = append(result, a.Arg)
	}
	return result
}

// // Select the 3 "middle" arg settings for dev set, with "size" as highest priority if present // updated
// func selectMiddleArgsForDev(args []ArgConfig) []ArgConfig { // updated
// 	if len(args) <= 3 {
// 		return args
// 	}
// 	// Build sort keys: "size" first (if present), then other numeric keys alphabetically
// 	type argWithSort struct {
// 		Arg      ArgConfig
// 		SortKeys []float64
// 	}
// 	var argList []argWithSort
// 	for _, arg := range args {
// 		var keys []string
// 		hasSize := false
// 		for k := range arg {
// 			if k == "trace-id" || k == "truth" {
// 				continue
// 			}
// 			if k == "size" {
// 				hasSize = true
// 			} else {
// 				keys = append(keys, k)
// 			}
// 		}
// 		sort.Strings(keys)
// 		finalKeys := []string{}
// 		if hasSize {
// 			finalKeys = append(finalKeys, "size")
// 		}
// 		finalKeys = append(finalKeys, keys...)

// 		var sortVec []float64
// 		for _, k := range finalKeys {
// 			switch v := arg[k].(type) {
// 			case int:
// 				sortVec = append(sortVec, float64(v))
// 			case float64:
// 				sortVec = append(sortVec, v)
// 			default:
// 				// ignore non-numeric
// 			}
// 		}
// 		argList = append(argList, argWithSort{Arg: arg, SortKeys: sortVec})
// 	}
// 	// Sort: lexicographically by sortVec
// 	sort.Slice(argList, func(i, j int) bool {
// 		a, b := argList[i].SortKeys, argList[j].SortKeys
// 		for x := 0; x < len(a) && x < len(b); x++ {
// 			if a[x] != b[x] {
// 				return a[x] < b[x] // ascending for middle selection
// 			}
// 		}
// 		return len(a) < len(b)
// 	})
// 	// Select the middle 3
// 	start := (len(argList) - 3) / 2
// 	if start < 0 {
// 		start = 0
// 	}
// 	end := start + 3
// 	if end > len(argList) {
// 		end = len(argList)
// 	}
// 	var result []ArgConfig
// 	for i := start; i < end; i++ {
// 		result = append(result, argList[i].Arg)
// 	}
// 	return result
// }

// Strategy explanation (for you): // updated
// For each arg setting, extract all numeric fields (excluding "trace-id" and "truth"), sort keys alphabetically, and build a vector.
// The "greatest" three are those with the lexicographically largest vectors, i.e., prioritize larger values for the first key, then second, etc.
// This is robust for any number of numeric args and is deterministic. // updated
