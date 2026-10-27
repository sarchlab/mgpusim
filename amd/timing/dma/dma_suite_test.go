package dma

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDMA(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DMA Suite")
}

type noopConn struct{}

func (*noopConn) Name() string                   { return "NoopConn" }
func (c *noopConn) BindPort(p messaging.Port)    { p.BindConnection(c) }
func (*noopConn) NotifyAvailable(messaging.Port) {}
func (*noopConn) NotifySend(messaging.Port)      {}
