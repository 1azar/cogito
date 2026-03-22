package runtime

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type Manager struct {
	mu   sync.RWMutex
	runs map[string]*Run
}

func NewManager() *Manager {
	return &Manager{
		runs: make(map[string]*Run),
	}
}

func (m *Manager) Start(ctx context.Context, sessionID string, input string) (context.Context, *Run) {
	runID := NewRunID("run")
	if ctx == nil {
		ctx = context.Background()
	}

	runCtx, cancel := context.WithCancel(WithRunID(ctx, runID))

	r := &Run{
		id:        runID,
		sessionID: sessionID,
		input:     input,
		status:    RunRunning,
		startedAt: time.Now(),
		cancel:    cancel,
	}

	m.mu.Lock()
	m.runs[runID] = r
	m.mu.Unlock()

	return runCtx, r
}

func (m *Manager) Complete(runID string) error {
	r, ok := m.getRun(runID)
	if !ok {
		return fmt.Errorf("run not found: %s", runID)
	}

	r.setStatus(RunCompleted, nil)
	return nil
}

func (m *Manager) Fail(runID string, err error) error {
	r, ok := m.getRun(runID)
	if !ok {
		return fmt.Errorf("run not found: %s", runID)
	}

	r.setStatus(RunFailed, err)
	return nil
}

func (m *Manager) Cancel(runID string, err error) error {
	r, ok := m.getRun(runID)
	if !ok {
		return fmt.Errorf("run not found: %s", runID)
	}

	r.Cancel()
	r.setStatus(RunCanceled, err)
	return nil
}

func (m *Manager) Get(runID string) (*Run, bool) {
	return m.getRun(runID)
}

func (m *Manager) List() []RunSnapshot {
	m.mu.RLock()
	list := make([]*Run, 0, len(m.runs))
	for _, r := range m.runs {
		list = append(list, r)
	}
	m.mu.RUnlock()

	snapshots := make([]RunSnapshot, 0, len(list))
	for _, r := range list {
		snapshots = append(snapshots, r.Snapshot())
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].StartedAt.Before(snapshots[j].StartedAt)
	})

	return snapshots
}

func (m *Manager) getRun(runID string) (*Run, bool) {
	m.mu.RLock()
	r, ok := m.runs[runID]
	m.mu.RUnlock()
	return r, ok
}
