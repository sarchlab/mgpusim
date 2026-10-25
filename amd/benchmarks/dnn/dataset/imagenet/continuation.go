package imagenet

import (
	"fmt"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

// State saves the lazy source cursor and label tables. The same immutable input
// files must remain available at Root when a fresh process resumes training.
type State struct {
	Root                 string
	ClassToLabel         map[string]int
	TestToClass          map[string]string
	Classes              []string
	Class, Image, Cursor int
	Training             bool
}

func (ds *DataSource) Snapshot() State {
	d := ds.dataSet
	return State{
		d.directory,
		d.mapClassToLabel,
		d.mapTestToClass,
		d.classes,
		d.curClass,
		d.curImage,
		ds.currPtr,
		d.isTrain,
	}
}
func Restore(to tensor.Operator, s State) (*DataSource, error) {
	if getDirPath() != s.Root {
		return nil, fmt.Errorf("ImageNet input root changed: %q != %q", getDirPath(), s.Root)
	}
	return &DataSource{
		to:      to,
		currPtr: s.Cursor,
		dataSet: &DataSet{
			directory:       s.Root,
			mapClassToLabel: s.ClassToLabel,
			mapTestToClass:  s.TestToClass,
			classes:         s.Classes,
			curClass:        s.Class,
			curImage:        s.Image,
			isTrain:         s.Training,
		},
	}, nil
}
