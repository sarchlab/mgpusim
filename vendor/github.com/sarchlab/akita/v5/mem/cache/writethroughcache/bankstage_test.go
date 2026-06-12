package writethroughcache

import (
	"testing"

	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
)

func TestBankStageExtractFromBufRoutesMissFillToPostBuffer(t *testing.T) {
	stage := makeBankStageWithAction(bankActionReadMissFill)

	if !stage.extractFromBuf() {
		t.Fatal("extractFromBuf should move a miss-fill transaction")
	}

	next := stage.cache.comp.GetNextState()
	if got := next.BankBufs[0].Elements; len(got) != 0 {
		t.Fatalf("bank buffer = %v, want empty", got)
	}
	if got := next.BankPipelines[0].Stages; len(got) != 0 {
		t.Fatalf("bank pipeline stages = %v, want empty", got)
	}
	if got := next.BankPostBufs[0].Elements; len(got) != 1 || got[0] != 0 {
		t.Fatalf("bank post buffer = %v, want [0]", got)
	}
}

func TestBankStageExtractFromBufRoutesNormalActionToPipeline(t *testing.T) {
	stage := makeBankStageWithAction(bankActionReadHit)

	if !stage.extractFromBuf() {
		t.Fatal("extractFromBuf should move a normal bank transaction")
	}

	next := stage.cache.comp.GetNextState()
	if got := next.BankBufs[0].Elements; len(got) != 0 {
		t.Fatalf("bank buffer = %v, want empty", got)
	}
	if got := next.BankPostBufs[0].Elements; len(got) != 0 {
		t.Fatalf("bank post buffer = %v, want empty", got)
	}
	if got := next.BankPipelines[0].Stages; len(got) != 1 {
		t.Fatalf("bank pipeline stage count = %d, want 1; stages = %v", len(got), got)
	} else if got[0].Item != 0 || got[0].Stage != 0 {
		t.Fatalf("bank pipeline stage = %+v, want item 0 at stage 0", got[0])
	}
}

func makeBankStageWithAction(action bankActionType) *bankStage {
	comp := modeling.NewBuilder[Spec, State]().
		WithSpec(Spec{}).
		Build("BankStageTest")
	comp.SetState(State{
		Transactions: []transactionState{{
			BankAction:    action,
			MissFillDelay: 5,
		}},
		BankBufs: []queueing.Buffer[int]{
			{Cap: 1, Elements: []int{0}},
		},
		BankPipelines: []queueing.Pipeline[int]{
			{Width: 1, NumStages: 3},
		},
		BankPostBufs: []queueing.Buffer[int]{
			{Cap: 1},
		},
	})

	return &bankStage{
		cache:  &pipelineMW{comp: comp},
		bankID: 0,
	}
}
