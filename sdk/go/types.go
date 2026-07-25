package cageclient

import "time"

type SandboxStatus string

const (
	StatusRunning SandboxStatus = "running"
	StatusPaused  SandboxStatus = "paused"
	StatusStopped SandboxStatus = "stopped"
)

type Sandbox struct {
	ID           string        `json:"id"`
	Status       SandboxStatus `json:"status"`
	CreatedAt    time.Time     `json:"created_at"`
	ExpiresAt    time.Time     `json:"expires_at"`
	TemplateSlug string        `json:"template_slug"`
}

type Template struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Image       string `json:"image"`
	Description string `json:"description"`
}

type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

type CreateSandboxOptions struct {
	Template string `json:"template,omitempty"`
}
