package tools

import (
	"context"
	"time"

	"github.com/1azar/cogito/tool"
)

type CurrentTimeInput struct {
	Timezone string `json:"timezone,omitempty"` // optional, e.g. "Europe/Amsterdam"
}

type CurrentTimeOutput struct {
	Datetime string `json:"datetime"` // RFC3339 format
}

func currentTime(ctx context.Context, in CurrentTimeInput) (CurrentTimeOutput, error) {
	loc := time.UTC

	if in.Timezone != "" {
		l, err := time.LoadLocation(in.Timezone)
		if err != nil {
			return CurrentTimeOutput{}, err
		}
		loc = l
	}

	now := time.Now().In(loc)

	return CurrentTimeOutput{
		Datetime: now.Format(time.RFC3339),
	}, nil
}

var CurrentTimeTool tool.Tool

func init() {
	var err error

	CurrentTimeTool, err = tool.Func(
		"current_time",
		"Returns the current date and time. Optionally accepts an IANA timezone (e.g. Europe/Amsterdam). Returns datetime in RFC3339 format.",
		currentTime,
	)
	if err != nil {
		panic(err)
	}
}
