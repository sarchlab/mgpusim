package driver

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// A page service can outlive the fault response that admitted it. Its task
// and last completed wait are checkpointed alongside the protocol phase.
type pageTrace struct {
	ID   uint64                `json:"id"`
	Last timing.VTimeInPicoSec `json:"last"`
}

func (m *migrationMW) traceStart(parent uint64, reason string) pageTrace {
	t := m.comp.TaskTracker()
	if t == nil {
		return pageTrace{}
	}
	if parent == 0 {
		parent = m.comp.Resources.Runtime.SimulationTask()
	}
	return pageTrace{ID: t.StartTask(m.comp, tasks.StartInfo{
		ParentID: parent, Kind: "Page service", What: reason,
	}), Last: m.now()}
}

func (m *migrationMW) mark(p *migrationPage, kind tasks.MilestoneKind, what string) {
	if p.Trace.ID == 0 || m.now() <= p.Trace.Last {
		return
	}
	m.comp.TaskTracker().AddMilestone(tasks.Milestone{TaskID: p.Trace.ID, Kind: kind, What: what})
	p.Trace.Last = m.now()
}

func (m *migrationMW) tracePage(p *migrationPage) {
	if p.Trace.ID != 0 {
		m.comp.TaskTracker().AddTag(tasks.Tag{TaskID: p.Trace.ID,
			What: fmt.Sprintf("PID %d VA %#x", p.PID, p.VAddr)})
	}
}

func (m *migrationMW) markAck(p *migrationPage) {
	switch p.Phase {
	case pageHold:
		m.mark(p, tasks.MilestoneKindDependency, "address translators held")
	case pagePrepare:
		m.mark(p, tasks.MilestoneKindData, "source and destination frames prepared")
	case pageCopy:
		m.mark(p, tasks.MilestoneKindData, "page copy acknowledged")
	case pageShootdown:
		m.mark(p, tasks.MilestoneKindTranslation, "TLB shootdown acknowledged")
	case pageRelease:
		m.mark(p, tasks.MilestoneKindDependency, "last release acknowledged")
	}
}
