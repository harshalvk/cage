package firecracker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

const guestAgentPort = 52000

// agentRequest/agentResponse mirror the guest agent's protocol exactly —
// keep these in sync with guest-agent/main.go's Request/Response types.
type agentRequest struct {
	Type    string   `json:"type"`
	Cmd     []string `json:"cmd,omitempty"`
	Path    string   `json:"path,omitempty"`
	Content string   `json:"content,omitempty"`
}

type agentResponse struct {
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Content  string `json:"content,omitempty"`
	Error    string `json:"error,omitempty"`
}

// vsockClient talks to one VM's guest agent over Firecracker's host-side
// vsock Unix socket (the .vsock file, distinct from the API socket).
type vsockClient struct {
	udsPath string
	timeout time.Duration
}

func newVsockClient(udsPath string) *vsockClient {
	return &vsockClient{udsPath: udsPath, timeout: 15 * time.Second}
}

// send performs one request/response round trip. Firecracker's vsock proxy
// requires a fresh CONNECT handshake per logical connection, so we dial,
// handshake, send, read, and close on every call — simplest correct
// approach; connection pooling is a possible future optimization.
func (c *vsockClient) send(req agentRequest) (*agentResponse, error) {
	conn, err := net.DialTimeout("unix", c.udsPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to dial vsock uds: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, fmt.Errorf("failed to set vsock deadline: %w", err)
	}

	if _, err := fmt.Fprintf(conn, "CONNECT %d\n", guestAgentPort); err != nil {
		return nil, fmt.Errorf("failed to send vsock connect: %w", err)
	}

	reader := bufio.NewReader(conn)

	// Firecracker replies "OK <port>\n" on a successful proxy handshake.
	handshake, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read vsock handshake: %w", err)
	}
	if !strings.HasPrefix(handshake, "OK") {
		return nil, fmt.Errorf("vsock handshake failed: %s", strings.TrimSpace(handshake))
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal agent request: %w", err)
	}
	if _, err := conn.Write(append(reqBytes, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write agent request: %w", err)
	}

	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read agent response: %w", err)
	}

	var resp agentResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse agent response: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("guest agent error: %s", resp.Error)
	}
	return &resp, nil
}

// waitReady polls until the guest agent responds or the timeout elapses —
// used right after VM start, since there's a real boot delay before the
// agent's systemd service is up and listening.
func (c *vsockClient) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		_, err := c.send(agentRequest{Type: "exec", Cmd: []string{"true"}})
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("guest agent did not become ready within %s: %w", timeout, lastErr)
}
