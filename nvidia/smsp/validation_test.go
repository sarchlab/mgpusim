package smsp

import (
	"testing"

	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

func TestRejectInvalidPeers(t *testing.T) {
	for _, spec := range []Spec{{}, {ToSM: "sm"}, {ToVectorMem: "memory"}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("invalid Spec accepted: %+v", spec)
				}
			}()
			c := &Comp{}
			c.Resources.Traces = &trace.Store{}
			c.Spec = spec
			c.State.Initialize(c)
		}()
	}
}
