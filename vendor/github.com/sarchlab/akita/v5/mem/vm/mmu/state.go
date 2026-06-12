package mmu

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

// transactionState is the canonical transaction representation.
type transactionState struct {
	ReqID        uint64         `json:"req_id"`
	RecvTaskID   uint64         `json:"recv_task_id"`
	ReqSrc       sim.RemotePort `json:"req_src"`
	ReqDst       sim.RemotePort `json:"req_dst"`
	PID          uint32         `json:"pid"`
	VAddr        uint64         `json:"vaddr"`
	DeviceID     uint64         `json:"device_id"`
	TransLatency uint64         `json:"trans_latency"`
	Page         vm.Page        `json:"page"`
	CycleLeft    int            `json:"cycle_left"`

	MigrationReqID  uint64         `json:"migration_req_id"`
	MigrationReqSrc sim.RemotePort `json:"migration_req_src"`
	MigrationReqDst sim.RemotePort `json:"migration_req_dst"`
	HasMigration    bool           `json:"has_migration"`
}

// devicePageAccess is a serializable replacement for map[uint64][]uint64,
// since map keys must be strings for state validation.
type devicePageAccess struct {
	PageVAddr uint64   `json:"page_vaddr"`
	DeviceIDs []uint64 `json:"device_ids"`
}

// State contains mutable runtime data for the MMU.
type State struct {
	WalkingTranslations      []transactionState `json:"walking_translations"`
	MigrationQueue           []transactionState `json:"migration_queue"`
	CurrentOnDemandMigration transactionState   `json:"current_on_demand_migration"`
	IsDoingMigration         bool               `json:"is_doing_migration"`
	PageAccessedByDeviceID   []devicePageAccess `json:"page_accessed_by_device_id"`
	NextPhysicalPage         uint64             `json:"next_physical_page"`
	ToRemoveFromPTW          []int              `json:"to_remove_from_ptw"`
}
