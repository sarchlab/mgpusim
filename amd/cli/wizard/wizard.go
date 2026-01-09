// Package wizard provides an interactive configuration wizard.
package wizard

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/sarchlab/mgpusim/v4/amd/cli/config"
	"github.com/sarchlab/mgpusim/v4/amd/cli/registry"
)

// ErrWizardCancelled is returned when the user cancels the wizard.
var ErrWizardCancelled = errors.New("wizard cancelled")

// Wizard guides users through simulation configuration.
type Wizard struct {
	config          *config.Config
	presetBenchmark string
}

// New creates a new wizard.
func New() *Wizard {
	return &Wizard{
		config: config.NewDefault(),
	}
}

// SetBenchmark pre-selects a benchmark.
func (w *Wizard) SetBenchmark(name string) {
	w.presetBenchmark = name
}

// Run executes the wizard and returns the configuration.
func (w *Wizard) Run() (*config.Config, error) {
	// Step 1: Simulation mode
	if err := w.stepSimulation(); err != nil {
		return nil, err
	}

	// Step 2: Hardware configuration
	if err := w.stepHardware(); err != nil {
		return nil, err
	}

	// Step 3: Benchmark selection
	if err := w.stepBenchmark(); err != nil {
		return nil, err
	}

	// Step 4: Benchmark parameters
	if err := w.stepBenchmarkParams(); err != nil {
		return nil, err
	}

	// Step 5: Report metrics
	if err := w.stepReport(); err != nil {
		return nil, err
	}

	// Step 6: Review and confirm
	return w.stepReview()
}

func (w *Wizard) stepSimulation() error {
	var timingMode string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select simulation mode").
				Description("Emulation is fast, timing is cycle-accurate").
				Options(
					huh.NewOption("Emulation (fast, functional)", "emulation"),
					huh.NewOption("Timing (cycle-accurate)", "timing"),
				).
				Value(&timingMode),

			huh.NewConfirm().
				Title("Run in parallel?").
				Description("Uses multiple CPU threads for simulation").
				Value(&w.config.Simulation.Parallel),

			huh.NewConfirm().
				Title("Verify results?").
				Description("Compare GPU results with CPU reference").
				Value(&w.config.Simulation.Verify),
		),
	)

	if err := form.Run(); err != nil {
		return err
	}

	w.config.Simulation.Timing = (timingMode == "timing")
	return nil
}

func (w *Wizard) stepHardware() error {
	var gpuStr string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select GPU architecture").
				Options(
					huh.NewOption("GCN3 (default)", "gcn3"),
					huh.NewOption("CDNA3 (MI300A)", "cdna3"),
				).
				Value(&w.config.Hardware.Arch),

			huh.NewSelect[string]().
				Title("Select GPU model").
				Options(
					huh.NewOption("R9 Nano (GCN3)", "r9nano"),
					huh.NewOption("MI300A (CDNA3)", "mi300a"),
				).
				Value(&w.config.Hardware.GPU),

			huh.NewInput().
				Title("GPU IDs to use").
				Description("Comma-separated list (e.g., 1,2,3,4)").
				Placeholder("1").
				Value(&gpuStr),

			huh.NewConfirm().
				Title("Use unified memory?").
				Description("Enable page migration between CPU and GPU").
				Value(&w.config.Hardware.UnifiedMemory),
		),
	)

	if err := form.Run(); err != nil {
		return err
	}

	// Parse GPU IDs
	w.config.Hardware.GPUs = parseGPUIDs(gpuStr)
	if len(w.config.Hardware.GPUs) == 0 {
		w.config.Hardware.GPUs = []int{1}
	}

	return nil
}

func (w *Wizard) stepBenchmark() error {
	if w.presetBenchmark != "" {
		if _, ok := registry.Registry[w.presetBenchmark]; ok {
			w.config.Benchmark.Name = w.presetBenchmark
			return nil
		}
	}

	// Build benchmark options grouped by category
	categories := []string{"heteromark", "amdappsdk", "polybench", "shoc", "rodinia", "dnn"}
	var options []huh.Option[string]

	for _, cat := range categories {
		benchmarks := registry.GetBenchmarksByCategory(cat)
		names := make([]string, 0, len(benchmarks))
		for _, b := range benchmarks {
			names = append(names, b.Name)
		}
		sort.Strings(names)

		for _, name := range names {
			meta := registry.Registry[name]
			label := fmt.Sprintf("[%s] %s - %s", cat, name, meta.Description)
			options = append(options, huh.NewOption(label, name))
		}
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select benchmark").
				Options(options...).
				Value(&w.config.Benchmark.Name),
		),
	)

	return form.Run()
}

