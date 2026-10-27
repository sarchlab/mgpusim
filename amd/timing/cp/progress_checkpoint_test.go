package cp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

func progressMonitor(t *testing.T) (*monitoring.Monitor, func() []monitoring.ProgressBarSnapshot) {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	monitor := monitoring.MakeBuilder().WithPortNumber(port).Build()
	client := &http.Client{Timeout: 5 * time.Second}
	read := func() []monitoring.ProgressBarSnapshot {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
			fmt.Sprintf("http://127.0.0.1:%d/api/progress", port), nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var bars []monitoring.ProgressBarSnapshot
		if err := json.NewDecoder(response.Body).Decode(&bars); err != nil {
			t.Fatal(err)
		}
		return bars
	}
	return monitor, read
}

// Exercise real LoadCheckpoint on an already initialized component and inspect
// the monitor's public list, not just a retained pointer to one progress bar.
//
//nolint:funlen // Keep the successive rewind and observer lifecycle assertions together.
func TestProgressBarCheckpointRewind(t *testing.T) {
	monitor, read := progressMonitor(t)
	_, c := checkpointCP(t, "round-robin", nil, monitor)
	state := &c.State.Dispatchers[0]
	state.IsDispatching = true
	state.Dispatching = messaging.Msg{ID: c.NewID(), Payload: protocol.LaunchKernelReq{}}
	state.Algorithm.Grid.NumWG = 10
	state.NumDispatchedWGs = 6
	state.NumCompletedWGs = 4
	state.CycleLeft = 100 // Inspect observer synchronization without dispatching work.
	tick := func() { c.Middlewares.Data.dispatchers[0].Tick() }
	expect := func(total, inProgress, finished uint64) monitoring.ProgressBarSnapshot {
		t.Helper()
		bars := read()
		if len(bars) != 1 {
			t.Fatalf("expected exactly one bar, got %d", len(bars))
		}
		bar := bars[0]
		if bar.Total != total || bar.InProgress != inProgress || bar.Finished != finished {
			t.Fatalf("bar counts = %d/%d/%d, want %d/%d/%d",
				bar.Total, bar.InProgress, bar.Finished, total, inProgress, finished)
		}
		return bar
	}
	var early bytes.Buffer
	if err := c.SaveCheckpoint(&early); err != nil {
		t.Fatal(err)
	}
	tick()
	first := expect(10, 2, 4)
	tick()
	if got := expect(10, 2, 4); got.ID != first.ID {
		t.Fatal("unchanged state recreated a progress bar")
	}
	state.NumDispatchedWGs = 8
	state.NumCompletedWGs = 6
	tick()
	advanced := expect(10, 2, 6)
	if err := c.LoadCheckpoint(bytes.NewReader(early.Bytes())); err != nil {
		t.Fatal(err)
	}
	tick()
	rewound := expect(10, 2, 4)
	if rewound.ID == advanced.ID {
		t.Fatal("same-kernel rewind did not replace obsolete counters")
	}
	if err := c.LoadCheckpoint(bytes.NewReader(early.Bytes())); err != nil {
		t.Fatal(err)
	}
	tick()
	if got := expect(10, 2, 4); got.ID != rewound.ID {
		t.Fatal("identical checkpoint recreated a bar")
	}
	state = &c.State.Dispatchers[0]
	state.Algorithm.Grid.NumWG = 12
	tick()
	expect(12, 2, 4)
	state.Dispatching.ID = c.NewID()
	tick()
	changed := expect(12, 2, 4)
	if changed.Name == rewound.Name {
		t.Fatal("new kernel retained old bar name")
	}
	state.IsDispatching = false
	var idle bytes.Buffer
	if err := c.SaveCheckpoint(&idle); err != nil {
		t.Fatal(err)
	}
	if err := c.LoadCheckpoint(bytes.NewReader(early.Bytes())); err != nil {
		t.Fatal(err)
	}
	tick()
	expect(10, 2, 4)
	if err := c.LoadCheckpoint(bytes.NewReader(idle.Bytes())); err != nil {
		t.Fatal(err)
	}
	tick()
	if bars := read(); len(bars) != 0 {
		t.Fatalf("idle restore left %d orphan bars", len(bars))
	}
}

func TestProgressBarRemovedOnKernelCompletion(t *testing.T) {
	monitor, read := progressMonitor(t)
	_, c := checkpointCP(t, "round-robin", nil, monitor)
	state := &c.State.Dispatchers[0]
	state.IsDispatching = true
	state.Dispatching = messaging.Msg{ID: c.NewID(), Src: c.Spec.Driver, Payload: protocol.LaunchKernelReq{}}
	state.Algorithm.Grid.NumWG = 1
	state.Algorithm.Dispatched = 1
	state.NumDispatchedWGs = 1
	state.NumCompletedWGs = 1
	state.CycleLeft = 1
	dispatcher := c.Middlewares.Data.dispatchers[0]
	dispatcher.Tick()
	if bars := read(); len(bars) != 1 {
		t.Fatalf("expected active completion bar, got %d", len(bars))
	}
	dispatcher.Tick()
	if state.IsDispatching {
		t.Fatal("kernel did not complete")
	}
	if bars := read(); len(bars) != 0 {
		t.Fatalf("completion left %d bars", len(bars))
	}
	dispatcher.Tick()
	if bars := read(); len(bars) != 0 {
		t.Fatal("idle tick recreated completed bar")
	}
}
