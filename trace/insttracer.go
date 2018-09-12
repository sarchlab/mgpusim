package trace

import (
	"io"
	"log"
	"reflect"
	"sort"
	"sync"

	"encoding/binary"

	"github.com/golang/protobuf/proto"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/timing"
	"gitlab.com/akita/gcn3/trace/instpb"
)

// A InstTracer is a LogHook that keep record of instruction execution status
type InstTracer struct {
	mutex        sync.Mutex
	writer       io.Writer
	tracingInsts map[*timing.Inst]*instpb.Inst
}

// NewInstTracer creates a new InstTracer
func NewInstTracer(writer io.Writer) *InstTracer {
	t := new(InstTracer)
	t.writer = writer
	t.tracingInsts = make(map[*timing.Inst]*instpb.Inst)
	return t
}

// Type of InstTracer claims the inst tracer is hooking to the timing.Wavefront type
func (t *InstTracer) Type() reflect.Type {
	return reflect.TypeOf((*timing.Wavefront)(nil))
}

// Pos of InstTracer returns akita.AnyHookPos. Since InstTracer is not standard hook
// for event or request, it has to use akita.AnyHookPos position.
func (t *InstTracer) Pos() akita.HookPos {
	return akita.AnyHookPos
}

// Func defines the behavior of the tracer when the tracer is invoked.
func (t *InstTracer) Func(item interface{}, domain akita.Hookable, info interface{}) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	wf := item.(*timing.Wavefront)
	instInfo := info.(*timing.InstHookInfo)
	inst := instInfo.Inst

	instTraceItem, ok := t.tracingInsts[inst]
	if !ok {
		instTraceItem = new(instpb.Inst)
		instTraceItem.Events = make([]*instpb.Event, 0)
		t.tracingInsts[inst] = instTraceItem
	}

	switch instInfo.Stage {
	case "FetchStart":
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_FetchStart,
			},
		)
	case "FetchDone":
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_FetchDone,
			},
		)
	case "Issue":
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_Issue,
			})
	case "DecodeStart":
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_DecodeStart,
			})
	case "DecodeDone":
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_DecodeDone,
			})
	case "ReadStart":
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_ReadStart,
			})
	case "ReadDone":
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_ReadDone,
			})
	case "ExecStart":
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_ExecStart,
			})
	case "ExecDone":
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_ExecDone,
			})
	case "WriteStart":
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_WriteStart,
			})
	case "WriteDone":
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_WriteDone,
			})
	case "WaitMem":
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_WaitMem,
			})
	case "MemReturn":
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_MemReturn,
			})
	case "Completed":
		instTraceItem.Id = inst.ID
		instTraceItem.Asm = inst.String(nil)
		instTraceItem.WavefrontId = uint32(wf.FirstWiFlatID)
		instTraceItem.SimdId = uint32(wf.SIMDID)
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_Complete,
			},
		)

		sort.Slice(instTraceItem.Events, func(i, j int) bool {
			return instTraceItem.Events[i].Stage < instTraceItem.Events[j].Stage
		})

		data, err := proto.Marshal(instTraceItem)
		if err != nil {
			log.Panic(err)
		}

		size := uint32(len(data))
		err = binary.Write(t.writer, binary.LittleEndian, size)
		if err != nil {
			log.Panic(err)
		}

		_, err = t.writer.Write(data)
		if err != nil {
			log.Panic(err)
		}

		delete(t.tracingInsts, inst)
	}
}
