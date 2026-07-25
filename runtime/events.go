package runtime

import "time"

type EventType string

const (
	EventRunStarted             EventType = "run_started"
	EventRunFinished            EventType = "run_finished"
	EventRunFailed              EventType = "run_failed"
	EventRunCanceled            EventType = "run_canceled"
	EventNodeStarted            EventType = "node_started"
	EventNodeFinished           EventType = "node_finished"
	EventNodeFailed             EventType = "node_failed"
	EventEdgeEvaluated          EventType = "edge_evaluated"
	EventControllerStepStarted  EventType = "controller_step_started"
	EventControllerStepFinished EventType = "controller_step_finished"
	EventLLMCallStarted         EventType = "llm_call_started"
	EventLLMCallFinished        EventType = "llm_call_finished"
	EventLLMTextDelta           EventType = "llm_text_delta"
	EventToolCallStarted        EventType = "tool_call_started"
	EventToolCallFinished       EventType = "tool_call_finished"
	EventRLMRunStarted          EventType = "rlm_run_started"
	EventRLMRunFinished         EventType = "rlm_run_finished"
	EventRLMIterationStarted    EventType = "rlm_iteration_started"
	EventRLMIterationFinished   EventType = "rlm_iteration_finished"
	EventRLMCodeExecuted        EventType = "rlm_code_executed"
	EventRLMSubcallStarted      EventType = "rlm_subcall_started"
	EventRLMSubcallFinished     EventType = "rlm_subcall_finished"
	EventRLMLimitReached        EventType = "rlm_limit_reached"
)

type Event struct {
	Timestamp time.Time
	Type      EventType

	RunID     string
	SessionID string

	Component  string
	NodeID     string
	Step       int
	ToolName   string
	ToolCallID string
	Attempts   int
	TimedOut   bool

	Duration time.Duration
	Message  string
	Err      error

	InputTokens         int64
	OutputTokens        int64
	TotalTokens         int64
	EstimatedCostMicros int64
}
