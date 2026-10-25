package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

//nolint:funlen // Keep the HTTP allocation, upload, launch and readback scenario together.
func TestSynchronousKernelCopyRoundTrip(t *testing.T) {
	s := simulation.MakeBuilder().Build()
	rt := emusystem.MakeBuilder().WithSimulation(s).Build()
	MakeBuilder().WithRuntime(rt).WithSimulation(s).Build()
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	alloc := func(size int) driver.Ptr {
		body := marshalTestJSON(t, mallocInput{Size: uint64(size)})
		w := httptest.NewRecorder()
		handleMalloc(
			w,
			httptest.NewRequestWithContext(context.Background(), "GET", "/malloc?data="+url.QueryEscape(string(body)), nil),
		)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var out mallocOutput
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return driver.Ptr(out.Ptr)
	}
	upload := func(ptr driver.Ptr, data []float32) {
		var buf bytes.Buffer
		if err := binary.Write(&buf, binary.LittleEndian, data); err != nil {
			t.Fatal(err)
		}
		body := marshalTestJSON(
			t,
			memcopyH2DInput{Ptr: uint64(ptr), Data: base64.StdEncoding.EncodeToString(buf.Bytes())},
		)
		w := httptest.NewRecorder()
		handleMemcopyH2D(
			w,
			httptest.NewRequestWithContext(context.Background(), "POST", "/memcopy_h2d", bytes.NewReader(body)),
		)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	input := make([]float32, 64)
	for i := range input {
		input[i] = float32(i)
	}
	in, out, filter, history := alloc(256), alloc(256), alloc(16), alloc(16)
	upload(in, input)
	upload(filter, []float32{0, 1, 2, 3})
	upload(history, make([]float32, 4))
	code, err := os.ReadFile("../benchmarks/heteromark/fir/kernels.hsaco")
	if err != nil {
		t.Fatal(err)
	}
	var args bytes.Buffer
	if err := binary.Write(
		&args,
		binary.LittleEndian,
		fir.GCN3KernelArgs{Input: in, Output: out, Filter: filter, History: history, NumTaps: 4},
	); err != nil {
		t.Fatal(err)
	}
	body := marshalTestJSON(t, launchKernelInput{
		CodeObject: base64.StdEncoding.EncodeToString(code),
		Args:       base64.StdEncoding.EncodeToString(args.Bytes()),
		NumBlocks:  dim3{1, 1, 1},
		DimBlocks:  dim3{64, 1, 1},
	})
	w := httptest.NewRecorder()
	handleLaunchKernel(
		w,
		httptest.NewRequestWithContext(context.Background(), "POST", "/launch_kernel", bytes.NewReader(body)),
	)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	body = marshalTestJSON(t, memcopyD2HInput{Ptr: uint64(out), Size: 256})
	w = httptest.NewRecorder()
	handleMemcopyD2H(
		w,
		httptest.NewRequestWithContext(
			context.Background(),
			"GET",
			"/memcopy_d2h?data="+url.QueryEscape(string(body)),
			nil,
		),
	)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var response memcopyD2HOutput
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(response.Data)
	if err != nil {
		t.Fatal(err)
	}
	values := make([]float32, 64)
	if err := binary.Read(bytes.NewReader(raw), binary.LittleEndian, values); err != nil {
		t.Fatal(err)
	}
	for i, v := range values {
		var want float32
		for j := 0; j < 4; j++ {
			if i >= j {
				want += float32(i-j) * float32(j)
			}
		}
		if v != want {
			t.Fatalf("HTTP FIR[%d]=%v, want %v", i, v, want)
		}
	}
}

func marshalTestJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
