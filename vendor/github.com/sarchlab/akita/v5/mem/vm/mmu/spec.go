package mmu

import "github.com/sarchlab/akita/v5/sim"

// Spec contains immutable configuration for the MMU.
type Spec struct {
	Freq                     sim.Freq       `json:"freq"`
	Latency                  int            `json:"latency"`
	MaxRequestsInFlight      int            `json:"max_requests_in_flight"`
	MigrationQueueSize       int            `json:"migration_queue_size"`
	AutoPageAllocation       bool           `json:"auto_page_allocation"`
	Log2PageSize             uint64         `json:"log2_page_size"`
	MigrationServiceProvider sim.RemotePort `json:"migration_service_provider"`
}
