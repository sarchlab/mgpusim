package trace

import (
	"io"
	"log"
	"reflect"
	"sync"

	"encoding/binary"

	"github.com/golang/protobuf/proto"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/timing/cu"
	"gitlab.com/yaotsu/gcn3/trace/instpb"
)

// A InstTracer is a LogHook that keep record of instruction execution status
type InstTracer struct {
	mutex        sync.Mutex
	writer       io.Writer
	tracingInsts map[*cu.Inst]*instpb.Inst
}

// NewInstTracer creates a new InstTracer
func NewInstTracer(writer io.Writer) *InstTracer {
	t := new(InstTracer)
	t.writer = writer
	t.tracingInsts = make(map[*cu.Inst]*instpb.Inst)
	return t
}

// Type of InstTracer claims the inst tracer is hooking to the cu.Wavefront type
func (t *InstTracer) Type() reflect.Type {
	return reflect.TypeOf((*cu.Wavefront)(nil))
}

// Pos of InstTracer returns core.Any. Since InstTracer is not standard hook
// for event or request, it has to use core.Any position.
func (t *InstTracer) Pos() core.HookPos {
	return core.Any
}

// Func defines the behavior of the tracer when the tracer is invoked.
func (t *InstTracer) Func(item interface{}, domain core.Hookable, info interface{}) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	wf := item.(*cu.Wavefront)
	wf.RLock()
	inst := wf.Inst
	wf.RUnlock()
	instInfo := info.(*cu.InstHookInfo)

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
	case "Completed":
		instTraceItem.Id = inst.ID
		instTraceItem.Asm = inst.String()
		instTraceItem.Events = append(instTraceItem.Events,
			&instpb.Event{
				Time:  float64(instInfo.Now),
				Stage: instpb.Stage_Complete,
			},
		)

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
	}
}
