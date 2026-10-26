package mnist

import (
	"path/filepath"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training"
)

// DataSource feeds samples from a validated external dataset.
type DataSource struct{ *training.ResidentDataSource }

func NewTrainingDataSource(to tensor.Operator) *DataSource { return newSource(to, true) }
func NewTestDataSource(to tensor.Operator) *DataSource     { return newSource(to, false) }
func newSource(to tensor.Operator, train bool) *DataSource {
	var files []string
	prefix := "train"
	if !train {
		prefix = "t10k"
	}
	files = []string{
		filepath.Join(getDirPath(), prefix+"-images-idx3-ubyte.gz"),
		filepath.Join(getDirPath(), prefix+"-labels-idx1-ubyte.gz"),
	}
	d, err := training.NewExternalDataSource(to, "mnist", files, []int{1, 28, 28}, "NCHW")
	if err != nil {
		panic(err)
	}
	return &DataSource{d}
}
