//go:build integration

package docker

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/1azar/cogito/rlm/environment"
)

func TestSessionPersistenceAndIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	session, err := New(Config{}).NewSession(ctx, environment.SessionConfig{
		Context: []byte(`{"value":41}`), CodeTimeout: 10 * time.Second, MaxOutputBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close(context.Background())
	handler := environment.CallHandlerFunc(func(context.Context, environment.CallRequest) environment.CallResult {
		return environment.CallResult{Results: []string{"ok"}}
	})
	if _, err := session.Execute(ctx, `saved = context["value"] + 1`, handler); err != nil {
		t.Fatal(err)
	}
	result, err := session.Execute(ctx, `print(saved); print(llm_query("x"))`, handler)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Stdout, "42") || !strings.Contains(result.Stdout, "ok") {
		t.Fatalf("stdout = %q", result.Stdout)
	}
	_, err = session.Execute(ctx, `import socket; socket.create_connection(("example.com", 80), timeout=1)`, handler)
	if err == nil {
		t.Fatal("network access unexpectedly succeeded")
	}
}

func TestSessionCodeTimeoutKillsContainer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	session, err := New(Config{}).NewSession(ctx, environment.SessionConfig{
		Context: []byte(`{}`), CodeTimeout: 100 * time.Millisecond, MaxOutputBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close(context.Background())
	_, err = session.Execute(ctx, `while True: pass`, environment.CallHandlerFunc(
		func(context.Context, environment.CallRequest) environment.CallResult { return environment.CallResult{} },
	))
	if err == nil {
		t.Fatal("expected timeout")
	}
}
