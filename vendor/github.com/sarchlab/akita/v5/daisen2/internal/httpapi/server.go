// Package httpapi provides a trace visualization server for Akita simulations.
// It serves replay data from a SQLite file.
package httpapi

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/sarchlab/akita/v5/daisen2/static"
)

// Server is the Daisen replay server. It reads trace data from a SQLite file
// and serves it over HTTP for the Daisen dashboard.
type Server struct {
	mode        string
	addr        string
	traceReader *SQLiteTraceReader
	fs          http.FileSystem
	httpServer  *http.Server
}

// NewReplayServer creates a new Server in replay mode. It reads trace data
// from the given SQLite file and serves it on the given address.
func NewReplayServer(sqliteFile, addr string) *Server {
	if sqliteFile == "" {
		panic("must specify a SQLite file")
	}

	reader := NewSQLiteTraceReader(sqliteFile)
	reader.Init()

	return &Server{
		mode:        "replay",
		addr:        addr,
		traceReader: reader,
		fs:          static.GetAssets(),
	}
}

// NewReplayServerReadOnly creates a Server with a read-only SQLite connection.
// Used for concurrent trace access while DBTracer writes.
func NewReplayServerReadOnly(sqliteFile string) *Server {
	if sqliteFile == "" {
		panic("must specify a SQLite file")
	}

	reader := NewSQLiteTraceReader(sqliteFile)
	reader.InitReadOnly()

	return &Server{
		mode:        "replay",
		traceReader: reader,
		fs:          static.GetAssets(),
	}
}

// Start starts the HTTP server in replay mode. It listens on the configured
// address and blocks until the server is shut down.
func (s *Server) Start() {
	mux := s.setupRoutes()
	s.startReplayServer(mux)
}

// StartServer starts the server (alias for Start).
func (s *Server) StartServer() {
	s.Start()
}

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop() {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := s.httpServer.Shutdown(ctx)
		if err != nil {
			log.Printf("Error shutting down server: %v", err)
		}
	}
}

// StopServer stops the server (alias for Stop).
func (s *Server) StopServer() {
	s.Stop()
}

func (s *Server) setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	s.RegisterReplayRoutes(mux)

	return mux
}

// RegisterReplayRoutes registers all replay/trace routes on the provided mux.
// This includes the mode endpoint, trace endpoints, GPT proxy, and static
// assets. Used by the replay server itself.
func (s *Server) RegisterReplayRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/mode", s.apiMode)
	s.RegisterTraceRoutes(mux)
}

// RegisterTraceAPIRoutes registers only the trace/data and assistant API routes
// on the provided mux. It intentionally does not register static SPA routes, so
// callers can serve their own frontend while reusing Daisen's trace APIs.
func (s *Server) RegisterTraceAPIRoutes(mux *http.ServeMux) {
	// Trace endpoints
	mux.HandleFunc("/api/trace", s.httpTrace)
	mux.HandleFunc("/api/trace_range", s.httpTraceTimeRange)
	mux.HandleFunc("/api/compnames", s.httpComponentNames)
	mux.HandleFunc("/api/compinfo", s.httpComponentInfo)
	mux.HandleFunc("/api/segments", s.httpSegments)

	// GPT proxy endpoints
	mux.HandleFunc("/api/gpt", s.httpGPTProxy)
	mux.HandleFunc("/api/githubisavailable", s.httpGithubIsAvailableProxy)
	mux.HandleFunc("/api/checkenv", s.httpCheckEnvFile)
}

// RegisterTraceRoutes registers trace/data routes and Daisen's replay SPA
// routes on the provided mux, excluding the /api/mode endpoint.
func (s *Server) RegisterTraceRoutes(mux *http.ServeMux) {
	s.RegisterTraceAPIRoutes(mux)

	// Static assets / SPA fallback
	fServer := http.FileServer(s.fs)
	mux.HandleFunc("/dashboard", s.serveIndex)
	mux.HandleFunc("/component", s.serveIndex)
	mux.HandleFunc("/task", s.serveIndex)
	mux.Handle("/", fServer)
}

func (s *Server) startReplayServer(mux *http.ServeMux) {
	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	fmt.Printf("Listening %s\n", s.addr)

	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		dieOnErr(err)
	}
}

// ---- Shared handlers ----

func (s *Server) apiMode(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"mode":%q}`, s.mode)
}

func (s *Server) serveIndex(w http.ResponseWriter, _ *http.Request) {
	f, err := s.fs.Open("index.html")
	dieOnErr(err)

	p, err := io.ReadAll(f)
	dieOnErr(err)

	_, err = w.Write(p)
	dieOnErr(err)
}

func dieOnErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}
