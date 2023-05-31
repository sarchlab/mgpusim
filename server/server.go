// Package server defines a server that can receives commands from external
// applications.
package server

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/sarchlab/mgpusim/v3/driver"
)

type server struct {
	driver *driver.Driver
	ctx    *driver.Context
}

var serverInstance server

// Builder can help building the server instance with parameters.
type Builder struct {
	driver *driver.Driver
}

// MakeBuilder creates a builder with default configurations.
func MakeBuilder() Builder {
	return Builder{}
}

// WithDriver sets the driver the the server works with.
func (b Builder) WithDriver(d *driver.Driver) Builder {
	b.driver = d
	return b
}

// Build creates the server instance. This function should be called after
// all the configuration is completed, and before the server start to listen
// to a port.
func (b Builder) Build() {
	serverInstance = server{
		driver: b.driver,
	}

	b.driver.Run()

	serverInstance.ctx = serverInstance.driver.Init()
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
