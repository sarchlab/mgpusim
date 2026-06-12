package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	// Enable profiling.
	_ "net/http/pprof"

	"github.com/google/pprof/profile"
	"github.com/sarchlab/akita/v5/daisen"
	"github.com/sarchlab/akita/v5/daisen/static"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/shirou/gopsutil/process"
	"github.com/syifan/goseth"
)

// Monitor provides live simulation monitoring capabilities. It serves HTTP
// endpoints for engine control, component inspection, progress bars,
// resource monitoring, and embeds Daisen's trace replay functionality.
type Monitor struct {
	// Configuration (set before StartServer).
	port        int
	engine      sim.Engine
	traceDBPath string
	visTracer   *tracing.DBTracer

	// Internal state.
	components       []sim.Component
	buffers          []bufferState
	progressBarsLock sync.Mutex
	progressBars     []*daisen.ProgressBar
	httpServer       *http.Server
	replayServer     *daisen.Server // for trace endpoints
	fs               http.FileSystem
}

// NewMonitor creates a new Monitor with default settings. The monitor is not
// started until StartServer() is called.
func NewMonitor() *Monitor {
	return &Monitor{
		fs: static.GetAssets(),
	}
}

// WithPortNumber sets the port number for the monitoring server. Returns the
// Monitor for method chaining.
func (m *Monitor) WithPortNumber(port int) *Monitor {
	if port < 1000 {
		fmt.Fprintf(os.Stderr,
			"Port number %d is assigned to the server, "+
				"which is not allowed. Using a random port instead.\n",
			port,
		)
		port = 0
	}

	m.port = port

	return m
}

// RegisterEngine registers the simulation engine with the monitor.
func (m *Monitor) RegisterEngine(e sim.Engine) {
	m.engine = e
}

// RegisterComponent registers a component with the monitor so its internal
// state can be inspected via the monitoring server.
func (m *Monitor) RegisterComponent(c sim.Component) {
	m.components = append(m.components, c)
	m.registerBuffers(c)
}

// RegisterVisTracer registers a visualization tracer with the monitor.
func (m *Monitor) RegisterVisTracer(tr *tracing.DBTracer) {
	m.visTracer = tr
}

// SetTraceDBPath sets the path to the SQLite trace database used to serve
// trace data through the monitoring server.
func (m *Monitor) SetTraceDBPath(path string) {
	m.traceDBPath = path
}

// GetServer returns the underlying Daisen replay server. This can be used to
// access advanced server functionality (e.g., CreateProgressBar) or to pass
// to components that require a *daisen.Server directly.
func (m *Monitor) GetServer() *daisen.Server {
	return m.replayServer
}

// CreateProgressBar creates a new progress bar tracked by the monitor.
func (m *Monitor) CreateProgressBar(name string, total uint64) *daisen.ProgressBar {
	bar := &daisen.ProgressBar{
		ID:    sim.GetIDGenerator().Generate(),
		Name:  name,
		Total: total,
	}

	m.progressBarsLock.Lock()
	defer m.progressBarsLock.Unlock()

	m.progressBars = append(m.progressBars, bar)

	return bar
}

// CompleteProgressBar removes a bar from the progress list.
func (m *Monitor) CompleteProgressBar(pb *daisen.ProgressBar) {
	m.progressBarsLock.Lock()
	defer m.progressBarsLock.Unlock()

	newBars := make([]*daisen.ProgressBar, 0, len(m.progressBars)-1)

	for _, b := range m.progressBars {
		if b != pb {
			newBars = append(newBars, b)
		}
	}

	m.progressBars = newBars
}

// StartServer initializes and starts the monitoring HTTP server. It creates
// a combined HTTP server with live monitoring and trace replay endpoints.
func (m *Monitor) StartServer() {
	// Create replay server for trace endpoints (if traceDBPath set).
	if m.traceDBPath != "" {
		m.replayServer = daisen.NewReplayServerReadOnly(m.traceDBPath)
	}

	// Build combined mux.
	mux := http.NewServeMux()

	// Register live-mode endpoints (these override replay defaults).
	mux.HandleFunc("/api/mode", m.apiMode)
	mux.HandleFunc("/api/pause", m.pauseEngine)
	mux.HandleFunc("/api/continue", m.continueEngine)
	mux.HandleFunc("/api/now", m.now)
	mux.HandleFunc("/api/run", m.run)
	mux.HandleFunc("/api/tick/", m.tick)
	mux.HandleFunc("/api/list_components", m.listComponents)
	mux.HandleFunc("/api/component/", m.listComponentDetails)
	mux.HandleFunc("/api/field/", m.listFieldValue)
	mux.HandleFunc("/api/hangdetector/buffers", m.hangDetectorBuffers)
	mux.HandleFunc("/api/progress", m.listProgressBars)
	mux.HandleFunc("/api/resource", m.listResources)
	mux.HandleFunc("/api/profile", m.collectProfile)
	mux.HandleFunc("/api/trace/start", m.apiTraceStart)
	mux.HandleFunc("/api/trace/end", m.apiTraceEnd)
	mux.HandleFunc("/api/trace/is_tracing", m.apiTraceIsTracing)

	// Register trace/replay endpoints (from daisen.Server), excluding /api/mode.
	if m.replayServer != nil {
		m.replayServer.RegisterTraceRoutes(mux)
	} else {
		// Serve static assets even without trace DB.
		m.setupStaticRoutes(mux)
	}

	// Find port and start listener.
	listener := m.findPort()
	port := listener.Addr().(*net.TCPAddr).Port

	fmt.Fprintf(os.Stderr,
		"Monitoring simulation with http://localhost:%d\n", port)

	m.httpServer = &http.Server{Handler: mux}

	go func() {
		err := m.httpServer.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			log.Panic(err)
		}
	}()
}

