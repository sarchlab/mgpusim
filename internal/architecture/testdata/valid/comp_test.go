package valid_test

import (
	checks "github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	fixture "github.com/sarchlab/mgpusim/v5/internal/architecture/testdata/valid"
	"testing"
)

func TestModel(t *testing.T) { checks.CheckTicking(t, (*fixture.Comp)(nil), fixture.Spec{}) }
