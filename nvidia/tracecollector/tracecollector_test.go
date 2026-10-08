package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ncuOutputFile mimics what runNCU returns: ncu's log (messages and the CSV
// table) followed by the program output.
const ncuOutputFile = "testdata/ncu_output.txt"

func readNCUOutput(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile(ncuOutputFile)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

func TestParseNCUCSV(t *testing.T) {
	kernels, err := parseNCUCSV(readNCUOutput(t))
	if err != nil {
		t.Fatal(err)
	}

	want := []kernelProfile{
		{ID: "0", Name: "atax_kernel1", Cycles: 1234567, DurationNs: 776320},
		{ID: "1", Name: "atax_kernel2", Cycles: 1500500, DurationNs: 943750},
	}

	if len(kernels) != len(want) {
		t.Fatalf("got %d kernels, want %d: %+v", len(kernels), len(want), kernels)
	}

	for i := range want {
		if kernels[i] != want[i] {
			t.Errorf("kernel %d: got %+v, want %+v", i, kernels[i], want[i])
		}
	}
}

func TestParseNCUCSVWithoutTable(t *testing.T) {
	_, err := parseNCUCSV("==PROF== Connected to process 1\n==WARNING== No kernels were profiled.\n")
	if !errors.Is(err, errNoCSV) {
		t.Fatalf("got %v, want errNoCSV", err)
	}
}

func TestNCUFailureReason(t *testing.T) {
	tests := []struct {
		name   string
		output string
		runErr error
		want   []string
	}{
		{
			name: "no permission",
			output: "==ERROR== ERR_NVGPUCTRPERM - The user does not have permission " +
				"to access NVIDIA GPU Performance Counters on the target device 0.\n",
			want: []string{"ERR_NVGPUCTRPERM", "sudo", "NVreg_RestrictProfilingToAdminUsers"},
		},
		{
			name:   "no kernels",
			output: "==WARNING== No kernels were profiled.\n",
			want:   []string{"no kernels"},
		},
		{
			name:   "program failed",
			output: "atax.cu:40: cudaMalloc failed: no CUDA-capable device is detected\n",
			runErr: errors.New("exit status 1"),
			want:   []string{"exit status 1", "no CUDA-capable device"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ncuFailureReason(tc.output, tc.runErr)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("reason %q does not mention %q", got, w)
				}
			}
		})
	}
}

func TestReports(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	index := "MemcpyHtoD,0x1000,64\nkernel-1.traceg\nkernel-2.traceg\n"
	write(simulatorIndex, index)
	write("kernel-1.traceg", strings.Repeat("a", 1_000_000))
	write("kernel-2.traceg", strings.Repeat("b", 500_000))
	// 1,500,000 bytes of kernel traces plus 53 bytes of kernelslist.g.
	write("stats_ctx_0x1", "not part of the trace")

	opts := options{outDir: dir}

	trace := traceReport(opts, nil)
	for _, w := range []string{"# Trace Info", "Kernels:        2", "Memory copies:  1",
		"Trace size:     1.5001 MB"} {
		if !strings.Contains(trace, w) {
			t.Errorf("trace report misses %q:\n%s", w, trace)
		}
	}

	kernels, err := parseNCUCSV(readNCUOutput(t))
	if err != nil {
		t.Fatal(err)
	}

	profile := profileReport(opts, &profileResult{NCU: "/usr/local/cuda/bin/ncu", Kernels: kernels})
	for _, w := range []string{"# Profile Info", "Status:         OK", "atax_kernel2",
		"Total cycles:   2735067", "Total duration: 1720.070 us"} {
		if !strings.Contains(profile, w) {
			t.Errorf("profile report misses %q:\n%s", w, profile)
		}
	}

	missing := profileReport(opts, &profileResult{Reason: "ncu not found"})
	if !strings.Contains(missing, "Not available: ncu not found") {
		t.Errorf("unexpected report:\n%s", missing)
	}
}

func TestValidate(t *testing.T) {
	both := options{command: []string{"app"}, outDir: t.TempDir(), traceOnly: true, profileOnly: true}
	if err := validate(&both); err == nil {
		t.Error("-trace-only with -profile-only should be rejected")
	}

	// Profiling alone needs neither the tracer nor an empty directory.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, simulatorIndex), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	profileOnly := options{command: []string{"app"}, outDir: dir, profileOnly: true}
	if err := validate(&profileOnly); err != nil {
		t.Errorf("-profile-only on a trace directory: %v", err)
	}

	trace := options{command: []string{"app"}, outDir: t.TempDir()}
	if err := validate(&trace); err == nil {
		t.Error("tracing without -tracer and -processor should be rejected")
	}
}

func TestFindNCUExplicitPath(t *testing.T) {
	path, reason := findNCU("/nonexistent/ncu")
	if path != "" || !strings.Contains(reason, "/nonexistent/ncu") {
		t.Errorf("got path %q, reason %q", path, reason)
	}
}