func (m *Monitor) setupStaticRoutes(mux *http.ServeMux) {
	fServer := http.FileServer(m.fs)
	mux.HandleFunc("/dashboard", m.serveIndex)
	mux.HandleFunc("/component", m.serveIndex)
	mux.HandleFunc("/task", m.serveIndex)
	mux.Handle("/", fServer)
}

func (m *Monitor) findPort() net.Listener {
	if m.port > 0 {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", m.port))
		if err != nil {
			log.Panicf("failed to listen on port %d: %v", m.port, err)
		}

		return listener
	}

	startPort := 32776
	maxAttempts := 100

	for i := 0; i < maxAttempts; i++ {
		listener, err := net.Listen("tcp",
			fmt.Sprintf(":%d", startPort+i))
		if err == nil {
			return listener
		}
	}

	log.Panic("failed to find available port")

	return nil
}

// StopServer gracefully shuts down the monitoring server.
func (m *Monitor) StopServer() {
	if m.httpServer != nil {
		m.httpServer.Close()
	}
}

// ---- Buffer inspection ----

// bufferState is a minimal interface for buffer inspection by the hang detector.
type bufferState interface {
	Name() string
	Size() int
	Capacity() int
}

// portBufferAdapter wraps a port to expose one of its internal buffers
// (incoming or outgoing) as a bufferState for the hang detector.
type portBufferAdapter struct {
	port      sim.Port
	direction string // "in" or "out"
}

func (a *portBufferAdapter) Name() string {
	return a.port.Name() + "." + a.direction
}

func (a *portBufferAdapter) Size() int {
	if a.direction == "in" {
		return a.port.NumIncoming()
	}

	return a.port.NumOutgoing()
}

func (a *portBufferAdapter) Capacity() int {
	return a.Size()
}

func (m *Monitor) registerBuffers(c sim.Component) {
	m.registerComponentOrPortBuffers(c)

	for _, p := range c.Ports() {
		m.registerComponentOrPortBuffers(p)
		m.registerPortBuffers(p)
	}
}

func (m *Monitor) registerPortBuffers(p sim.Port) {
	m.buffers = append(m.buffers,
		&portBufferAdapter{port: p, direction: "in"},
		&portBufferAdapter{port: p, direction: "out"},
	)
}

func (m *Monitor) registerComponentOrPortBuffers(c any) {
	v := reflect.ValueOf(c).Elem()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := field.Type()
		bufferType := reflect.TypeOf((*bufferState)(nil)).Elem()

		if fieldType.Implements(bufferType) ||
			reflect.PointerTo(fieldType).Implements(bufferType) {
			fieldRef := reflect.NewAt(
				field.Type(),
				unsafe.Pointer(field.UnsafeAddr()),
			).Elem().Interface().(bufferState)
			m.buffers = append(m.buffers, fieldRef)
		}
	}
}

// ---- Live HTTP handlers ----

