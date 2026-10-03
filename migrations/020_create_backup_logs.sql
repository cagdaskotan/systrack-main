-- Backup operation logs
CREATE TABLE IF NOT EXISTS backup_logs (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NULL,
    user_email VARCHAR(255) NULL,
    action ENUM('export','import') NOT NULL,
    status ENUM('success','error') NOT NULL,
    message VARCHAR(500) NULL,
    metadata JSON NULL,
    file_name VARCHAR(255) NULL,
    ip_address VARCHAR(45) NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_backup_logs_created_at (created_at),
    INDEX idx_backup_logs_action (action),
    INDEX idx_backup_logs_user (user_id),
    CONSTRAINT fk_backup_logs_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
