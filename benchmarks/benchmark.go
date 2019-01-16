package benchmarks

// A Benchmark is a GPU program that can run on the GCN3 simulator
type Benchmark interface {
	Run()
	Verify()
}
