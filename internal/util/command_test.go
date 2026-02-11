package util

import (
	"bytes"
	"context"
	"testing"
)

func TestRun_StreamsToStdoutAndCaptures(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &DefaultCommandRunner{
		stdout: &stdout,
		stderr: &stderr,
	}
	output, err := runner.Run(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(output, []byte("hello")) {
		t.Errorf("expected captured output to contain 'hello', got %q", output)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("hello")) {
		t.Errorf("expected stdout to contain 'hello', got %q", stdout.String())
	}
}

func TestRun_ErrorPropagation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &DefaultCommandRunner{
		stdout: &stdout,
		stderr: &stderr,
	}
	_, err := runner.Run(context.Background(), "false")
	if err == nil {
		t.Fatal("expected error from 'false' command, got nil")
	}
}

func TestRunQuiet_ReturnsFullOutputOnSuccess(t *testing.T) {
	runner := NewCommandRunner()
	output, err := runner.RunQuiet(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(output, []byte("hello")) {
		t.Errorf("expected output to contain 'hello', got %q", output)
	}
}

func TestRunQuiet_ReturnsOutputOnError(t *testing.T) {
	runner := NewCommandRunner()
	output, err := runner.RunQuiet(context.Background(), "sh", "-c", "echo failure-output && exit 1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !bytes.Contains(output, []byte("failure-output")) {
		t.Errorf("expected output to contain 'failure-output', got %q", output)
	}
}
