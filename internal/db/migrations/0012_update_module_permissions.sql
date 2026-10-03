-- Migration: Update Module Permissions System
-- Description: Updates module permissions for existing users and sets up new module structure
-- Date: 2025-01-18

-- First, clear old module permissions that no longer exist
DELETE FROM module_permissions
WHERE module_name IN (
    'alerts', 'sla_reports', 'performance_trends',
    'ip_blacklist', 'whois_domain', 'domain_registry',
    'users', 'settings'
);

-- Initialize module permissions for all existing users based on their role
-- This will set default permissions for the new module structure

-- Admin users: Full access to all modules
INSERT INTO module_permissions (user_id, module_name, can_view, can_edit)
SELECT
    u.id,
    m.module_name,
    true as can_view,
    true as can_edit
FROM users u
CROSS JOIN (
    SELECT 'dashboard' as module_name UNION ALL
    SELECT 'targets' UNION ALL
    SELECT 'ip_scanner' UNION ALL
    SELECT 'notifications' UNION ALL
    SELECT 'notification_settings' UNION ALL
    SELECT 'reporting' UNION ALL
    SELECT 'user_management' UNION ALL
    SELECT 'module_permissions' UNION ALL
    SELECT 'backup'
) m
WHERE u.role = 'admin'
ON DUPLICATE KEY UPDATE
    can_view = true,
    can_edit = true;

-- User role: Default access to operational modules
INSERT INTO module_permissions (user_id, module_name, can_view, can_edit)
SELECT
    u.id,
    'dashboard' as module_name,
    true as can_view,
    false as can_edit
FROM users u
WHERE u.role = 'user'
ON DUPLICATE KEY UPDATE
    can_view = true,
    can_edit = false;

INSERT INTO module_permissions (user_id, module_name, can_view, can_edit)
SELECT
    u.id,
    m.module_name,
    true as can_view,
    true as can_edit
FROM users u
CROSS JOIN (
    SELECT 'targets' as module_name UNION ALL
    SELECT 'ip_scanner'
) m
WHERE u.role = 'user'
ON DUPLICATE KEY UPDATE
    can_view = true,
    can_edit = true;

INSERT INTO module_permissions (user_id, module_name, can_view, can_edit)
SELECT
    u.id,
    m.module_name,
    true as can_view,
    false as can_edit
FROM users u
CROSS JOIN (
    SELECT 'notifications' as module_name UNION ALL
    SELECT 'notification_settings' UNION ALL
    SELECT 'reporting'
) m
WHERE u.role = 'user'
ON DUPLICATE KEY UPDATE
    can_view = true,
    can_edit = false;

INSERT INTO module_permissions (user_id, module_name, can_view, can_edit)
SELECT
    u.id,
    m.module_name,
    false as can_view,
    false as can_edit
FROM users u
CROSS JOIN (
    SELECT 'user_management' as module_name UNION ALL
    SELECT 'module_permissions' UNION ALL
    SELECT 'backup'
) m
WHERE u.role = 'user'
ON DUPLICATE KEY UPDATE
    can_view = false,
    can_edit = false;

-- Viewer role: Read-only access to operational modules
INSERT INTO module_permissions (user_id, module_name, can_view, can_edit)
SELECT
    u.id,
    m.module_name,
    true as can_view,
    false as can_edit
FROM users u
CROSS JOIN (
    SELECT 'dashboard' as module_name UNION ALL
    SELECT 'targets' UNION ALL
    SELECT 'ip_scanner' UNION ALL
    SELECT 'notifications' UNION ALL
    SELECT 'notification_settings' UNION ALL
    SELECT 'reporting'
) m
WHERE u.role = 'viewer'
ON DUPLICATE KEY UPDATE
    can_view = true,
    can_edit = false;

INSERT INTO module_permissions (user_id, module_name, can_view, can_edit)
SELECT
    u.id,
    m.module_name,
    false as can_view,
    false as can_edit
FROM users u
CROSS JOIN (
    SELECT 'user_management' as module_name UNION ALL
    SELECT 'module_permissions' UNION ALL
    SELECT 'backup'
) m
WHERE u.role = 'viewer'
ON DUPLICATE KEY UPDATE
    can_view = false,
    can_edit = false;

-- Add indexes for better performance
CREATE INDEX IF NOT EXISTS idx_module_permissions_user_module
ON module_permissions(user_id, module_name);

CREATE INDEX IF NOT EXISTS idx_module_permissions_module
ON module_permissions(module_name);