func (m *Monitor) apiMode(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"mode":"live"}`)
}

func (m *Monitor) serveIndex(w http.ResponseWriter, _ *http.Request) {
	f, err := m.fs.Open("index.html")
	if err != nil {
		log.Panic(err)
	}

	p, err := readAll(f)
	if err != nil {
		log.Panic(err)
	}

	_, err = w.Write(p)
	if err != nil {
		log.Panic(err)
	}
}

func (m *Monitor) pauseEngine(w http.ResponseWriter, _ *http.Request) {
	m.engine.Pause()
	_, err := w.Write(nil)

	if err != nil {
		log.Panic(err)
	}
}

func (m *Monitor) continueEngine(w http.ResponseWriter, _ *http.Request) {
	m.engine.Continue()
	_, err := w.Write(nil)

	if err != nil {
		log.Panic(err)
	}
}

func (m *Monitor) now(w http.ResponseWriter, _ *http.Request) {
	nowTime := m.engine.CurrentTime()
	fmt.Fprintf(w, "{\"now\":%d}", nowTime)
}

func (m *Monitor) run(_ http.ResponseWriter, _ *http.Request) {
	go func() {
		err := m.engine.Run()
		if err != nil {
			panic(err)
		}
	}()
}

func (m *Monitor) listComponents(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprint(w, "[")

	for i, c := range m.components {
		if i > 0 {
			fmt.Fprint(w, ",")
		}

		fmt.Fprintf(w, "\"%s\"", c.Name())
	}

	fmt.Fprint(w, "]")
}

type tickingComponent interface {
	TickLater()
}

func (m *Monitor) tick(w http.ResponseWriter, r *http.Request) {
	compName := strings.TrimPrefix(r.URL.Path, "/api/tick/")

	comp := m.findComponentOr404(w, compName)
	if comp == nil {
		return
	}

	tickingComp, ok := comp.(tickingComponent)
	if !ok {
		w.WriteHeader(405)
		return
	}

	tickingComp.TickLater()
	w.WriteHeader(200)
}

func (m *Monitor) listComponentDetails(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := strings.TrimPrefix(r.URL.Path, "/api/component/")

	component := m.findComponentOr404(w, name)
	if component == nil {
		return
	}

	m.engine.Pause()
	defer m.engine.Continue()

	serializer := goseth.NewSerializer()
	serializer.SetRoot(component)
	serializer.SetMaxDepth(1)

	err := serializer.Serialize(w)
	if err != nil {
		log.Panic(err)
	}
}

type fieldReq struct {
	CompName  string `json:"comp_name,omitempty"`
	FieldName string `json:"field_name,omitempty"`
}

func (m *Monitor) listFieldValue(w http.ResponseWriter, r *http.Request) {
	jsonString := strings.TrimPrefix(r.URL.Path, "/api/field/")
	req := fieldReq{}

	err := json.Unmarshal([]byte(jsonString), &req)
	if err != nil {
		log.Panic(err)
	}

	name := req.CompName
	fields := strings.Split(req.FieldName, ".")

	component := m.findComponentOr404(w, name)
	if component == nil {
		return
	}

	m.engine.Pause()
	defer m.engine.Continue()

	serializer := goseth.NewSerializer()
	serializer.SetRoot(component)
	serializer.SetMaxDepth(1)

	err = serializer.SetEntryPoint(fields)
	if err != nil {
		log.Panic(err)
	}

	err = serializer.Serialize(w)
	if err != nil {
		log.Panic(err)
	}
}

func (m *Monitor) hangDetectorBuffers(
	w http.ResponseWriter,
	r *http.Request,
) {
	sortMethod, limit, offset, err := buffersParseParams(r)
	if err != nil {
		w.WriteHeader(400)
		fmt.Fprintf(w, "Error: %s", err)

		return
	}

	sortedBuffers := m.sortAndSelectBuffers(sortMethod, limit, offset)

	fmt.Fprintf(w, "[")

	for i, b := range sortedBuffers {
		if i > 0 {
			fmt.Fprint(w, ",")
		}

		fmt.Fprintf(w, "{\"buffer\":\"%s\",\"level\":%d,\"cap\":%d}",
			b.Name(), b.Size(), b.Capacity())
	}

	fmt.Fprint(w, "]")
}

func buffersParseParams(
	r *http.Request,
) (sortStr string, limit, offset int, err error) {
	sortMethod := r.URL.Query().Get("sort")
	if sortMethod == "" {
		sortMethod = "percent"
	}

	if sortMethod != "level" && sortMethod != "percent" {
		errStr := fmt.Sprintf(
			"Invalid sort method: %s. "+
				"Allowed values are `level` and `percent`",
			sortMethod,
		)

		return "", 0, 0, errors.New(errStr)
	}

	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = "0"
	}

	limitNumber, err := strconv.Atoi(limitStr)
	if err != nil {
		return sortMethod, 0, 0, err
	}

	offsetStr := r.URL.Query().Get("offset")
	if offsetStr == "" {
		offsetStr = "0"
	}

	offsetNumber, err := strconv.Atoi(offsetStr)
	if err != nil {
		return sortMethod, limitNumber, 0, err
	}

	return sortMethod, limitNumber, offsetNumber, nil
}

func bufferPercent(b bufferState) float64 {
	return float64(b.Size()) / float64(b.Capacity())
}

func (m *Monitor) sortAndSelectBuffers(
	sortMethod string,
	limit, offset int,
) []bufferState {
	sortedBuffers := make([]bufferState, len(m.buffers))
	copy(sortedBuffers, m.buffers)

	if sortMethod == "level" {
		sort.Slice(sortedBuffers, func(i, j int) bool {
			sizeI := sortedBuffers[i].Size()
			sizeJ := sortedBuffers[j].Size()
			percentI := bufferPercent(sortedBuffers[i])
			percentJ := bufferPercent(sortedBuffers[j])

			if sizeI > sizeJ {
				return true
			} else if sizeI < sizeJ {
				return false
			}

			return percentI > percentJ
		})
	} else if sortMethod == "percent" {
		sort.Slice(sortedBuffers, func(i, j int) bool {
			sizeI := sortedBuffers[i].Size()
			sizeJ := sortedBuffers[j].Size()
			percentI := bufferPercent(sortedBuffers[i])
			percentJ := bufferPercent(sortedBuffers[j])

			if percentI > percentJ {
				return true
			} else if percentI < percentJ {
				return false
			}

			return sizeI > sizeJ
		})
	} else {
		panic("Invalid sort method " + sortMethod)
	}

	if offset >= len(sortedBuffers) {
		return []bufferState{}
	}

	if offset+limit > len(sortedBuffers) {
		limit = len(sortedBuffers) - offset
	}

	sortedBuffers = sortedBuffers[offset : offset+limit]

	return sortedBuffers
}

func (m *Monitor) findComponentOr404(
	w http.ResponseWriter,
	name string,
) sim.Component {
	var component sim.Component

	for _, c := range m.components {
		if c.Name() == name {
			component = c
		}
	}

	if component == nil {
		w.WriteHeader(http.StatusNotFound)

		_, err := w.Write([]byte("Component not found"))
		if err != nil {
			log.Panic(err)
		}
	}

	return component
}

func (m *Monitor) listProgressBars(
	w http.ResponseWriter,
	_ *http.Request,
) {
	b, err := json.Marshal(m.progressBars)
	if err != nil {
		log.Panic(err)
	}

	_, err = w.Write(b)
	if err != nil {
		log.Panic(err)
	}
}

type resourceRsp struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemorySize uint64  `json:"memory_size"`
}

func (m *Monitor) listResources(w http.ResponseWriter, _ *http.Request) {
	pid := os.Getpid()

	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		log.Panic(err)
	}

	cpuPercent, err := proc.CPUPercent()
	if err != nil {
		log.Panic(err)
	}

	memorySize, err := proc.MemoryInfo()
	if err != nil {
		log.Panic(err)
	}

	rsp := resourceRsp{
		CPUPercent: cpuPercent,
		MemorySize: memorySize.RSS,
	}

	b, err := json.Marshal(rsp)
	if err != nil {
		log.Panic(err)
	}

	_, err = w.Write(b)
	if err != nil {
		log.Panic(err)
	}
}

func (m *Monitor) collectProfile(w http.ResponseWriter, _ *http.Request) {
	buf := bytes.NewBuffer(nil)

	err := pprof.StartCPUProfile(buf)
	if err != nil {
		log.Panic(err)
	}

	time.Sleep(time.Second)

	pprof.StopCPUProfile()

	prof, err := profile.ParseData(buf.Bytes())
	if err != nil {
		log.Panic(err)
	}

	b, err := json.Marshal(prof)
	if err != nil {
		log.Panic(err)
	}

	_, err = w.Write(b)
	if err != nil {
		log.Panic(err)
	}
}

func (m *Monitor) apiTraceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if m.visTracer == nil {
		fmt.Println("Error: tracer is nil")
		http.Error(w, "tracer is nil", http.StatusInternalServerError)

		return
	}

	m.visTracer.StartTracing()
	w.WriteHeader(200)
	w.Write([]byte(`{"status":"started"}`))
}

func (m *Monitor) apiTraceEnd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if m.visTracer == nil {
		fmt.Println("Error: tracer is nil")
		http.Error(w, "tracer is nil", http.StatusInternalServerError)

		return
	}

	m.visTracer.StopTracing()
	w.WriteHeader(200)
	w.Write([]byte(`{"status":"ended"}`))
}

func (m *Monitor) apiTraceIsTracing(
	w http.ResponseWriter,
	_ *http.Request,
) {
	var isTracing bool
	if m.visTracer != nil {
		isTracing = m.visTracer.IsTracing()
	}

	response := map[string]bool{"isTracing": isTracing}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		http.Error(w, "Internal Server Error",
			http.StatusInternalServerError)
	}
}

// readAll is a helper to read all bytes from an http.File.
func readAll(f http.File) ([]byte, error) {
	var buf bytes.Buffer

	_, err := buf.ReadFrom(f)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
