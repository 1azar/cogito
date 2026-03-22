package runtime

import (
	"sync"
	"time"
)

type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunRunning   RunStatus = "running"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
	RunCanceled  RunStatus = "canceled"
)

type Run struct {
	id        string
	sessionID string
	input     string

	status    RunStatus
	startedAt time.Time
	endedAt   time.Time
	err       error

	cancel func()
	mu     sync.RWMutex
}

type RunSnapshot struct {
	ID        string
	SessionID string
	Input     string
	Status    RunStatus
	StartedAt time.Time
	EndedAt   time.Time
	Err       error
	Duration  time.Duration
}

func (r *Run) ID() string {
	if r == nil {
		return ""
	}
	return r.id
}

func (r *Run) Cancel() {
	if r == nil {
		return
	}
	r.mu.RLock()
	cancel := r.cancel
	r.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
}

func (r *Run) Snapshot() RunSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	endedAt := r.endedAt
	if endedAt.IsZero() {
		endedAt = time.Now()
	}

	return RunSnapshot{
		ID:        r.id,
		SessionID: r.sessionID,
		Input:     r.input,
		Status:    r.status,
		StartedAt: r.startedAt,
		EndedAt:   r.endedAt,
		Err:       r.err,
		Duration:  endedAt.Sub(r.startedAt),
	}
}

func (r *Run) setStatus(status RunStatus, err error) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status == RunCompleted || r.status == RunFailed || r.status == RunCanceled {
		return
	}

	r.status = status
	r.err = err
	r.endedAt = time.Now()
}
