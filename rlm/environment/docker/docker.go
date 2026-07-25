package docker

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1azar/cogito/rlm/environment"
)

type Config struct {
	Image      string
	DockerPath string
	CPUs       float64
	MemoryMB   int
	PIDs       int
}

// Factory that can create new python sessions
type Factory struct {
	config Config
}

func New(config Config) *Factory {
	if config.Image == "" {
		config.Image = "python:3.12-slim"
	}
	if config.DockerPath == "" {
		config.DockerPath = "docker"
	}
	if config.CPUs == 0 {
		config.CPUs = 1
	}
	if config.MemoryMB == 0 {
		config.MemoryMB = 256
	}
	if config.PIDs == 0 {
		config.PIDs = 64
	}
	return &Factory{config: config}
}

// NewSession starts safe python sessions in docker
func (f *Factory) NewSession(ctx context.Context, config environment.SessionConfig) (environment.Session, error) {
	// docker args
	args := []string{
		"run", "--rm", "-i",
		"--network", "none",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=16m",
		"--user", "65534:65534",
		"--cpus", strconv.FormatFloat(f.config.CPUs, 'f', -1, 64),
		"--memory", strconv.Itoa(f.config.MemoryMB) + "m",
		"--pids-limit", strconv.Itoa(f.config.PIDs),
		f.config.Image, "python", "-u", "-c", pythonRuntime,
	}
	// build command
	cmd := exec.CommandContext(ctx, f.config.DockerPath, args...)
	// for commands from go
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	// for outputs from python
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	// for python errors runtime/container
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	// start python repl
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	session := &session{
		cmd:            cmd,
		stdin:          stdin,
		stdout:         bufio.NewReader(stdout),
		codeTimeout:    config.CodeTimeout,
		maxOutputBytes: config.MaxOutputBytes,
		done:           make(chan struct{}),
	}
	// listen stderr separately if docker/runtime fails
	go session.captureStderr(stderr)
	// send initial frame to python (go: are you ready?)
	if err := session.writeFrame(initFrame{
		Type:           "init",
		Context:        config.Context,
		MaxOutputBytes: config.MaxOutputBytes,
	}); err != nil {
		_ = session.Close(context.Background())
		return nil, err
	}
	// read response from python (write_frame({"type": "ready"}))
	var ready protocolFrame
	if err := session.readFrame(&ready); err != nil {
		_ = session.Close(context.Background())
		return nil, fmt.Errorf("initialize python REPL: %w", err)
	}
	if ready.Type != "ready" {
		_ = session.Close(context.Background())
		return nil, fmt.Errorf("initialize python REPL: unexpected frame %q", ready.Type)
	}
	// session is ready
	return session, nil
}

type session struct {
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	stdout         *bufio.Reader
	codeTimeout    time.Duration
	maxOutputBytes int
	mu             sync.Mutex
	closeOnce      sync.Once
	done           chan struct{}
	stderrMu       sync.Mutex
	stderr         strings.Builder
}

type initFrame struct {
	Type           string          `json:"type"`
	Context        json.RawMessage `json:"context"`
	MaxOutputBytes int             `json:"max_output_bytes"`
}

type executeFrame struct {
	Type string `json:"type"`
	Code string `json:"code"`
}

type protocolFrame struct {
	Type      string                  `json:"type"`
	Request   environment.CallRequest `json:"request"`
	Stdout    string                  `json:"stdout"`
	Stderr    string                  `json:"stderr"`
	Answer    *environment.Answer     `json:"answer"`
	Truncated bool                    `json:"truncated"`
	Error     string                  `json:"error"`
}

func (s *session) Execute(ctx context.Context, code string, handler environment.CallHandler) (environment.ExecutionResult, error) {
	// one code for single session at a time
	s.mu.Lock()
	defer s.mu.Unlock()
	started := time.Now()
	// prevent `while True: pass`
	if s.codeTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.codeTimeout)
		defer cancel()
	}
	// send python code
	if err := s.writeFrame(executeFrame{Type: "execute", Code: code}); err != nil {
		return environment.ExecutionResult{}, err
	}

	// start reading result
	type readResult struct {
		frame protocolFrame
		err   error
	}
	frames := make(chan readResult, 1)
	readNext := func() {
		var frame protocolFrame
		err := s.readFrame(&frame)
		frames <- readResult{frame: frame, err: err}
	}
	go readNext()
	for {
		select {
		case <-ctx.Done():
			_ = s.terminate()
			return environment.ExecutionResult{}, fmt.Errorf("%w: %v", environment.ErrCodeTimeout, ctx.Err())
		case item := <-frames:
			if item.err != nil {
				return environment.ExecutionResult{}, s.withContainerStderr(item.err)
			}
			switch item.frame.Type {
			case "call":
				result := handler.HandleCall(ctx, item.frame.Request)
				if err := s.writeFrame(struct {
					Type   string                 `json:"type"`
					Result environment.CallResult `json:"result"`
				}{Type: "call_result", Result: result}); err != nil {
					return environment.ExecutionResult{}, err
				}
				go readNext()
			case "done":
				result := environment.ExecutionResult{
					Stdout: item.frame.Stdout, Stderr: item.frame.Stderr, Answer: item.frame.Answer,
					Truncated: item.frame.Truncated, Duration: time.Since(started),
				}
				if item.frame.Error != "" {
					return result, errors.New(item.frame.Error)
				}
				return result, nil
			default:
				return environment.ExecutionResult{}, fmt.Errorf("unexpected python frame %q", item.frame.Type)
			}
		}
	}
}

