package dma

import (
	"strconv"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

func TestPendingRemovalPreservesOrderAndClearsTail(t *testing.T) {
	for _, id := range []uint64{1, 2, 3} {
		t.Run(strconv.FormatUint(id, 10), func(t *testing.T) {
			g := NewWithT(t)
			pending := []messaging.Msg{
				{ID: 1, Payload: memprotocol.WriteReq{Data: []byte{1}}},
				{ID: 2, Payload: memprotocol.WriteReq{Data: []byte{2}}},
				{ID: 3, Payload: memprotocol.WriteReq{Data: []byte{3}}},
			}
			mw := dmaMiddleware{comp: &Comp{}}
			mw.comp.State.PendingReqs = pending
			removed := mw.removeReqFromPendingReqList(id)
			g.Expect(removed.ID).To(Equal(id))
			g.Expect(removed.Payload.(memprotocol.WriteReq).Data).To(Equal([]byte{byte(id)}))
			var want []uint64
			for candidate := uint64(1); candidate <= 3; candidate++ {
				if candidate != id {
					want = append(want, candidate)
				}
			}
			got := mw.comp.State.PendingReqs
			g.Expect([]uint64{got[0].ID, got[1].ID}).To(Equal(want))
			g.Expect(pending[2]).To(Equal(messaging.Msg{}))
		})
	}
}

func TestPendingRemovalDuplicatesAndMissingID(t *testing.T) {
	g := NewWithT(t)
	mw := dmaMiddleware{comp: &Comp{}}
	mw.comp.State.PendingReqs = []messaging.Msg{
		{ID: 1}, {ID: 2}, {ID: 1, TrafficBytes: 3},
	}
	removed := mw.removeReqFromPendingReqList(1)
	g.Expect(removed.TrafficBytes).To(Equal(3))
	g.Expect(mw.comp.State.PendingReqs).To(Equal([]messaging.Msg{{ID: 2}}))
	g.Expect(func() { mw.removeReqFromPendingReqList(9) }).To(Panic())
	g.Expect(mw.comp.State.PendingReqs).To(Equal([]messaging.Msg{{ID: 2}}))
	mw.removeReqFromPendingReqList(2)
	g.Expect(mw.comp.State.PendingReqs).To(BeEmpty())
	g.Expect(func() { mw.removeReqFromPendingReqList(2) }).To(Panic())
}

func TestPendingRemovalDoesNotAllocate(t *testing.T) {
	mw := dmaMiddleware{comp: &Comp{}}
	pending := []messaging.Msg{{ID: 1}, {ID: 2}, {ID: 3}}
	allocs := testing.AllocsPerRun(100, func() {
		for i := range pending {
			pending[i].ID = uint64(i + 1)
		}
		mw.comp.State.PendingReqs = pending
		mw.removeReqFromPendingReqList(2)
	})
	NewWithT(t).Expect(allocs).To(BeZero())
}

func BenchmarkPendingRemoval(b *testing.B) {
	mw := dmaMiddleware{comp: &Comp{}}
	pending := make([]messaging.Msg, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range pending {
			pending[j].ID = uint64(j + 1)
		}
		mw.comp.State.PendingReqs = pending
		mw.removeReqFromPendingReqList(2048)
	}
}
