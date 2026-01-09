package commands

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sarchlab/mgpusim/v4/amd/cli/registry"
	"github.com/sarchlab/mgpusim/v4/amd/cli/wizard"
)

var wizardCmd = &cobra.Command{
	Use:   "wizard",
	Short: "Interactive configuration wizard",
	Long: `Launch an interactive wizard to configure and run a simulation.

The wizard guides you through:
  1. Simulation mode selection (emulation/timing)
  2. Hardware configuration
  3. Benchmark selection
  4. Benchmark parameters
  5. Report metrics
  6. Review and run/export

Examples:
  mgpusim_amd wizard
  mgpusim_amd wizard --benchmark=fir
  mgpusim_amd wizard --export=my_config.yaml`,
	RunE: runWizard,
}

var (
	wizardBenchmark string
	wizardExport    string
)

func init() {
	wizardCmd.Flags().StringVar(&wizardBenchmark, "benchmark", "",
		"Pre-select benchmark")
	wizardCmd.Flags().StringVar(&wizardExport, "export", "",
		"Export config to file instead of running")
}

func runWizard(cmd *cobra.Command, args []string) error {
	w := wizard.New()

	if wizardBenchmark != "" {
		w.SetBenchmark(wizardBenchmark)
	}

	cfg, err := w.Run()
	if err != nil {
		if errors.Is(err, wizard.ErrWizardCancelled) {
			fmt.Println("Wizard cancelled.")
			return nil
		}
		return err
	}

	// Export if requested
	if wizardExport != "" {
		if err := cfg.SaveToFile(wizardExport); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Printf("Configuration saved to %s\n", wizardExport)
		return nil
	}

	// Otherwise run the simulation
	meta, ok := registry.Registry[cfg.Benchmark.Name]
	if !ok {
		return fmt.Errorf("unknown benchmark: %s", cfg.Benchmark.Name)
	}

	printConfigSummary(cfg)
	return executeSimulation(cfg, meta)
}
