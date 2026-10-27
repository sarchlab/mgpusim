package sm

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

func TestRejectInvalidPeers(t *testing.T) {
	for _, spec := range []Spec{
		{}, {ToGPU: "gpu", SMSPs: []messaging.RemotePort{""}},
		{ToGPU: "gpu", SMSPs: []messaging.RemotePort{"port", "port"}},
	} {
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
