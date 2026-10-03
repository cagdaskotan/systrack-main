-- Migration: Module Permissions System
-- Description: Adds module-based permission system for users

-- Create module_permissions table
CREATE TABLE IF NOT EXISTS module_permissions (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    module_name VARCHAR(50) NOT NULL,
    can_view BOOLEAN DEFAULT FALSE,
    can_edit BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE KEY unique_user_module (user_id, module_name)
);

-- Insert default modules
INSERT INTO module_permissions (user_id, module_name, can_view, can_edit) 
SELECT 
    u.id,
    'dashboard',
    CASE WHEN u.role = 'admin' THEN TRUE ELSE TRUE END,
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END
FROM users u
ON DUPLICATE KEY UPDATE can_view = VALUES(can_view), can_edit = VALUES(can_edit);

INSERT INTO module_permissions (user_id, module_name, can_view, can_edit) 
SELECT 
    u.id,
    'targets',
    CASE WHEN u.role = 'admin' THEN TRUE ELSE TRUE END,
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END
FROM users u
ON DUPLICATE KEY UPDATE can_view = VALUES(can_view), can_edit = VALUES(can_edit);

INSERT INTO module_permissions (user_id, module_name, can_view, can_edit) 
SELECT 
    u.id,
    'alerts',
    CASE WHEN u.role = 'admin' THEN TRUE ELSE TRUE END,
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END
FROM users u
ON DUPLICATE KEY UPDATE can_view = VALUES(can_view), can_edit = VALUES(can_edit);

INSERT INTO module_permissions (user_id, module_name, can_view, can_edit) 
SELECT 
    u.id,
    'sla_reports',
    CASE WHEN u.role = 'admin' THEN TRUE ELSE TRUE END,
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END
FROM users u
ON DUPLICATE KEY UPDATE can_view = VALUES(can_view), can_edit = VALUES(can_edit);

INSERT INTO module_permissions (user_id, module_name, can_view, can_edit) 
SELECT 
    u.id,
    'performance_trends',
    CASE WHEN u.role = 'admin' THEN TRUE ELSE TRUE END,
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END
FROM users u
ON DUPLICATE KEY UPDATE can_view = VALUES(can_view), can_edit = VALUES(can_edit);

INSERT INTO module_permissions (user_id, module_name, can_view, can_edit) 
SELECT 
    u.id,
    'ip_blacklist',
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END,
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END
FROM users u
ON DUPLICATE KEY UPDATE can_view = VALUES(can_view), can_edit = VALUES(can_edit);

INSERT INTO module_permissions (user_id, module_name, can_view, can_edit) 
SELECT 
    u.id,
    'reporting',
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END,
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END
FROM users u
ON DUPLICATE KEY UPDATE can_view = VALUES(can_view), can_edit = VALUES(can_edit);

INSERT INTO module_permissions (user_id, module_name, can_view, can_edit) 
SELECT 
    u.id,
    'users',
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END,
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END
FROM users u
ON DUPLICATE KEY UPDATE can_view = VALUES(can_view), can_edit = VALUES(can_edit);

INSERT INTO module_permissions (user_id, module_name, can_view, can_edit) 
SELECT 
    u.id,
    'notifications',
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END,
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END
FROM users u
ON DUPLICATE KEY UPDATE can_view = VALUES(can_view), can_edit = VALUES(can_edit);

INSERT INTO module_permissions (user_id, module_name, can_view, can_edit) 
SELECT 
    u.id,
    'settings',
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END,
    CASE WHEN u.role = 'admin' THEN TRUE ELSE FALSE END
FROM users u
ON DUPLICATE KEY UPDATE can_view = VALUES(can_view), can_edit = VALUES(can_edit);
