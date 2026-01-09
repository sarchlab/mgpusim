// Package config defines the configuration structures for the MGPUSim CLI.
package config

// Config is the root configuration structure for a simulation.
type Config struct {
	Version    string           `yaml:"version" mapstructure:"version"`
	Benchmark  BenchmarkConfig  `yaml:"benchmark" mapstructure:"benchmark"`
	Simulation SimulationConfig `yaml:"simulation" mapstructure:"simulation"`
	Hardware   HardwareConfig   `yaml:"hardware" mapstructure:"hardware"`
	Report     ReportConfig     `yaml:"report" mapstructure:"report"`
	Tracing    TracingConfig    `yaml:"tracing" mapstructure:"tracing"`
}

// BenchmarkConfig holds the benchmark name and its parameters.
type BenchmarkConfig struct {
	Name   string         `yaml:"name" mapstructure:"name"`
	Params map[string]any `yaml:"params" mapstructure:"params"`
}

// SimulationConfig holds simulation mode settings.
type SimulationConfig struct {
	Timing   bool   `yaml:"timing" mapstructure:"timing"`
	Parallel bool   `yaml:"parallel" mapstructure:"parallel"`
	Verify   bool   `yaml:"verify" mapstructure:"verify"`
	MaxInst  uint64 `yaml:"max_inst" mapstructure:"max_inst"`
}

// HardwareConfig holds hardware/platform settings.
type HardwareConfig struct {
	Arch            string `yaml:"arch" mapstructure:"arch"`
	GPU             string `yaml:"gpu" mapstructure:"gpu"`
	GPUs            []int  `yaml:"gpus" mapstructure:"gpus"`
	UnifiedGPUs     []int  `yaml:"unified_gpus" mapstructure:"unified_gpus"`
	UnifiedMemory   bool   `yaml:"unified_memory" mapstructure:"unified_memory"`
	MagicMemoryCopy bool   `yaml:"magic_memory_copy" mapstructure:"magic_memory_copy"`
}

// ReportConfig holds reporting settings.
type ReportConfig struct {
	All      bool          `yaml:"all" mapstructure:"all"`
	Filename string        `yaml:"filename" mapstructure:"filename"`
	Metrics  MetricsConfig `yaml:"metrics" mapstructure:"metrics"`
}

// MetricsConfig holds individual metric reporting flags.
type MetricsConfig struct {
	InstCount            bool `yaml:"inst_count" mapstructure:"inst_count"`
	CacheLatency         bool `yaml:"cache_latency" mapstructure:"cache_latency"`
	CacheHitRate         bool `yaml:"cache_hit_rate" mapstructure:"cache_hit_rate"`
	TLBHitRate           bool `yaml:"tlb_hit_rate" mapstructure:"tlb_hit_rate"`
	RDMATransactionCount bool `yaml:"rdma_transaction_count" mapstructure:"rdma_transaction_count"`
	DRAMTransactionCount bool `yaml:"dram_transaction_count" mapstructure:"dram_transaction_count"`
	SIMDBusyTime         bool `yaml:"simd_busy_time" mapstructure:"simd_busy_time"`
	CPIStack             bool `yaml:"cpi_stack" mapstructure:"cpi_stack"`
}

// TracingConfig holds tracing settings.
type TracingConfig struct {
	Visualization bool    `yaml:"visualization" mapstructure:"visualization"`
	VisDB         string  `yaml:"vis_db" mapstructure:"vis_db"`
	VisDBFile     string  `yaml:"vis_db_file" mapstructure:"vis_db_file"`
	VisStartTime  float64 `yaml:"vis_start_time" mapstructure:"vis_start_time"`
	VisEndTime    float64 `yaml:"vis_end_time" mapstructure:"vis_end_time"`
	Memory        bool    `yaml:"memory" mapstructure:"memory"`
	ISADebug      bool    `yaml:"isa_debug" mapstructure:"isa_debug"`
}

// NewDefault creates a Config with default values.
func NewDefault() *Config {
	return &Config{
		Version: "1.0",
		Benchmark: BenchmarkConfig{
			Params: make(map[string]any),
		},
		Simulation: SimulationConfig{
			Timing:   false,
			Parallel: false,
			Verify:   false,
			MaxInst:  0,
		},
		Hardware: HardwareConfig{
			Arch:            "gcn3",
			GPU:             "r9nano",
			GPUs:            []int{1},
			UnifiedGPUs:     nil,
			UnifiedMemory:   false,
			MagicMemoryCopy: false,
		},
		Report: ReportConfig{
			All:      false,
			Filename: "metrics",
			Metrics:  MetricsConfig{},
		},
		Tracing: TracingConfig{
			VisDB:        "sqlite",
			VisStartTime: -1,
			VisEndTime:   -1,
		},
	}
}
