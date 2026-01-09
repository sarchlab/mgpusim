package commands

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/sarchlab/mgpusim/v4/amd/cli/registry"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available benchmarks",
	Long: `List all available benchmarks and their parameters.

Examples:
  mgpusim_amd list
  mgpusim_amd list --category=heteromark
  mgpusim_amd list --benchmark=fir`,
	RunE: runList,
}

var (
	listCategory  string
	listBenchmark string
)

func init() {
	listCmd.Flags().StringVar(&listCategory, "category", "",
		"Filter by category (heteromark, amdappsdk, polybench, shoc, rodinia, dnn)")
	listCmd.Flags().StringVar(&listBenchmark, "benchmark", "",
		"Show details for a specific benchmark")
}

func runList(cmd *cobra.Command, args []string) error {
	if listBenchmark != "" {
		return showBenchmarkDetails(listBenchmark)
	}

	if listCategory != "" {
		return listByCategory(listCategory)
	}

	return listAll()
}

func listAll() error {
	fmt.Println("Available Benchmarks")
	fmt.Println("====================")

	// Group by category
	categories := []string{"heteromark", "amdappsdk", "polybench", "shoc", "rodinia", "dnn"}

	for _, cat := range categories {
		benchmarks := registry.GetBenchmarksByCategory(cat)
		if len(benchmarks) == 0 {
			continue
		}

		fmt.Printf("\n[%s]\n", cat)
		names := make([]string, 0, len(benchmarks))
		for _, b := range benchmarks {
			names = append(names, b.Name)
		}
		sort.Strings(names)

		for _, name := range names {
			meta := registry.Registry[name]
			fmt.Printf("  %-20s %s\n", name, meta.Description)
		}
	}

	fmt.Println("\nUse 'mgpusim_amd list --benchmark=<name>' for details")
	return nil
}

func listByCategory(category string) error {
	benchmarks := registry.GetBenchmarksByCategory(category)
	if len(benchmarks) == 0 {
		return fmt.Errorf("no benchmarks found in category: %s", category)
	}

	fmt.Printf("Benchmarks in [%s]\n", category)
	fmt.Println("====================")

	names := make([]string, 0, len(benchmarks))
	for _, b := range benchmarks {
		names = append(names, b.Name)
	}
	sort.Strings(names)

	for _, name := range names {
		meta := registry.Registry[name]
		fmt.Printf("  %-20s %s\n", name, meta.Description)
	}

	return nil
}

func showBenchmarkDetails(name string) error {
	meta, ok := registry.Registry[name]
	if !ok {
		return fmt.Errorf("unknown benchmark: %s", name)
	}

	fmt.Printf("Benchmark: %s\n", meta.Name)
	fmt.Printf("Category:  %s\n", meta.Category)
	fmt.Printf("Description: %s\n", meta.Description)
	fmt.Println()

	if len(meta.Parameters) == 0 {
		fmt.Println("Parameters: (none)")
	} else {
		fmt.Println("Parameters:")
		for _, p := range meta.Parameters {
			fmt.Printf("  --%s.%s (%s)\n", name, p.Name, p.Type)
			fmt.Printf("      Default: %v\n", p.Default)
			fmt.Printf("      %s\n", p.Description)
		}
	}

	fmt.Println()
	fmt.Printf("Example:\n")
	fmt.Printf("  mgpusim_amd run --benchmark=%s", name)
	for _, p := range meta.Parameters {
		fmt.Printf(" --%s.%s=%v", name, p.Name, p.Default)
	}
	fmt.Println(" --sim.timing")

	return nil
}
