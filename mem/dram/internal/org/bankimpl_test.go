package org

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
)

var _ = Describe("Bank", func() {
	var (
		b BankImpl
	)

	BeforeEach(func() {
		b = BankImpl{
			cyclesToCmdAvailable: make(map[signal.CommandKind]int),
			CmdCycles: map[signal.CommandKind]int{
				signal.CmdKindRead:           1,
				signal.CmdKindReadPrecharge:  1,
				signal.CmdKindWrite:          1,
				signal.CmdKindWritePrecharge: 1,
				signal.CmdKindActivate:       6,
				signal.CmdKindPrecharge:      1,
				signal.CmdKindRefreshBank:    1,
				signal.CmdKindRefresh:        1,
				signal.CmdKindSRefEnter:      1,
				signal.CmdKindSRefExit:       1,
			},
		}
	})

	Context("tick", func() {
		It("should reduce timing count", func() {
			subTrans := &signal.SubTransaction{}
			cmd := &signal.Command{
				Kind:      signal.CmdKindRead,
				SubTrans:  subTrans,
				CycleLeft: 1,
			}
			b.currentCmd = cmd
			b.cyclesToCmdAvailable[signal.CmdKindRead] = 2
			b.cyclesToCmdAvailable[signal.CmdKindPrecharge] = 1

			madeProgress := b.Tick(10)

			Expect(madeProgress).To(BeTrue())

			Expect(b.cyclesToCmdAvailable[signal.CmdKindRead]).To(Equal(1))
			Expect(b.cyclesToCmdAvailable[signal.CmdKindPrecharge]).To(Equal(0))
			Expect(b.cyclesToCmdAvailable[signal.CmdKindActivate]).To(Equal(0))

			Expect(cmd.CycleLeft).To(Equal(0))
			Expect(subTrans.Completed).To(BeTrue())
			Expect(b.currentCmd).To(BeNil())
		})
	})

	Context("bank state closed", func() {
		BeforeEach(func() {
			b.state = BankStateClosed
		})

		Context("read command", func() {
			It("should return activate command", func() {
				subTrans := &signal.SubTransaction{}
				readCmd := &signal.Command{
					Kind:     signal.CmdKindRead,
					SubTrans: subTrans,
				}
				b.cyclesToCmdAvailable[signal.CmdKindActivate] = 0

				readyCmd := b.GetReadyCommand(10, readCmd)

				Expect(readyCmd.Kind).To(Equal(signal.CmdKindActivate))
				Expect(readyCmd.SubTrans).To(BeIdenticalTo(subTrans))
			})
		})

		Context("activate", func() {
			It("should open row", func() {
				cmd := &signal.Command{
					Kind:     signal.CmdKindActivate,
					SubTrans: &signal.SubTransaction{},
				}
				cmd.Row = 1

				b.StartCommand(10, cmd)

				Expect(b.state).To(Equal(BankStateOpen))
				Expect(b.openRow).To(Equal(uint64(1)))
				Expect(b.currentCmd).To(BeIdenticalTo(cmd))
				Expect(cmd.CycleLeft).To(Equal(6))
			})
		})
	})

	Context("bank state open", func() {
		var (
			readCmd *signal.Command
		)
		BeforeEach(func() {
			b.state = BankStateOpen
			readCmd = &signal.Command{
				Kind:     signal.CmdKindRead,
				SubTrans: &signal.SubTransaction{},
			}
			readCmd.Row = 6
		})

		Context("read command", func() {
			It("should do the read if the row is open", func() {
				b.openRow = 6
				b.cyclesToCmdAvailable[signal.CmdKindRead] = 0

				cmd := b.GetReadyCommand(10, readCmd)

				Expect(cmd.Kind).To(Equal(signal.CmdKindRead))
			})

			It("should do the precharge if another row is open", func() {
				b.openRow = 7
				b.cyclesToCmdAvailable[signal.CmdKindPrecharge] = 0

				cmd := b.GetReadyCommand(10, readCmd)

				Expect(cmd.Kind).To(Equal(signal.CmdKindPrecharge))
			})
		})

		Context("precharge", func() {
			It("should close", func() {
				cmd := &signal.Command{
					Kind:     signal.CmdKindPrecharge,
					SubTrans: &signal.SubTransaction{},
				}
				cmd.Row = 1

				b.StartCommand(10, cmd)

				Expect(b.state).To(Equal(BankStateClosed))
			})
		})
	})

	It("should update timing", func() {
		b.cyclesToCmdAvailable[signal.CmdKindActivate] = 10
		b.cyclesToCmdAvailable[signal.CmdKindRead] = 6

		b.UpdateTiming(signal.CmdKindActivate, 8)
		b.UpdateTiming(signal.CmdKindRead, 8)

		Expect(b.cyclesToCmdAvailable[signal.CmdKindActivate]).To(Equal(10))
		Expect(b.cyclesToCmdAvailable[signal.CmdKindRead]).To(Equal(8))
	})

})
