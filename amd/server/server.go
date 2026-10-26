// Package server defines a server that can receives commands from external
// applications.
package server

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

type server struct {
	mu         sync.Mutex
	simulation *simulation.Simulation
	component  driver.Ticker
	queue      driver.CommandQueue
	driver     *driver.Runtime
	ctx        driver.Context
}

var serverInstance server

// Builder can help building the server instance with parameters.
type Builder struct {
	simulation *simulation.Simulation
	driver     *driver.Runtime
}

// MakeBuilder creates a builder with default configurations.
func MakeBuilder() Builder {
	return Builder{}
}

// WithRuntime sets the driver the the server works with.
func (b Builder) WithRuntime(d *driver.Runtime) Builder {
	b.driver = d
	return b
}

func (b Builder) WithSimulation(s *simulation.Simulation) Builder { b.simulation = s; return b }

// Build creates the server instance. This function should be called after
// all the configuration is completed, and before the server start to listen
// to a port.
func (b Builder) Build() {
	if b.driver == nil || b.simulation == nil {
		panic("server requires runtime and simulation")
	}
	serverInstance = server{driver: b.driver, simulation: b.simulation}
	for _, c := range b.simulation.Components() {
		if c.Name() == "Driver" {
			serverInstance.component = c.(driver.Ticker)
		}
	}
	if serverInstance.component == nil {
		panic("server requires Driver")
	}
	serverInstance.ctx = b.driver.Init()
	b.driver.SelectGPU(serverInstance.ctx, 1)
	serverInstance.queue = b.driver.CreateCommandQueue(serverInstance.ctx)
}
func (s *server) run() error {
	s.component.TickLater()
	if err := s.simulation.Run(); err != nil {
		return err
	}
	for i, n := range s.driver.Unfinished() {
		if n != 0 {
			return fmt.Errorf("queue %d stalled with %d commands", i+1, n)
		}
	}
	return nil
}

// RegisterHandlers registers all the handlers of the MGPUSim server
func RegisterHandlers() {
	r := mux.NewRouter()
	r.HandleFunc("/device_count", handleDeviceCount)
	r.HandleFunc("/device_properties/{id:[0-9]+}", handleDeviceProperties)
	r.HandleFunc("/malloc", handleMalloc)
	r.HandleFunc("/memcopy_h2d", handleMemcopyH2D)
	r.HandleFunc("/memcopy_d2h", handleMemcopyD2H)
	r.HandleFunc("/launch_kernel", handleLaunchKernel)
	http.Handle("/", r)
}