func (s *session) Close(ctx context.Context) error {
	var closeErr error
	s.closeOnce.Do(func() {
		_ = s.writeFrame(struct {
			Type string `json:"type"`
		}{Type: "close"})
		_ = s.stdin.Close()
		wait := make(chan error, 1)
		go func() {
			wait <- s.cmd.Wait()
			close(s.done)
		}()
		select {
		case err := <-wait:
			if err != nil {
				closeErr = s.withContainerStderr(err)
			}
		case <-ctx.Done():
			closeErr = ctx.Err()
			_ = s.terminate()
		case <-time.After(2 * time.Second):
			_ = s.terminate()
		}
	})
	return closeErr
}

func (s *session) terminate() error {
	if s.cmd.Process == nil {
		return nil
	}
	return s.cmd.Process.Kill()
}

func (s *session) writeFrame(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(payload) > 64<<20 {
		return errors.New("protocol frame exceeds 64 MiB")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := s.stdin.Write(header[:]); err != nil {
		return err
	}
	_, err = s.stdin.Write(payload)
	return err
}

func (s *session) readFrame(value any) error {
	var header [4]byte
	if _, err := io.ReadFull(s.stdout, header[:]); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size > 64<<20 {
		return errors.New("protocol frame exceeds 64 MiB")
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(s.stdout, payload); err != nil {
		return err
	}
	return json.Unmarshal(payload, value)
}

func (s *session) captureStderr(reader io.Reader) {
	data, _ := io.ReadAll(io.LimitReader(reader, 64<<10))
	s.stderrMu.Lock()
	s.stderr.Write(data)
	s.stderrMu.Unlock()
}

func (s *session) withContainerStderr(err error) error {
	s.stderrMu.Lock()
	defer s.stderrMu.Unlock()
	if s.stderr.Len() == 0 {
		return err
	}
	return fmt.Errorf("%w: %s", err, strings.TrimSpace(s.stderr.String()))
}

var _ environment.Factory = (*Factory)(nil)
var _ environment.Session = (*session)(nil)

// pythonRuntime is a python server
const pythonRuntime = `
import contextlib
import io
import json
import struct
import sys
import traceback

inp = sys.stdin.buffer
out = sys.stdout.buffer

def read_frame():
    header = inp.read(4)
    if not header:
        return None
    if len(header) != 4:
        raise EOFError("short frame header")
    size = struct.unpack(">I", header)[0]
    payload = inp.read(size)
    if len(payload) != size:
        raise EOFError("short frame payload")
    return json.loads(payload)

def write_frame(value):
    payload = json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode()
    out.write(struct.pack(">I", len(payload)))
    out.write(payload)
    out.flush()

initial = read_frame()
if initial is None or initial.get("type") != "init":
    raise RuntimeError("expected init frame")
context = initial.get("context")
max_output_bytes = int(initial.get("max_output_bytes") or 20480)
current_answer = None

def host_call(kind, prompts, batched):
    if isinstance(prompts, str):
        prompts = [prompts]
    if not isinstance(prompts, list) or not all(isinstance(x, str) for x in prompts):
        raise TypeError("prompt must be a string or list of strings")
    write_frame({"type": "call", "request": {"kind": kind, "prompts": prompts, "batched": batched}})
    response = read_frame()
    if response is None or response.get("type") != "call_result":
        raise RuntimeError("expected call_result frame")
    result = response.get("result") or {}
    if result.get("error"):
        raise RuntimeError(result["error"])
    values = result.get("results") or []
    return values if batched else (values[0] if values else "")

def llm_query(prompt):
    return host_call("llm", prompt, False)

def llm_query_batched(prompts):
    return host_call("llm", prompts, True)

def rlm_query(prompt):
    return host_call("rlm", prompt, False)

def rlm_query_batched(prompts):
    return host_call("rlm", prompts, True)

def answer(value="", tool_calls=None):
    global current_answer
    if isinstance(value, dict):
        current_answer = {
            "text": str(value.get("text") or ""),
            "tool_calls": value.get("tool_calls"),
        }
    else:
        current_answer = {"text": str(value), "tool_calls": tool_calls}

namespace = {
    "context": context,
    "llm_query": llm_query,
    "llm_query_batched": llm_query_batched,
    "rlm_query": rlm_query,
    "rlm_query_batched": rlm_query_batched,
    "answer": answer,
}
write_frame({"type": "ready"})

while True:
    frame = read_frame()
    if frame is None or frame.get("type") == "close":
        break
    if frame.get("type") != "execute":
        write_frame({"type": "done", "error": "expected execute frame"})
        continue
    stdout_buffer = io.StringIO()
    stderr_buffer = io.StringIO()
    current_answer = None
    error = ""
    try:
        with contextlib.redirect_stdout(stdout_buffer), contextlib.redirect_stderr(stderr_buffer):
            exec(frame.get("code") or "", namespace, namespace)
    except BaseException:
        error = traceback.format_exc()
    stdout_value = stdout_buffer.getvalue()
    stderr_value = stderr_buffer.getvalue()
    combined = (stdout_value + stderr_value).encode()
    truncated = len(combined) > max_output_bytes
    if truncated:
        remaining = max_output_bytes
        stdout_bytes = stdout_value.encode()[:remaining]
        remaining -= len(stdout_bytes)
        stderr_bytes = stderr_value.encode()[:max(remaining, 0)]
        stdout_value = stdout_bytes.decode(errors="replace")
        stderr_value = stderr_bytes.decode(errors="replace")
    write_frame({
        "type": "done",
        "stdout": stdout_value,
        "stderr": stderr_value,
        "answer": current_answer,
        "truncated": truncated,
        "error": error,
    })
`
