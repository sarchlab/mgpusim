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

func Bind(to tensor.Operator, s *State) (*DataSource, error) {
	if s == nil {
		return nil, fmt.Errorf("missing ImageNet state")
	}
	if getDirPath() != s.Root {
		return nil, fmt.Errorf("ImageNet input root changed: %q != %q", getDirPath(), s.Root)
	}
	return &DataSource{State: s, to: to, dataSet: &DataSet{State: s}}, nil
}
