package training

import "github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"

// DataState owns the resident samples and cursor, independent of open files.
type DataState struct {
	External   *ExternalData `json:"External,omitempty"`
	Data       []float64     `json:"Data,omitempty"`
	Labels     []int         `json:"Labels,omitempty"`
	Shape      []int         `json:"Shape"`
	Descriptor string        `json:"Descriptor"`
	Cursor     int           `json:"Cursor"`
}

type ResidentDataSource struct {
	State    *DataState
	Operator tensor.Operator
	data     []float64
	labels   []int
}

func (d *ResidentDataSource) SavedData() *DataState { return d.State }
func (d *ResidentDataSource) Rewind()               { d.State.Cursor = 0 }
func (d *ResidentDataSource) NextBatch(n int) (tensor.Tensor, []int) {
	s := d.State
	data, labels := s.Data, s.Labels
	if s.External != nil {
		data, labels = d.data, d.labels
	}
	if s.Cursor >= len(labels) {
		return nil, nil
	}
	end := min(s.Cursor+n, len(labels))
	shape := append([]int{end - s.Cursor}, s.Shape...)
	width := 1
	for _, v := range s.Shape {
		width *= v
	}
	out := d.Operator.CreateWithData(data[s.Cursor*width:end*width], shape, s.Descriptor)
	batchLabels := labels[s.Cursor:end]
	s.Cursor = end
	return out, batchLabels
}

// SavedDataSource exposes the actual serializable input cursor state.
type SavedDataSource interface{ SavedData() *DataState }
