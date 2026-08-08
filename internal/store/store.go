package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SandboxStatus string

const (
	StatusRunning SandboxStatus = "running"
	StatusStopped SandboxStatus = "stopped"
	StatusPaused  SandboxStatus = "paused"
)

type Sandbox struct {
	ID            string        `json:"id"`
	ContainerID   string        `json:"-"`
	Status        SandboxStatus `json:"status"`
	CreatedAt     time.Time     `json:"created_at"`
	ExpiresAt     time.Time     `json:"expires_at"`
	TemplateSlug  string        `json:"template"`
	PausedImageID *string       `json:"-"`
}

type Template struct {
	ID                    string  `json:"id"`
	Slug                  string  `json:"slug"`
	Image                 string  `json:"image"`                   // docker-specific
	FirecrackerRootfsSlug *string `json:"firecracker_rootfs_slug"` // firecracker-specific, nullable
	Description           string  `json:"description"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(ctx context.Context, connString string) (*Store, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Save(ctx context.Context, sb *Sandbox) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sandboxes (id, container_id, status, created_at, expires_at, template_slug, paused_image_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
		container_id = $2, status = $3, created_at = $4, expires_at = $5, template_slug = $6, paused_image_id = $7`,
		sb.ID, sb.ContainerID, sb.Status, sb.CreatedAt, sb.ExpiresAt, sb.TemplateSlug, sb.PausedImageID,
	)
	if err != nil {
		return fmt.Errorf("failed to save sandbox: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, id string) (*Sandbox, error) {
	var sb Sandbox
	err := s.pool.QueryRow(ctx,
		`SELECT id, container_id, status, created_at, expires_at, template_slug, paused_image_id FROM sandboxes WHERE id = $1`,
		id,
	).Scan(&sb.ID, &sb.ContainerID, &sb.Status, &sb.CreatedAt, &sb.ExpiresAt, &sb.TemplateSlug, &sb.PausedImageID)

	if err == pgx.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("faled to get sandboxes: %w", err)
	}

	return &sb, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sandboxes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete sandbox: %w", err)
	}
	return nil
}

func (s *Store) List(ctx context.Context) ([]*Sandbox, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, container_id, status, created_at, expires_at, template_slug FROM sandboxes ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list sandboxes: %w", err)
	}
	defer rows.Close()

	sandboxes := []*Sandbox{} // also fixes the null-vs-[] issue from earlier
	for rows.Next() {
		var sb Sandbox
		if err := rows.Scan(&sb.ID, &sb.ContainerID, &sb.Status, &sb.CreatedAt, &sb.ExpiresAt, &sb.TemplateSlug); err != nil {
			return nil, fmt.Errorf("failed to scan sandbox: %w", err)
		}
		sandboxes = append(sandboxes, &sb)
	}

	return sandboxes, nil
}

func (s *Store) CreateAPIKey(ctx context.Context, name, keyHash string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO api_keys (id, key_hash, name) VALUES (gen_random_uuid(), $1, $2)`,
		keyHash, name,
	)
	if err != nil {
		return fmt.Errorf("failed to create api key: %w", err)
	}

	return nil
}

// this func checks if a hashed key exists and hasn't been revoked
func (s *Store) ValidateAPIKey(ctx context.Context, keyHash string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL
		)`,
		keyHash,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to validate api key: %w", err)
	}
	return exists, nil
}

func (s *Store) GetTemplateBySlug(ctx context.Context, slug string) (*Template, error) {
	var t Template
	err := s.pool.QueryRow(ctx,
		`SELECT id, slug, image, firecracker_rootfs_slug, description FROM templates WHERE slug = $1`,
		slug,
	).Scan(&t.ID, &t.Slug, &t.Image, &t.FirecrackerRootfsSlug, &t.Description)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}
	return &t, nil
}

func (s *Store) ListTemplate(ctx context.Context) ([]*Template, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, slug, image, firecracker_rootfs_slug, description FROM templates ORDER BY slug`)
	if err != nil {
		return nil, fmt.Errorf("failed to list templates: %w", err)
	}
	defer rows.Close()

	templates := []*Template{}
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Slug, &t.Image, &t.FirecrackerRootfsSlug, &t.Description); err != nil {
			return nil, fmt.Errorf("failed to scan template: %w", err)
		}
		templates = append(templates, &t)
	}
	return templates, nil
}

// ResolveRef returns the correct backend-specific reference for this
// template, given the active isolation backend. Returns an error if the
// template has no reference configured for that backend — this is
// deliberately a hard failure, not a silent fallback, since booting a
// Docker image string as a Firecracker rootfs path (or vice versa) fails
// in confusing, hard-to-debug ways rather than a clear error.
func (t *Template) ResolveRef(isolationBackend string) (string, error) {
	switch isolationBackend {
	case "firecracker":
		if t.FirecrackerRootfsSlug == nil || *t.FirecrackerRootfsSlug == "" {
			return "", fmt.Errorf("template %q has no firecracker_rootfs_slug configured", t.Slug)
		}
		return *t.FirecrackerRootfsSlug, nil
	default:
		if t.Image == "" {
			return "", fmt.Errorf("template %q has no docker image configured", t.Slug)
		}
		return t.Image, nil
	}
}
