package config

import (
	"os"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// LoadFromFile loads configuration from a YAML file.
func LoadFromFile(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	cfg := NewDefault()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadFromViper loads configuration from a pre-configured Viper instance.
func LoadFromViper(v *viper.Viper) (*Config, error) {
	cfg := NewDefault()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SaveToFile saves the configuration to a YAML file.
func (c *Config) SaveToFile(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SetupViper configures a Viper instance with defaults and environment binding.
func SetupViper() *viper.Viper {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Environment variable support (e.g., MGPUSIM_SIM_TIMING=true)
	v.SetEnvPrefix("MGPUSIM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	return v
}

func setDefaults(v *viper.Viper) {
	// Simulation defaults
	v.SetDefault("simulation.timing", false)
	v.SetDefault("simulation.parallel", false)
	v.SetDefault("simulation.verify", false)
	v.SetDefault("simulation.max_inst", 0)

	// Hardware defaults
	v.SetDefault("hardware.arch", "gcn3")
	v.SetDefault("hardware.gpu", "r9nano")
	v.SetDefault("hardware.gpus", []int{1})
	v.SetDefault("hardware.unified_gpus", []int{})
	v.SetDefault("hardware.unified_memory", false)
	v.SetDefault("hardware.magic_memory_copy", false)

	// Report defaults
	v.SetDefault("report.all", false)
	v.SetDefault("report.filename", "metrics")

	// Tracing defaults
	v.SetDefault("tracing.vis_db", "sqlite")
	v.SetDefault("tracing.vis_start_time", -1.0)
	v.SetDefault("tracing.vis_end_time", -1.0)
}
