ALTER TABLE templates ADD COLUMN firecracker_rootfs_slug TEXT;

-- Backfill the two templates we already know are Firecracker-compatible,
-- matching the "base" rootfs built in Step F2. Extend this as more
-- templates gain Firecracker rootfs images.
UPDATE templates SET firecracker_rootfs_slug = 'base' WHERE slug = 'base';