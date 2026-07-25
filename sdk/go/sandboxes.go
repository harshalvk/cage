package cageclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// CreateSandbox creates a new sandbox. pass an empty CreateSandboxOptions{}
// to use the server's default template
func (c *Client) CreateSandbox(ctx context.Context, opts CreateSandboxOptions) (*Sandbox, error) {
	var sb Sandbox
	if err := c.do(ctx, http.MethodPost, "/sandboxes", opts, &sb); err != nil {
		return nil, err
	}
	return &sb, nil
}

func (c *Client) GetSandbox(ctx context.Context, id string) (*Sandbox, error) {
	var sb Sandbox
	if err := c.do(ctx, http.MethodGet, "/sandboxes"+id, nil, &sb); err != nil {
		return nil, err
	}
	return &sb, nil
}

func (c *Client) ListSandboxes(ctx context.Context) ([]*Sandbox, error) {
	var sandboxes []*Sandbox
	if err := c.do(ctx, http.MethodGet, "/sandboxes", nil, &sandboxes); err != nil {
		return nil, err
	}
	return sandboxes, nil
}

func (c *Client) DeleteSandbox(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/sandboxes/"+id, nil, nil)
}

func (c *Client) PauseSandbox(ctx context.Context, id string) (*Sandbox, error) {
	var sb Sandbox
	if err := c.do(ctx, http.MethodPost, "/sandboxes/"+id+"/pause", nil, &sb); err != nil {
		return nil, err
	}
	return &sb, nil
}

func (c *Client) ResumeSandbox(ctx context.Context, id string) (*Sandbox, error) {
	var sb Sandbox
	if err := c.do(ctx, http.MethodPost, "/sandboxes/"+id+"/resume", nil, &sb); err != nil {
		return nil, err
	}
	return &sb, nil
}

func (c *Client) Exec(ctx context.Context, sandboxID string, cmd []string) (*ExecResult, error) {
	var result ExecResult
	body := map[string]any{"cmd": cmd}
	if err := c.do(ctx, http.MethodPost, "/sandboxes/"+sandboxID+"/exec", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) WriteFile(ctx context.Context, sandboxID, path, content string) error {
	body := map[string]any{"path": path, "content": content}
	return c.do(ctx, http.MethodPost, "/sandboxes/"+sandboxID+"/files", body, nil)
}

func (c *Client) ReadFile(ctx context.Context, sandboxID, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/sandboxes/"+sandboxID+"/files?path="+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			fmt.Printf("cage sdk: failed to close response body: %v\n", cerr)
		}
	}()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	return io.ReadAll(resp.Body)
}

func (c *Client) ListTemplates(ctx context.Context) ([]*Template, error) {
	var templates []*Template
	if err := c.do(ctx, http.MethodGet, "/templates", nil, &templates); err != nil {
		return nil, err
	}
	return templates, nil
}
