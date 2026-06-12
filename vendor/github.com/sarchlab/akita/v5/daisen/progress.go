package daisen

import (
	"sync"
	"time"
)

// A ProgressBar is a tracker of the progress.
type ProgressBar struct {
	sync.Mutex

	ID         uint64    `json:"id"`
	Name       string    `json:"name"`
	StartTime  time.Time `json:"start_time"`
	Total      uint64    `json:"total"`
	Finished   uint64    `json:"finished"`
	InProgress uint64    `json:"in_progress"`
}

// IncrementInProgress adds the number of in-progress elements.
func (b *ProgressBar) IncrementInProgress(amount uint64) {
	b.Lock()
	defer b.Unlock()

	b.InProgress += amount
}

// IncrementFinished adds a certain amount to finished elements.
func (b *ProgressBar) IncrementFinished(amount uint64) {
	b.Lock()
	defer b.Unlock()

	b.Finished += amount
}

// MoveInProgressToFinished reduces the number of in progress items by a certain
// amount and increases the finished items by the same amount.
func (b *ProgressBar) MoveInProgressToFinished(amount uint64) {
	b.Lock()
	defer b.Unlock()

	b.InProgress -= amount
	b.Finished += amount
}
