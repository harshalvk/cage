package store

import (
	"context"
	"fmt"
)

func (s *Store) ListExpired(ctx context.Context) ([]*Sandbox, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, container_id, status, created_at, expires_at, template_slug, paused_image_id
		 FROM sandboxes
		 WHERE expires_at < now() AND status IN ('running', 'paused')`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list expired sandboxes: %w", err)
	}
	defer rows.Close()

	sandboxes := []*Sandbox{}
	for rows.Next() {
		var sb Sandbox
		if err := rows.Scan(&sb.ID, &sb.ContainerID, &sb.Status, &sb.CreatedAt, &sb.ExpiresAt, &sb.TemplateSlug, &sb.PausedImageID); err != nil {
			return nil, fmt.Errorf("failed to scan sandbox: %w", err)
		}
		sandboxes = append(sandboxes, &sb)
	}
	return sandboxes, nil
}
