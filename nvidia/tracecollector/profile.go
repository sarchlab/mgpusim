package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	// The two metrics behind "Elapsed Cycles" and "Duration" in ncu's GPU
	// Speed Of Light Throughput section, which mnt-collector summed.
	metricCycles   = "gpc__cycles_elapsed.max"
	metricDuration = "gpu__time_duration.sum"

	profileCSVName    = "profile_ncu.csv"
	profileFailedName = "profile_ncu_failed.log"
	profileTextName   = "profile.txt"

	permissionHint = " (no permission to read GPU performance counters: run " +
		"the command with sudo, or let all users profile with the nvidia " +
		"driver option NVreg_RestrictProfilingToAdminUsers=0; see " +
		"https://developer.nvidia.com/ERR_NVGPUCTRPERM)"
)

var errNoCSV = errors.New("ncu wrote no CSV table")

// kernelProfile is the profile of one kernel launch.
type kernelProfile struct {
	ID         string
	Name       string
	Cycles     float64
	DurationNs float64
}

// profileResult is the outcome of a profile run. Reason is set when no
// profile is available.
type profileResult struct {
	NCU     string
	Version string
	Kernels []kernelProfile
	Reason  string
	Note    string
}

func (r *profileResult) available() bool {
	return r != nil && r.Reason == "" && len(r.Kernels) > 0
}

// collectProfile runs the program under ncu and records the elapsed cycles
// and duration of every kernel. It never fails: when the profile cannot be
// taken, the result carries the reason.
func collectProfile(ctx context.Context, opts options) *profileResult {
	ncu, reason := findNCU(opts.ncu)
	if reason != "" {
		return &profileResult{Reason: reason}
	}

	r := &profileResult{NCU: ncu, Version: ncuVersion(ctx, ncu)}

	if reason := checkGPU(ctx); reason != "" {
		r.Reason = reason
		return r
	}

	fmt.Printf("Profiling %s with %s\n", strings.Join(opts.command, " "), ncu)

	// ncu writes to a temporary log that replaces profile_ncu.csv only when
	// the profile succeeds, so a failed run does not overwrite a good one.
	logPath := filepath.Join(opts.outDir, profileCSVName+".tmp")
	defer os.Remove(logPath)

	output, runErr := runNCU(ctx, ncu, logPath, opts.command, true)
	if mentionsUnknownOption(output) {
		// Older ncu versions lack --print-units or --print-kernel-base.
		output, runErr = runNCU(ctx, ncu, logPath, opts.command, false)
	}

	kernels, err := parseNCUCSV(output)
	if err != nil && !errors.Is(err, errNoCSV) {
		r.Reason = "cannot read the ncu output: " + err.Error()
	} else if len(kernels) == 0 {
		r.Reason = ncuFailureReason(output, runErr)
	}

	if r.Reason != "" {
		keepLog(logPath, filepath.Join(opts.outDir, profileFailedName))
		return r
	}

	keepLog(logPath, filepath.Join(opts.outDir, profileCSVName))

	r.Kernels = kernels

	if runErr != nil {
		r.Note = fmt.Sprintf("the profiled run exited with an error (%v); "+
			"check the program output above", runErr)
	}

	return r
}

// findNCU returns the ncu executable, or the reason why there is none.
func findNCU(explicit string) (string, string) {
	if explicit != "" {
		if isExecutable(explicit) {
			return explicit, ""
		}

		return "", "ncu not found at " + explicit + " (given by -ncu or NCU)"
	}

	if p, err := exec.LookPath("ncu"); err == nil {
		return p, ""
	}

	candidates := []string{"/usr/local/cuda/bin/ncu"}

	for _, pattern := range []string{
		"/usr/local/cuda-*/bin/ncu",
		"/opt/nvidia/nsight-compute/*/ncu",
	} {
		matches, _ := filepath.Glob(pattern)
		slices.Sort(matches)
		slices.Reverse(matches) // newest version first
		candidates = append(candidates, matches...)
	}

	for _, c := range candidates {
		if isExecutable(c) {
			return c, ""
		}
	}

	return "", "ncu not found in PATH, /usr/local/cuda*/bin, or " +
		"/opt/nvidia/nsight-compute; install Nsight Compute or pass -ncu <path>"
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// ncuVersion returns the version line of `ncu --version`, if any.
func ncuVersion(ctx context.Context, ncu string) string {
	out, err := exec.CommandContext(ctx, ncu, "--version").CombinedOutput()
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "Version") {
			return strings.TrimSpace(line)
		}
	}

	return ""
}

