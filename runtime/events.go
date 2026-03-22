package runtime

import "time"

type EventType string

const (
	EventRunStarted    EventType = "run_started"
	EventRunFinished   EventType = "run_finished"
	EventRunFailed     EventType = "run_failed"
	EventRunCanceled   EventType = "run_canceled"
	EventNodeStarted   EventType = "node_started"
	EventNodeFinished  EventType = "node_finished"
	EventNodeFailed    EventType = "node_failed"
	EventEdgeEvaluated EventType = "edge_evaluated"
)

type Event struct {
	Timestamp time.Time
	Type      EventType

	RunID     string
	SessionID string

	Component string
	NodeID    string
	Step      int

	Duration time.Duration
	Message  string
	Err      error
}
