package org

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/addressmapping"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
)

var _ = Describe("ChannelImpl", func() {
	var (
		mockCtrl *gomock.Controller
		channel  ChannelImpl
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		channel = ChannelImpl{}

		channel.Banks = MakeBanks(2, 2, 2)
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				for k := 0; k < 2; k++ {
					channel.Banks[i][j][k] = NewMockBank(mockCtrl)
				}
			}
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should get ready command from the corresponding bank", func() {
		cmd := &signal.Command{
			Kind: signal.CmdKindRead,
			Location: addressmapping.Location{
				Rank:      0,
				BankGroup: 0,
				Bank:      0,
			},
		}
		retCmd := &signal.Command{
			Kind: signal.CmdKindActivate,
		}

		channel.Banks.GetBank(0, 0, 0).(*MockBank).EXPECT().
			GetReadyCommand(sim.VTimeInSec(10), cmd).
			Return(retCmd)

		finalCmd := channel.GetReadyCommand(10, cmd)

		Expect(finalCmd).To(Equal(retCmd))
	})

	It("should update the state of the corresponding bank", func() {
		cmd := &signal.Command{
			Kind: signal.CmdKindRead,
			Location: addressmapping.Location{
				Rank:      0,
				BankGroup: 0,
				Bank:      0,
			},
		}

		channel.Banks.GetBank(0, 0, 0).(*MockBank).EXPECT().
			StartCommand(sim.VTimeInSec(10), cmd)

		channel.StartCommand(10, cmd)
	})

	It("should update timing", func() {
		t := Timing{}

		t.SameBank = MakeTimeTable()
		t.SameBank[signal.CmdKindRead] = []TimeTableEntry{
			{signal.CmdKindRead, 1},
		}

		t.OtherBanksInBankGroup = MakeTimeTable()
		t.OtherBanksInBankGroup[signal.CmdKindRead] = []TimeTableEntry{
			{signal.CmdKindRead, 2},
		}

		t.SameRank = MakeTimeTable()
		t.SameRank[signal.CmdKindRead] = []TimeTableEntry{
			{signal.CmdKindRead, 3},
		}

		t.OtherRanks = MakeTimeTable()
		t.OtherRanks[signal.CmdKindRead] = []TimeTableEntry{
			{signal.CmdKindRead, 4},
		}

		channel.Timing = t

		cmd := &signal.Command{
			Kind: signal.CmdKindRead,
			Location: addressmapping.Location{
				Rank:      0,
				BankGroup: 0,
				Bank:      0,
			},
		}

		channel.Banks.GetBank(0, 0, 0).(*MockBank).EXPECT().
			UpdateTiming(signal.CmdKindRead, 1)
		channel.Banks.GetBank(0, 0, 1).(*MockBank).EXPECT().
			UpdateTiming(signal.CmdKindRead, 2)
		channel.Banks.GetBank(0, 1, 0).(*MockBank).EXPECT().
			UpdateTiming(signal.CmdKindRead, 3)
		channel.Banks.GetBank(0, 1, 1).(*MockBank).EXPECT().
			UpdateTiming(signal.CmdKindRead, 3)
		channel.Banks.GetBank(1, 0, 0).(*MockBank).EXPECT().
			UpdateTiming(signal.CmdKindRead, 4)
		channel.Banks.GetBank(1, 0, 1).(*MockBank).EXPECT().
			UpdateTiming(signal.CmdKindRead, 4)
		channel.Banks.GetBank(1, 1, 0).(*MockBank).EXPECT().
			UpdateTiming(signal.CmdKindRead, 4)
		channel.Banks.GetBank(1, 1, 1).(*MockBank).EXPECT().
			UpdateTiming(signal.CmdKindRead, 4)

		channel.UpdateTiming(10, cmd)
	})
})
