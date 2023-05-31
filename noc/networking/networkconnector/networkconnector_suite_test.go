package networkconnector

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
)

func TestNetworkconnector(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Networkconnector Suite")
}

var _ = Describe("Connector", func() {
	It("should establish route in a simple network", func() {
		engine := sim.NewSerialEngine()
		connector := MakeConnector().
			WithEngine(engine).
			WithDefaultFreq(1 * sim.GHz)
		connector.NewNetwork("Network")

		connector.AddSwitch()

		for i := 0; i < 2; i++ {
			port := sim.NewLimitNumMsgPort(nil, 1, fmt.Sprintf("Port%d", i))
			connector.ConnectDevice(0, []sim.Port{port},
				DeviceToSwitchLinkParameter{
					DeviceEndParam: LinkEndDeviceParameter{
						IncomingBufSize: 1,
						OutgoingBufSize: 1,
					},
					SwitchEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					LinkParam: LinkParameter{
						IsIdeal:       true,
						Frequency:     1 * sim.GHz,
						NumStage:      0,
						CyclePerStage: 0,
						PipelineWidth: 0,
					},
				})
		}

		connector.EstablishRoute()
	})

	It("should establish route in a small tree", func() {
		engine := sim.NewSerialEngine()
		connector := MakeConnector().
			WithEngine(engine).
			WithDefaultFreq(1 * sim.GHz)
		connector.NewNetwork("Network")

		for i := 0; i < 3; i++ {
			connector.AddSwitch()
		}

		for i := 0; i < 2; i++ {
			port := sim.NewLimitNumMsgPort(nil, 1, fmt.Sprintf("Port%d", i))
			connector.ConnectDevice(1+i, []sim.Port{port},
				DeviceToSwitchLinkParameter{
					DeviceEndParam: LinkEndDeviceParameter{
						IncomingBufSize: 1,
						OutgoingBufSize: 1,
					},
					SwitchEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					LinkParam: LinkParameter{
						IsIdeal:       true,
						Frequency:     1 * sim.GHz,
						NumStage:      0,
						CyclePerStage: 0,
						PipelineWidth: 0,
					},
				})
		}

		for i := 1; i < 3; i++ {
			connector.ConnectSwitches(i, (i-1)/2,
				SwitchToSwitchLinkParameter{
					LeftEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					RightEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					LinkParam: LinkParameter{
						IsIdeal:       true,
						Frequency:     1 * sim.GHz,
						NumStage:      0,
						CyclePerStage: 0,
						PipelineWidth: 0,
					},
				})
		}

		connector.EstablishRoute()
	})

	It("should establish route in a large tree", func() {
		engine := sim.NewSerialEngine()
		connector := MakeConnector().
			WithEngine(engine).
			WithDefaultFreq(1 * sim.GHz)
		connector.NewNetwork("Network")

		for i := 0; i < 16; i++ {
			connector.AddSwitch()
		}

		for i := 0; i < 8; i++ {
			port := sim.NewLimitNumMsgPort(nil, 1, fmt.Sprintf("Port%d", i))
			connector.ConnectDevice(8+i, []sim.Port{port},
				DeviceToSwitchLinkParameter{
					DeviceEndParam: LinkEndDeviceParameter{
						IncomingBufSize: 1,
						OutgoingBufSize: 1,
					},
					SwitchEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					LinkParam: LinkParameter{
						IsIdeal:       true,
						Frequency:     1 * sim.GHz,
						NumStage:      0,
						CyclePerStage: 0,
						PipelineWidth: 0,
					},
				})
		}

		for i := 1; i < 16; i++ {
			connector.ConnectSwitches(i, i/2,
				SwitchToSwitchLinkParameter{
					LeftEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					RightEndParam: LinkEndSwitchParameter{
						IncomingBufSize:  1,
						OutgoingBufSize:  1,
						NumInputChannel:  1,
						NumOutputChannel: 1,
						Latency:          1,
					},
					LinkParam: LinkParameter{
						IsIdeal:       true,
						Frequency:     1 * sim.GHz,
						NumStage:      0,
						CyclePerStage: 0,
						PipelineWidth: 0,
					},
				})
		}

		connector.EstablishRoute()
	})
})
