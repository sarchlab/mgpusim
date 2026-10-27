package platform_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	directconnection "github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/nvidia/gpu"
	"github.com/sarchlab/mgpusim/v5/nvidia/sm"
	"github.com/sarchlab/mgpusim/v5/nvidia/smsp"
)

func TestPeerChangesRejectCheckpoint(t *testing.T) {
	builders := map[string]func(*sim.Simulation, messaging.RemotePort){
		"GPU": func(s *sim.Simulation, remote messaging.RemotePort) {
			spec := gpu.DefaultSpec()
			spec.ToDriver = remote
			spec.SMs = []gpu.SMPeer{{Name: "SM", Port: remote}}
			c := modeling.MakeBuilder[*gpu.Comp](spec).WithSimulation(s).Build("GPU")
			bindTopologyPorts(s, c, "ToDriver", "ToSMs")
		},
		"SM": func(s *sim.Simulation, remote messaging.RemotePort) {
			spec := sm.DefaultSpec()
			spec.ToGPU = remote
			spec.SMSPs = []messaging.RemotePort{remote}
			c := modeling.MakeBuilder[*sm.Comp](spec).WithSimulation(s).Build("SM")
			bindTopologyPorts(s, c, "ToGPU", "ToSMSPs")
		},
		"SMSP": func(s *sim.Simulation, remote messaging.RemotePort) {
			spec := smsp.DefaultSpec()
			spec.ToSM = remote
			spec.ToVectorMem = remote
			c := modeling.MakeBuilder[*smsp.Comp](spec).WithSimulation(s).Build("SMSP")
			bindTopologyPorts(s, c, "ToSM", "ToVectorMem")
		},
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) { checkChangedPeer(t, build) })
	}
}

func checkChangedPeer(t *testing.T, build func(*sim.Simulation, messaging.RemotePort)) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "checkpoint.tar.gz")
	for i, remote := range []messaging.RemotePort{"peer", "peer", "changed"} {
		s := sim.MakeBuilder().Build()
		t.Cleanup(func() {
			if err := s.Terminate(); err != nil {
				t.Error(err)
			}
		})
		build(s, remote)
		if err := s.Initialize(); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if err := s.SaveCheckpoint(path); err != nil {
				t.Fatal(err)
			}
			continue
		}
		err := s.LoadCheckpoint(path)
		if i == 1 && err != nil {
			t.Fatal(err)
		}
		if i == 2 && (err == nil || !strings.Contains(err.Error(), "spec")) {
			t.Fatalf("changed peer accepted: %v", err)
		}
	}
}

func bindTopologyPorts(s *sim.Simulation, c interface{ BindPort(string, messaging.Port) }, slots ...string) {
	conn := directconnection.NewConnection("Connection", s, timing.GHz)
	for _, slot := range slots {
		p := twowaybuffered.MakeBuilder().WithSimulation(s).WithSpec(twowaybuffered.DefaultSpec()).Build()
		c.BindPort(slot, p)
		conn.BindPort(p)
	}
}