//nolint:funlen // This function handles all parameter types which requires the length
func (w *Wizard) stepBenchmarkParams() error {
	meta, ok := registry.Registry[w.config.Benchmark.Name]
	if !ok {
		return fmt.Errorf("unknown benchmark: %s", w.config.Benchmark.Name)
	}

	if len(meta.Parameters) == 0 {
		return nil
	}

	// Initialize params map
	w.config.Benchmark.Params = make(map[string]any)

	// Create input fields for each parameter
	fields := make([]huh.Field, 0, len(meta.Parameters))
	paramValues := make(map[string]*string)

	for _, param := range meta.Parameters {
		defaultVal := fmt.Sprintf("%v", param.Default)
		paramValues[param.Name] = &defaultVal

		fields = append(fields,
			huh.NewInput().
				Title(param.Name).
				Description(fmt.Sprintf("%s (default: %v)", param.Description, param.Default)).
				Placeholder(defaultVal).
				Value(paramValues[param.Name]),
		)
	}

	form := huh.NewForm(
		huh.NewGroup(fields...).Title(fmt.Sprintf("Configure %s parameters", w.config.Benchmark.Name)),
	)

	if err := form.Run(); err != nil {
		return err
	}

	// Convert values to appropriate types
	for _, param := range meta.Parameters {
		strVal := *paramValues[param.Name]
		if strVal == "" {
			w.config.Benchmark.Params[param.Name] = param.Default
			continue
		}

		switch param.Type {
		case "int", "uint":
			if v, err := strconv.Atoi(strVal); err == nil {
				w.config.Benchmark.Params[param.Name] = v
			} else {
				w.config.Benchmark.Params[param.Name] = param.Default
			}
		case "float":
			if v, err := strconv.ParseFloat(strVal, 64); err == nil {
				w.config.Benchmark.Params[param.Name] = v
			} else {
				w.config.Benchmark.Params[param.Name] = param.Default
			}
		case "bool":
			w.config.Benchmark.Params[param.Name] = strVal == "true" || strVal == "1"
		case "string":
			w.config.Benchmark.Params[param.Name] = strVal
		}
	}

	return nil
}

func (w *Wizard) stepReport() error {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable all reports?").
				Description("Generates comprehensive CSV metrics").
				Value(&w.config.Report.All),

			huh.NewMultiSelect[string]().
				Title("Select specific metrics").
				Description("Choose which metrics to report").
				Options(
					huh.NewOption("Instruction count", "inst_count"),
					huh.NewOption("Cache hit rate", "cache_hit_rate"),
					huh.NewOption("Cache latency", "cache_latency"),
					huh.NewOption("TLB hit rate", "tlb_hit_rate"),
					huh.NewOption("CPI stack", "cpi_stack"),
					huh.NewOption("SIMD busy time", "simd_busy_time"),
				).
				Value(&[]string{}), // We'll handle this below
		),
	)

	return form.Run()
}

func (w *Wizard) stepReview() (*config.Config, error) {
	// Show configuration summary
	summary := w.buildSummary()

	var action string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Configuration Summary").
				Description(summary),

			huh.NewSelect[string]().
				Title("What would you like to do?").
				Options(
					huh.NewOption("Run simulation", "run"),
					huh.NewOption("Export to YAML", "export"),
					huh.NewOption("Cancel", "cancel"),
				).
				Value(&action),
		),
	)

	if err := form.Run(); err != nil {
		return nil, err
	}

	switch action {
	case "run":
		return w.config, nil
	case "export":
		return w.config, nil
	default:
		return nil, ErrWizardCancelled
	}
}

func (w *Wizard) buildSummary() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Benchmark: %s\n", w.config.Benchmark.Name))

	mode := "Emulation"
	if w.config.Simulation.Timing {
		mode = "Timing"
	}
	sb.WriteString(fmt.Sprintf("Mode: %s", mode))
	if w.config.Simulation.Parallel {
		sb.WriteString(" (parallel)")
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("Architecture: %s (%s)\n", w.config.Hardware.Arch, w.config.Hardware.GPU))
	sb.WriteString(fmt.Sprintf("GPUs: %v\n", w.config.Hardware.GPUs))

	if len(w.config.Benchmark.Params) > 0 {
		sb.WriteString("\nParameters:\n")
		for k, v := range w.config.Benchmark.Params {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
	}

	return sb.String()
}

func parseGPUIDs(s string) []int {
	if s == "" {
		return nil
	}

	var result []int
	parts := strings.Split(s, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if v, err := strconv.Atoi(p); err == nil {
			result = append(result, v)
		}
	}
	return result
}
