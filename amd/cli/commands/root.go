// Package commands provides CLI command definitions.
package commands

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd is the base command.
var rootCmd = &cobra.Command{
	Use:   "mgpusim_amd",
	Short: "MGPUSim AMD GPU Simulator",
	Long: `MGPUSim is a cycle-accurate GPU simulator for AMD GCN3 and CDNA3 architectures.

Three modes of operation:
  1. Interactive wizard:  mgpusim_amd wizard
  2. Direct CLI:          mgpusim_amd run --benchmark=fir --fir.length=1024 --sim.timing
  3. Config file:         mgpusim_amd run --config=simulation.yaml`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"Config file (default: ./simulation.yaml)")

	// Add subcommands
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(wizardCmd)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("simulation")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
	}

	// Environment variables: MGPUSIM_SIM_TIMING=true -> sim.timing
	viper.SetEnvPrefix("MGPUSIM")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Try to read config file (ignore if not found)
	if err := viper.ReadInConfig(); err == nil {
		_, _ = os.Stderr.WriteString("Using config file: " + viper.ConfigFileUsed() + "\n")
	}
}
