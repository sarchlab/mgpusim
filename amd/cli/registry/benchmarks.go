// Package registry provides benchmark metadata and factory functions.
package registry

import (
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/amdappsdk/bitonicsort"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/amdappsdk/fastwalshtransform"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/amdappsdk/floydwarshall"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/amdappsdk/matrixmultiplication"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/amdappsdk/matrixtranspose"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/amdappsdk/nbody"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/amdappsdk/simpleconvolution"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/amdappsdk/vectoradd"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/layer_benchmarks/conv2d"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/layer_benchmarks/im2col"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/layer_benchmarks/relu"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/training_benchmarks/lenet"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/training_benchmarks/minerva"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/training_benchmarks/vgg16"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/training_benchmarks/xor"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/heteromark/aes"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/heteromark/kmeans"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/heteromark/pagerank"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/polybench/atax"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/polybench/bicg"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/rodinia/nw"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/shoc/bfs"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/shoc/fft"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/shoc/spmv"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/shoc/stencil2d"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
)

// BenchmarkFactory creates a benchmark instance.
type BenchmarkFactory func(d *driver.Driver) benchmarks.Benchmark

// ParamConfigurator applies parameters to a benchmark.
type ParamConfigurator func(b benchmarks.Benchmark, params map[string]any)

// ParameterMeta describes a benchmark parameter.
type ParameterMeta struct {
	Name        string
	Type        string // "int", "uint", "float", "string", "bool"
	Default     any
	Description string
}

// BenchmarkMeta holds metadata about a benchmark.
type BenchmarkMeta struct {
	Name        string
	Description string
	Category    string // heteromark, amdappsdk, polybench, shoc, rodinia, dnn
	Parameters  []ParameterMeta
	Factory     BenchmarkFactory
	Configure   ParamConfigurator
}