// checkGPU uses nvidia-smi, when it is installed, to make sure that a GPU is
// visible. It returns the reason when there is none.
func checkGPU(ctx context.Context) string {
	smi, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return "" // let ncu report the problem
	}

	out, err := exec.CommandContext(ctx, smi, "-L").CombinedOutput()
	if err != nil {
		return fmt.Sprintf("no usable NVIDIA GPU (nvidia-smi -L failed: %s)",
			firstLine(string(out), err.Error()))
	}

	if !strings.Contains(string(out), "GPU ") {
		return "no NVIDIA GPU found (nvidia-smi -L lists none)"
	}

	return ""
}

// runNCU runs the program under ncu. ncu writes its CSV table and messages
// to logPath; the program output goes to the terminal. It returns the log
// followed by the captured program output.
func runNCU(
	ctx context.Context,
	ncu, logPath string,
	command []string,
	withPrintOptions bool,
) (string, error) {
	_ = os.Remove(logPath)

	args := []string{
		"--csv",
		"--log-file", logPath,
		"--target-processes", "all",
		"--metrics", metricCycles + "," + metricDuration,
	}
	if withPrintOptions {
		args = append(args, "--print-units", "base", "--print-kernel-base", "function")
	}

	args = append(args, command...)

	var captured bytes.Buffer

	cmd := exec.CommandContext(ctx, ncu, args...)
	cmd.Stdout = io.MultiWriter(os.Stdout, &captured)
	cmd.Stderr = io.MultiWriter(os.Stderr, &captured)
	runErr := cmd.Run()

	logText, _ := os.ReadFile(logPath)

	return string(logText) + "\n" + captured.String(), runErr
}

func mentionsUnknownOption(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "unrecognised option") ||
		strings.Contains(lower, "unrecognized option")
}

// parseNCUCSV reads the per-kernel metrics from the CSV table that
// `ncu --csv --metrics ...` prints. Columns are found by name, and lines
// before the table (ncu messages, program output) are skipped.
func parseNCUCSV(output string) ([]kernelProfile, error) {
	table, ok := csvTable(output)
	if !ok {
		return nil, errNoCSV
	}

	r := csv.NewReader(strings.NewReader(table))
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return nil, err
	}

	col, err := columnIndex(header)
	if err != nil {
		return nil, err
	}

	var kernels []kernelProfile

	byID := map[string]int{}

	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}

		if err := addMetric(rec, col, byID, &kernels); err != nil {
			return nil, err
		}
	}

	return kernels, nil
}

// csvTable returns the part of output from the CSV header line on.
func csvTable(output string) (string, bool) {
	var b strings.Builder

	found := false

	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !found && !strings.HasPrefix(line, `"ID",`) {
			continue
		}

		// The table ends at the first line that is not a CSV row, e.g.,
		// program output captured after the log.
		if found && !strings.HasPrefix(line, `"`) {
			break
		}

		found = true

		b.WriteString(line)
		b.WriteByte('\n')
	}

	return b.String(), found
}

type columns struct{ id, name, metric, unit, value int }

func columnIndex(header []string) (columns, error) {
	find := func(names ...string) int {
		for i, h := range header {
			if slices.Contains(names, strings.TrimSpace(h)) {
				return i
			}
		}

		return -1
	}

	c := columns{
		id:     find("ID"),
		name:   find("Kernel Name", "Function Name", "Name"),
		metric: find("Metric Name"),
		unit:   find("Metric Unit"),
		value:  find("Metric Value"),
	}

	if c.id < 0 || c.name < 0 || c.metric < 0 || c.unit < 0 || c.value < 0 {
		return c, fmt.Errorf("unexpected CSV header %q", strings.Join(header, ","))
	}

	return c, nil
}

func addMetric(
	rec []string,
	col columns,
	byID map[string]int,
	kernels *[]kernelProfile,
) error {
	if len(rec) <= max(col.id, col.name, col.metric, col.unit, col.value) {
		return nil
	}

	value, ok := parseMetricValue(rec[col.value])
	if !ok {
		return nil
	}

	idx, seen := byID[rec[col.id]]
	if !seen {
		idx = len(*kernels)
		byID[rec[col.id]] = idx
		*kernels = append(*kernels, kernelProfile{ID: rec[col.id], Name: rec[col.name]})
	}

	k := &(*kernels)[idx]
	unit := rec[col.unit]

	switch rec[col.metric] {
	case metricCycles:
		scale, ok := cycleScale(unit)
		if !ok {
			return fmt.Errorf("unknown cycle unit %q", unit)
		}

		k.Cycles = value * scale
	case metricDuration:
		scale, ok := nanosecondScale(unit)
		if !ok {
			return fmt.Errorf("unknown duration unit %q", unit)
		}

		k.DurationNs = value * scale
	}

	return nil
}

