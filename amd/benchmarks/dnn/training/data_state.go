package training

import "github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"

// DataState owns the resident samples and cursor, independent of open files.
type DataState struct {
	Data       []float64
	Labels     []int
	Shape      []int
	Descriptor string
	Cursor     int
}
type ResidentDataSource struct {
	State    DataState
	Operator tensor.Operator
}

func (d *ResidentDataSource) SnapshotData() DataState { return d.State }
func (d *ResidentDataSource) Rewind()                 { d.State.Cursor = 0 }
func (d *ResidentDataSource) NextBatch(n int) (tensor.Tensor, []int) {
	s := &d.State
	if s.Cursor >= len(s.Labels) {
		return nil, nil
	}
	end := min(s.Cursor+n, len(s.Labels))
	shape := append([]int{end - s.Cursor}, s.Shape...)
	width := 1
	for _, v := range s.Shape {
		width *= v
	}
	out := d.Operator.CreateWithData(s.Data[s.Cursor*width:end*width], shape, s.Descriptor)
	labels := s.Labels[s.Cursor:end]
	s.Cursor = end
	return out, labels
}
