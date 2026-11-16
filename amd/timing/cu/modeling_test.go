package cu

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
)

func TestModelingContract(t *testing.T) {
	modelingtest.CheckTicking(t, (*Comp)(nil), DefaultSpec())
}

func TestDefaultSpecOwnsRegisterCounts(t *testing.T) {
	first := DefaultSpec()
	first.VGPRCounts[0] = 1
	if got := DefaultSpec().VGPRCounts[0]; got != 16384 {
		t.Fatalf("mutating one default changed another: %d", got)
	}
	if got := MakeBuilder().spec.VGPRCounts[0]; got != 16384 {
		t.Fatalf("mutating a default changed a new builder: %d", got)
	}
}
