package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// ExecutionResult holds the result of a code execution
type ExecutionResult struct {
	Success  bool
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
	Error    error
}

// Executor handles Docker container execution
type Executor struct {
	cli     *client.Client
	timeout time.Duration
}

// NewExecutor creates a new container executor
func NewExecutor(cli *client.Client, timeout time.Duration) *Executor {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Executor{
		cli:     cli,
		timeout: timeout,
	}
}

// Execute runs code in a Docker container with a bind mount to the sandbox directory
func (e *Executor) Execute(ctx context.Context, imageName, sandboxDir, code string, networkEnabled bool, environment map[string]string) ExecutionResult {
	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Convert environment map to Docker format (KEY=value)
	envVars := make([]string, 0, len(environment))
	for key, value := range environment {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}

	// Create container
	containerConfig := &container.Config{
		Image:           imageName,
		WorkingDir:      "/data",
		OpenStdin:       true,
		StdinOnce:       true,
		AttachStdin:     true,
		AttachStdout:    true,
		AttachStderr:    true,
		NetworkDisabled: !networkEnabled, // Network disabled by default for security
		User:            "1000:1000",     // Run as non-root user (must match chown in sandbox manager)
		Env:             envVars,         // Environment variables
	}

	// Bind mount the sandbox directory to /data in the container
	hostConfig := &container.HostConfig{
		Binds: []string{sandboxDir + ":/data"},
		Resources: container.Resources{
			Memory:   256 * 1024 * 1024, // 256MB
			NanoCPUs: 500000000,         // 0.5 CPU
		},
	}

	resp, err := e.cli.ContainerCreate(execCtx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return ExecutionResult{
			Success: false,
			Stderr:  fmt.Sprintf("Failed to create container: %v", err),
			Error:   err,
		}
	}

	containerID := resp.ID
	defer func() {
		// Clean up container
		removeCtx, removeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer removeCancel()
		e.cli.ContainerRemove(removeCtx, containerID, container.RemoveOptions{Force: true})
	}()

	// Attach to container to get stdin/stdout/stderr
	attachResp, err := e.cli.ContainerAttach(execCtx, containerID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return ExecutionResult{
			Success: false,
			Stderr:  fmt.Sprintf("Failed to attach to container: %v", err),
			Error:   err,
		}
	}
	defer attachResp.Close()

	// Start container
	if err := e.cli.ContainerStart(execCtx, containerID, container.StartOptions{}); err != nil {
		return ExecutionResult{
			Success: false,
			Stderr:  fmt.Sprintf("Failed to start container: %v", err),
			Error:   err,
		}
	}

	// Write code to stdin
	go func() {
		io.WriteString(attachResp.Conn, code)
		attachResp.CloseWrite()
	}()

	// Read output - demultiplex stdout and stderr
	var stdoutBuf, stderrBuf bytes.Buffer
	go stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attachResp.Reader)

	// Wait for container to finish
	statusCh, errCh := e.cli.ContainerWait(execCtx, containerID, container.WaitConditionNotRunning)

	var exitCode int64
	var timedOut bool

	select {
	case err := <-errCh:
		if err != nil {
			return ExecutionResult{
				Success: false,
				Stdout:  stdoutBuf.String(),
				Stderr:  fmt.Sprintf("Container wait error: %v\n%s", err, stderrBuf.String()),
				Error:   err,
			}
		}
	case status := <-statusCh:
		exitCode = status.StatusCode
	case <-execCtx.Done():
		timedOut = true
		exitCode = -1
	}

	// Give a moment for output to be fully read
	time.Sleep(100 * time.Millisecond)

	// Get stdout and stderr separately
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if timedOut {
		timeoutMsg := fmt.Sprintf("Execution timed out after %v", e.timeout)
		if stderr != "" {
			stderr = timeoutMsg + "\n" + stderr
		} else {
			stderr = timeoutMsg
		}
	}

	success := exitCode == 0 && !timedOut

	return ExecutionResult{
		Success:  success,
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: int(exitCode),
		TimedOut: timedOut,
	}
}

// PullImage pulls a Docker image if it doesn't exist locally
func (e *Executor) PullImage(ctx context.Context, imageName string) error {
	reader, err := e.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	// Consume the pull output
	io.Copy(io.Discard, reader)
	return nil
}
