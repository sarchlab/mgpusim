package cifar10

import (
	"fmt"
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
	first := 1
	if !train {
		first = 5
	} // Preserve the existing test source selection.
	for i := first; i <= 5; i++ {
		files = append(files, filepath.Join(getDirPath(), "cifar-10-batches-bin", fmt.Sprintf("data_batch_%d.bin", i)))
	}
	d, err := training.NewExternalDataSource(to, "cifar10", files, []int{3, 32, 32}, "NCHW")
	if err != nil {
		panic(err)
	}
	return &DataSource{d}
}