// Registry holds all registered benchmarks.
var Registry = map[string]BenchmarkMeta{
	// Hetero-Mark benchmarks
	"fir": {
		Name:        "fir",
		Description: "Finite Impulse Response filter",
		Category:    "heteromark",
		Parameters: []ParameterMeta{
			{Name: "length", Type: "int", Default: 4096, Description: "Number of samples to filter"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return fir.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			if v, ok := params["length"]; ok {
				b.(*fir.Benchmark).Length = toInt(v)
			}
		},
	},
	"kmeans": {
		Name:        "kmeans",
		Description: "K-Means clustering",
		Category:    "heteromark",
		Parameters: []ParameterMeta{
			{Name: "points", Type: "int", Default: 1024, Description: "Number of points"},
			{Name: "clusters", Type: "int", Default: 5, Description: "Number of clusters"},
			{Name: "features", Type: "int", Default: 32, Description: "Number of features per point"},
			{Name: "max-iter", Type: "int", Default: 5, Description: "Maximum iterations"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return kmeans.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*kmeans.Benchmark)
			if v, ok := params["points"]; ok {
				bench.NumPoints = toInt(v)
			}
			if v, ok := params["clusters"]; ok {
				bench.NumClusters = toInt(v)
			}
			if v, ok := params["features"]; ok {
				bench.NumFeatures = toInt(v)
			}
			if v, ok := params["max-iter"]; ok {
				bench.MaxIter = toInt(v)
			}
		},
	},
	"aes": {
		Name:        "aes",
		Description: "AES encryption",
		Category:    "heteromark",
		Parameters: []ParameterMeta{
			{Name: "length", Type: "int", Default: 65536, Description: "Length of data to encrypt"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return aes.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			if v, ok := params["length"]; ok {
				b.(*aes.Benchmark).Length = toInt(v)
			}
		},
	},
	"pagerank": {
		Name:        "pagerank",
		Description: "PageRank algorithm",
		Category:    "heteromark",
		Parameters: []ParameterMeta{
			{Name: "node", Type: "int", Default: 16, Description: "Number of nodes"},
			{Name: "sparsity", Type: "float", Default: 0.001, Description: "Graph sparsity"},
			{Name: "iterations", Type: "int", Default: 16, Description: "Number of iterations"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return pagerank.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*pagerank.Benchmark)
			numNode := 16
			sparsity := 0.001
			if v, ok := params["node"]; ok {
				numNode = toInt(v)
				bench.NumNodes = uint32(numNode)
			}
			if v, ok := params["sparsity"]; ok {
				sparsity = toFloat(v)
			}
			numConn := int(float64(numNode*numNode) * sparsity)
			if numConn < numNode {
				numConn = numNode
			}
			bench.NumConnections = uint32(numConn)
			if v, ok := params["iterations"]; ok {
				bench.MaxIterations = uint32(toInt(v))
			}
		},
	},

	// AMD APP SDK benchmarks
	"vectoradd": {
		Name:        "vectoradd",
		Description: "Vector addition",
		Category:    "amdappsdk",
		Parameters: []ParameterMeta{
			{Name: "width", Type: "uint", Default: 1024, Description: "Vector width"},
			{Name: "height", Type: "uint", Default: 1024, Description: "Vector height"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return vectoradd.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*vectoradd.Benchmark)
			if v, ok := params["width"]; ok {
				bench.Width = uint32(toInt(v))
			}
			if v, ok := params["height"]; ok {
				bench.Height = uint32(toInt(v))
			}
		},
	},
	"matrixmultiplication": {
		Name:        "matrixmultiplication",
		Description: "Matrix multiplication",
		Category:    "amdappsdk",
		Parameters: []ParameterMeta{
			{Name: "x", Type: "uint", Default: 64, Description: "Height of first matrix"},
			{Name: "y", Type: "uint", Default: 64, Description: "Width of first / height of second"},
			{Name: "z", Type: "uint", Default: 64, Description: "Width of second matrix"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark {
			return matrixmultiplication.NewBenchmark(d)
		},
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*matrixmultiplication.Benchmark)
			if v, ok := params["x"]; ok {
				bench.X = uint32(toInt(v))
			}
			if v, ok := params["y"]; ok {
				bench.Y = uint32(toInt(v))
			}
			if v, ok := params["z"]; ok {
				bench.Z = uint32(toInt(v))
			}
		},
	},
	"bitonicsort": {
		Name:        "bitonicsort",
		Description: "Bitonic sort",
		Category:    "amdappsdk",
		Parameters: []ParameterMeta{
			{Name: "length", Type: "int", Default: 1024, Description: "Array length"},
			{Name: "order-asc", Type: "bool", Default: true, Description: "Sort ascending"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return bitonicsort.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*bitonicsort.Benchmark)
			if v, ok := params["length"]; ok {
				bench.Length = toInt(v)
			}
			if v, ok := params["order-asc"]; ok {
				bench.OrderAscending = toBool(v)
			}
		},
	},
	"fastwalshtransform": {
		Name:        "fastwalshtransform",
		Description: "Fast Walsh Transform",
		Category:    "amdappsdk",
		Parameters: []ParameterMeta{
			{Name: "length", Type: "int", Default: 1024, Description: "Array length"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark {
			return fastwalshtransform.NewBenchmark(d)
		},
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			if v, ok := params["length"]; ok {
				b.(*fastwalshtransform.Benchmark).Length = uint32(toInt(v))
			}
		},
	},
	"floydwarshall": {
		Name:        "floydwarshall",
		Description: "Floyd-Warshall shortest path",
		Category:    "amdappsdk",
		Parameters: []ParameterMeta{
			{Name: "node", Type: "int", Default: 16, Description: "Number of nodes"},
			{Name: "iter", Type: "int", Default: 0, Description: "Iterations (0=num nodes)"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return floydwarshall.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*floydwarshall.Benchmark)
			if v, ok := params["node"]; ok {
				bench.NumNodes = uint32(toInt(v))
			}
			if v, ok := params["iter"]; ok {
				bench.NumIterations = uint32(toInt(v))
			}
		},
	},
	"matrixtranspose": {
		Name:        "matrixtranspose",
		Description: "Matrix transpose",
		Category:    "amdappsdk",
		Parameters: []ParameterMeta{
			{Name: "width", Type: "int", Default: 256, Description: "Matrix dimension"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return matrixtranspose.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			if v, ok := params["width"]; ok {
				b.(*matrixtranspose.Benchmark).Width = toInt(v)
			}
		},
	},
	"nbody": {
		Name:        "nbody",
		Description: "N-body simulation",
		Category:    "amdappsdk",
		Parameters: []ParameterMeta{
			{Name: "iter", Type: "int", Default: 8, Description: "Number of iterations"},
			{Name: "particles", Type: "int", Default: 1024, Description: "Number of particles"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return nbody.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*nbody.Benchmark)
			if v, ok := params["iter"]; ok {
				bench.NumIterations = int32(toInt(v))
			}
			if v, ok := params["particles"]; ok {
				bench.NumParticles = int32(toInt(v))
			}
		},
	},
	"simpleconvolution": {
		Name:        "simpleconvolution",
		Description: "Simple 2D convolution",
		Category:    "amdappsdk",
		Parameters: []ParameterMeta{
			{Name: "width", Type: "uint", Default: 254, Description: "Input width"},
			{Name: "height", Type: "uint", Default: 254, Description: "Input height"},
			{Name: "mask-size", Type: "uint", Default: 3, Description: "Mask size"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return simpleconvolution.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*simpleconvolution.Benchmark)
			if v, ok := params["width"]; ok {
				bench.Width = uint32(toInt(v))
			}
			if v, ok := params["height"]; ok {
				bench.Height = uint32(toInt(v))
			}
			if v, ok := params["mask-size"]; ok {
				bench.SetMaskSize(uint32(toInt(v)))
			}
		},
	},

	// Polybench benchmarks
	"atax": {
		Name:        "atax",
		Description: "Matrix transpose and vector multiply",
		Category:    "polybench",
		Parameters: []ParameterMeta{
			{Name: "x", Type: "int", Default: 4096, Description: "Matrix width"},
			{Name: "y", Type: "int", Default: 4096, Description: "Matrix height"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return atax.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*atax.Benchmark)
			if v, ok := params["x"]; ok {
				bench.NX = toInt(v)
			}
			if v, ok := params["y"]; ok {
				bench.NY = toInt(v)
			}
		},
	},
	"bicg": {
		Name:        "bicg",
		Description: "Biconjugate gradient",
		Category:    "polybench",
		Parameters: []ParameterMeta{
			{Name: "x", Type: "int", Default: 4096, Description: "Matrix width"},
			{Name: "y", Type: "int", Default: 4096, Description: "Matrix height"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return bicg.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*bicg.Benchmark)
			if v, ok := params["x"]; ok {
				bench.NX = toInt(v)
			}
			if v, ok := params["y"]; ok {
				bench.NY = toInt(v)
			}
		},
	},

	// SHOC benchmarks
	"bfs": {
		Name:        "bfs",
		Description: "Breadth-first search",
		Category:    "shoc",
		Parameters: []ParameterMeta{
			{Name: "node", Type: "int", Default: 64, Description: "Number of nodes"},
			{Name: "degree", Type: "int", Default: 3, Description: "Node degree"},
			{Name: "depth", Type: "int", Default: 0, Description: "Max depth (0=unlimited)"},
			{Name: "load-graph", Type: "string", Default: "", Description: "Path to graph file"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return bfs.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*bfs.Benchmark)
			if v, ok := params["node"]; ok {
				bench.NumNode = toInt(v)
			}
			if v, ok := params["degree"]; ok {
				bench.Degree = toInt(v)
			}
			if v, ok := params["depth"]; ok {
				depth := toInt(v)
				if depth == 0 {
					depth = 1<<31 - 1
				}
				bench.MaxDepth = depth
			}
			if v, ok := params["load-graph"]; ok {
				bench.Path = toString(v)
			}
		},
	},
	"fft": {
		Name:        "fft",
		Description: "Fast Fourier Transform",
		Category:    "shoc",
		Parameters: []ParameterMeta{
			{Name: "MB", Type: "int", Default: 8, Description: "Data size in MB"},
			{Name: "passes", Type: "int", Default: 2, Description: "Number of passes"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return fft.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*fft.Benchmark)
			if v, ok := params["MB"]; ok {
				bench.Bytes = int32(toInt(v))
			}
			if v, ok := params["passes"]; ok {
				bench.Passes = int32(toInt(v))
			}
		},
	},
	"spmv": {
		Name:        "spmv",
		Description: "Sparse matrix-vector multiply",
		Category:    "shoc",
		Parameters: []ParameterMeta{
			{Name: "dim", Type: "int", Default: 128, Description: "Matrix rows"},
			{Name: "sparsity", Type: "float", Default: 0.01, Description: "Sparsity ratio"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return spmv.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*spmv.Benchmark)
			if v, ok := params["dim"]; ok {
				bench.Dim = int32(toInt(v))
			}
			if v, ok := params["sparsity"]; ok {
				bench.Sparsity = toFloat(v)
			}
		},
	},
	"stencil2d": {
		Name:        "stencil2d",
		Description: "2D stencil computation",
		Category:    "shoc",
		Parameters: []ParameterMeta{
			{Name: "row", Type: "int", Default: 64, Description: "Number of rows"},
			{Name: "col", Type: "int", Default: 64, Description: "Number of columns"},
			{Name: "iter", Type: "int", Default: 5, Description: "Number of iterations"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return stencil2d.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*stencil2d.Benchmark)
			if v, ok := params["row"]; ok {
				bench.NumRows = toInt(v) + 2
			}
			if v, ok := params["col"]; ok {
				bench.NumCols = toInt(v) + 2
			}
			if v, ok := params["iter"]; ok {
				bench.NumIteration = toInt(v)
			}
		},
	},

	// Rodinia benchmarks
	"nw": {
		Name:        "nw",
		Description: "Needleman-Wunsch sequence alignment",
		Category:    "rodinia",
		Parameters: []ParameterMeta{
			{Name: "length", Type: "int", Default: 64, Description: "Sequence length"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return nw.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			if v, ok := params["length"]; ok {
				b.(*nw.Benchmark).SetLength(toInt(v))
			}
		},
	},

	// DNN layer benchmarks
	"conv2d": {
		Name:        "conv2d",
		Description: "2D convolution layer",
		Category:    "dnn",
		Parameters: []ParameterMeta{
			{Name: "N", Type: "int", Default: 1, Description: "Batch size"},
			{Name: "C", Type: "int", Default: 1, Description: "Input channels"},
			{Name: "H", Type: "int", Default: 28, Description: "Input height"},
			{Name: "W", Type: "int", Default: 28, Description: "Input width"},
			{Name: "output-channel", Type: "int", Default: 3, Description: "Output channels"},
			{Name: "kernel-height", Type: "int", Default: 3, Description: "Kernel height"},
			{Name: "kernel-width", Type: "int", Default: 3, Description: "Kernel width"},
			{Name: "pad-x", Type: "int", Default: 0, Description: "Padding X"},
			{Name: "pad-y", Type: "int", Default: 0, Description: "Padding Y"},
			{Name: "stride-x", Type: "int", Default: 1, Description: "Stride X"},
			{Name: "stride-y", Type: "int", Default: 1, Description: "Stride Y"},
			{Name: "enable-backward", Type: "bool", Default: false, Description: "Enable backward pass"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return conv2d.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*conv2d.Benchmark)
			if v, ok := params["N"]; ok {
				bench.N = toInt(v)
			}
			if v, ok := params["C"]; ok {
				bench.C = toInt(v)
			}
			if v, ok := params["H"]; ok {
				bench.H = toInt(v)
			}
			if v, ok := params["W"]; ok {
				bench.W = toInt(v)
			}
			if v, ok := params["output-channel"]; ok {
				bench.KernelChannel = toInt(v)
			}
			if v, ok := params["kernel-height"]; ok {
				bench.KernelHeight = toInt(v)
			}
			if v, ok := params["kernel-width"]; ok {
				bench.KernelWidth = toInt(v)
			}
			if v, ok := params["pad-x"]; ok {
				bench.PadX = toInt(v)
			}
			if v, ok := params["pad-y"]; ok {
				bench.PadY = toInt(v)
			}
			if v, ok := params["stride-x"]; ok {
				bench.StrideX = toInt(v)
			}
			if v, ok := params["stride-y"]; ok {
				bench.StrideY = toInt(v)
			}
			if v, ok := params["enable-backward"]; ok {
				bench.EnableBackward = toBool(v)
			}
		},
	},
	"im2col": {
		Name:        "im2col",
		Description: "Image to column transformation",
		Category:    "dnn",
		Parameters: []ParameterMeta{
			{Name: "N", Type: "int", Default: 1, Description: "Batch size"},
			{Name: "C", Type: "int", Default: 1, Description: "Input channels"},
			{Name: "H", Type: "int", Default: 28, Description: "Input height"},
			{Name: "W", Type: "int", Default: 28, Description: "Input width"},
			{Name: "kernel-height", Type: "int", Default: 3, Description: "Kernel height"},
			{Name: "kernel-width", Type: "int", Default: 3, Description: "Kernel width"},
			{Name: "pad-x", Type: "int", Default: 0, Description: "Padding X"},
			{Name: "pad-y", Type: "int", Default: 0, Description: "Padding Y"},
			{Name: "stride-x", Type: "int", Default: 1, Description: "Stride X"},
			{Name: "stride-y", Type: "int", Default: 1, Description: "Stride Y"},
			{Name: "dilate-x", Type: "int", Default: 1, Description: "Dilation X"},
			{Name: "dilate-y", Type: "int", Default: 1, Description: "Dilation Y"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return im2col.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*im2col.Benchmark)
			if v, ok := params["N"]; ok {
				bench.N = toInt(v)
			}
			if v, ok := params["C"]; ok {
				bench.C = toInt(v)
			}
			if v, ok := params["H"]; ok {
				bench.H = toInt(v)
			}
			if v, ok := params["W"]; ok {
				bench.W = toInt(v)
			}
			if v, ok := params["kernel-height"]; ok {
				bench.KernelHeight = toInt(v)
			}
			if v, ok := params["kernel-width"]; ok {
				bench.KernelWidth = toInt(v)
			}
			if v, ok := params["pad-x"]; ok {
				bench.PadX = toInt(v)
			}
			if v, ok := params["pad-y"]; ok {
				bench.PadY = toInt(v)
			}
			if v, ok := params["stride-x"]; ok {
				bench.StrideX = toInt(v)
			}
			if v, ok := params["stride-y"]; ok {
				bench.StrideY = toInt(v)
			}
			if v, ok := params["dilate-x"]; ok {
				bench.DilateX = toInt(v)
			}
			if v, ok := params["dilate-y"]; ok {
				bench.DilateY = toInt(v)
			}
		},
	},
	"relu": {
		Name:        "relu",
		Description: "ReLU activation",
		Category:    "dnn",
		Parameters: []ParameterMeta{
			{Name: "length", Type: "int", Default: 4096, Description: "Number of elements"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return relu.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			if v, ok := params["length"]; ok {
				b.(*relu.Benchmark).Length = toInt(v)
			}
		},
	},

	// DNN training benchmarks
	"lenet": {
		Name:        "lenet",
		Description: "LeNet training",
		Category:    "dnn",
		Parameters: []ParameterMeta{
			{Name: "epoch", Type: "int", Default: 1, Description: "Number of epochs"},
			{Name: "max-batch-per-epoch", Type: "int", Default: 2, Description: "Batches per epoch"},
			{Name: "batch-size", Type: "int", Default: 32, Description: "Batch size"},
			{Name: "enable-testing", Type: "bool", Default: false, Description: "Enable testing"},
			{Name: "enable-verification", Type: "bool", Default: false, Description: "Enable verification"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return lenet.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*lenet.Benchmark)
			if v, ok := params["epoch"]; ok {
				bench.Epoch = toInt(v)
			}
			if v, ok := params["max-batch-per-epoch"]; ok {
				bench.MaxBatchPerEpoch = toInt(v)
			}
			if v, ok := params["batch-size"]; ok {
				bench.BatchSize = toInt(v)
			}
			if v, ok := params["enable-testing"]; ok {
				bench.EnableTesting = toBool(v)
			}
			if v, ok := params["enable-verification"]; ok {
				bench.EnableVerification = toBool(v)
			}
		},
	},
	"vgg16": {
		Name:        "vgg16",
		Description: "VGG16 training",
		Category:    "dnn",
		Parameters: []ParameterMeta{
			{Name: "epoch", Type: "int", Default: 1, Description: "Number of epochs"},
			{Name: "max-batch-per-epoch", Type: "int", Default: 2, Description: "Batches per epoch"},
			{Name: "batch-size", Type: "int", Default: 8, Description: "Batch size"},
			{Name: "enable-testing", Type: "bool", Default: false, Description: "Enable testing"},
			{Name: "enable-verification", Type: "bool", Default: false, Description: "Enable verification"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return vgg16.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*vgg16.Benchmark)
			if v, ok := params["epoch"]; ok {
				bench.Epoch = toInt(v)
			}
			if v, ok := params["max-batch-per-epoch"]; ok {
				bench.MaxBatchPerEpoch = toInt(v)
			}
			if v, ok := params["batch-size"]; ok {
				bench.BatchSize = toInt(v)
			}
			if v, ok := params["enable-testing"]; ok {
				bench.EnableTesting = toBool(v)
			}
			if v, ok := params["enable-verification"]; ok {
				bench.EnableVerification = toBool(v)
			}
		},
	},
	"minerva": {
		Name:        "minerva",
		Description: "Minerva training",
		Category:    "dnn",
		Parameters: []ParameterMeta{
			{Name: "epoch", Type: "int", Default: 1, Description: "Number of epochs"},
			{Name: "max-batch-per-epoch", Type: "int", Default: 2, Description: "Batches per epoch"},
			{Name: "batch-size", Type: "int", Default: 32, Description: "Batch size"},
			{Name: "enable-testing", Type: "bool", Default: false, Description: "Enable testing"},
			{Name: "enable-verification", Type: "bool", Default: false, Description: "Enable verification"},
		},
		Factory: func(d *driver.Driver) benchmarks.Benchmark { return minerva.NewBenchmark(d) },
		Configure: func(b benchmarks.Benchmark, params map[string]any) {
			bench := b.(*minerva.Benchmark)
			if v, ok := params["epoch"]; ok {
				bench.Epoch = toInt(v)
			}
			if v, ok := params["max-batch-per-epoch"]; ok {
				bench.MaxBatchPerEpoch = toInt(v)
			}
			if v, ok := params["batch-size"]; ok {
				bench.BatchSize = toInt(v)
			}
			if v, ok := params["enable-testing"]; ok {
				bench.EnableTesting = toBool(v)
			}
			if v, ok := params["enable-verification"]; ok {
				bench.EnableVerification = toBool(v)
			}
		},
	},
	"xor": {
		Name:        "xor",
		Description: "XOR neural network training",
		Category:    "dnn",
		Parameters:  []ParameterMeta{},
		Factory:     func(d *driver.Driver) benchmarks.Benchmark { return xor.NewBenchmark(d) },
		Configure:   func(b benchmarks.Benchmark, params map[string]any) {},
	},
}

// GetBenchmarkNames returns a sorted list of benchmark names.
func GetBenchmarkNames() []string {
	names := make([]string, 0, len(Registry))
	for name := range Registry {
		names = append(names, name)
	}
	return names
}

// GetBenchmarksByCategory returns benchmarks filtered by category.
func GetBenchmarksByCategory(category string) []BenchmarkMeta {
	var result []BenchmarkMeta
	for _, meta := range Registry {
		if meta.Category == category {
			result = append(result, meta)
		}
	}
	return result
}

// GetCategories returns all available categories.
func GetCategories() []string {
	categories := make(map[string]bool)
	for _, meta := range Registry {
		categories[meta.Category] = true
	}
	result := make([]string, 0, len(categories))
	for cat := range categories {
		result = append(result, cat)
	}
	return result
}

// Helper functions for type conversion
func toInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		return 0
	default:
		return 0
	}
}

func toFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "1"
	default:
		return false
	}
}

func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}
