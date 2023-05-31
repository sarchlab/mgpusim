package switching

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v3/sim Port,Engine,Buffer
//go:generate mockgen -destination "mock_pipelining_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v3/pipelining Pipeline
//go:generate mockgen -destination "mock_routing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/noc/networking/routing Table
//go:generate mockgen -destination "mock_arbitration_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/noc/networking/arbitration Arbiter

func TestSwitching(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Switching Suite")
}

type sampleMsg struct {
	sim.MsgMeta
}

func (m *sampleMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}