// parseMetricValue parses a value such as "1,234,567". It reports false for
// values that are not numbers, e.g., "n/a".
func parseMetricValue(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	return v, err == nil
}

func cycleScale(unit string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "cycle", "cycles", "":
		return 1, true
	case "kcycle", "kcycles":
		return 1e3, true
	case "mcycle", "mcycles":
		return 1e6, true
	case "gcycle", "gcycles":
		return 1e9, true
	}

	return 0, false
}

func nanosecondScale(unit string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "nsecond", "ns":
		return 1, true
	case "usecond", "us":
		return 1e3, true
	case "msecond", "ms":
		return 1e6, true
	case "second", "s":
		return 1e9, true
	}

	return 0, false
}

// ncuFailureReason explains why ncu reported no kernel.
func ncuFailureReason(output string, runErr error) string {
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "==ERROR==") {
			continue
		}

		reason := "ncu: " + strings.TrimSpace(strings.TrimPrefix(
			strings.TrimSpace(line), "==ERROR=="))
		if strings.Contains(line, "ERR_NVGPUCTRPERM") {
			reason += permissionHint
		}

		return reason
	}

	if strings.Contains(output, "No kernels were profiled") {
		return "ncu profiled no kernels (did the program launch a kernel on the GPU?)"
	}

	if runErr != nil {
		return fmt.Sprintf("the profiled run failed (%v): %s", runErr,
			lastLine(output, "no output"))
	}

	return "ncu did not report any kernel; see " + profileFailedName
}

func firstLine(text, fallback string) string {
	for _, line := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}

	return fallback
}

func lastLine(text, fallback string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}

	return fallback
}

// profileReport formats the "# Profile Info" section.
func profileReport(opts options, r *profileResult) string {
	var b strings.Builder

	b.WriteString("# Profile Info\n")

	switch {
	case r == nil:
		b.WriteString("Status:         skipped (-trace-only)\n")
		return b.String()
	case !r.available():
		fmt.Fprintf(&b, "Status:         Not available: %s\n", r.Reason)
		return b.String()
	}

	b.WriteString("Status:         OK\n")
	fmt.Fprintf(&b, "Tool:           %s\n", toolDescription(r))
	fmt.Fprintf(&b, "Kernels:        %d\n", len(r.Kernels))
	writeKernelTable(&b, r.Kernels)

	var cycles, durationNs float64
	for _, k := range r.Kernels {
		cycles += k.Cycles
		durationNs += k.DurationNs
	}

	fmt.Fprintf(&b, "Total cycles:   %.0f\n", cycles)
	fmt.Fprintf(&b, "Total duration: %.3f us\n", durationNs/1e3)

	if durationNs > 0 {
		fmt.Fprintf(&b, "Average clock:  %.0f MHz (total cycles / total duration)\n",
			cycles/durationNs*1e3)
	}

	if r.Note != "" {
		fmt.Fprintf(&b, "Note:           %s\n", r.Note)
	}

	fmt.Fprintf(&b, "Saved:          %s, %s\n",
		filepath.Join(opts.outDir, profileTextName),
		filepath.Join(opts.outDir, profileCSVName))

	return b.String()
}

func toolDescription(r *profileResult) string {
	if r.Version == "" {
		return r.NCU
	}

	return fmt.Sprintf("%s (%s)", r.NCU, r.Version)
}

func writeKernelTable(b *strings.Builder, kernels []kernelProfile) {
	width := len("Kernel")
	for _, k := range kernels {
		width = max(width, min(len(k.Name), 48))
	}

	fmt.Fprintf(b, "  %-4s %-*s %14s %16s\n", "ID", width, "Kernel", "Cycles",
		"Duration (us)")

	for _, k := range kernels {
		name := k.Name
		if len(name) > width {
			name = name[:width-3] + "..."
		}

		fmt.Fprintf(b, "  %-4s %-*s %14.0f %16.3f\n", k.ID, width, name, k.Cycles,
			k.DurationNs/1e3)
	}
}

// keepLog moves ncu's log to its final name.
func keepLog(from, to string) {
	if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "warning: cannot save %s: %v\n", to, err)
	}
}

// saveProfileReport keeps the "# Profile Info" section next to the trace. A
// report of an unavailable profile does not replace an earlier good one.
func saveProfileReport(outDir string, r *profileResult, text string) {
	path := filepath.Join(outDir, profileTextName)
	if !r.available() {
		if _, err := os.Stat(path); err == nil {
			return
		}
	}

	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot write %s: %v\n", path, err)
	}
}
