package dma

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
)

func TestModelingContract(t *testing.T) {
	modelingtest.CheckTicking(t, (*Comp)(nil), DefaultSpec())
}
