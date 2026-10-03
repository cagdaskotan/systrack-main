-- Create reports table for reporting module
CREATE TABLE IF NOT EXISTS reports (
    id INT AUTO_INCREMENT PRIMARY KEY,
    type VARCHAR(50) NOT NULL,
    title VARCHAR(255) NOT NULL,
    format VARCHAR(20) NOT NULL,
    status ENUM('generating', 'completed', 'failed') DEFAULT 'generating',
    url VARCHAR(500),
    config JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

-- Create report_templates table for custom report templates
CREATE TABLE IF NOT EXISTS report_templates (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    description TEXT,
    config JSON NOT NULL,
    is_default BOOLEAN DEFAULT FALSE,
    created_by INT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

-- Insert default report templates
INSERT INTO report_templates (name, type, description, config, is_default) VALUES
('SLA Executive Summary', 'sla', 'High-level SLA overview for executives', 
 '{"sections": ["summary", "trends", "breaches"], "charts": ["uptime_trend", "compliance_status"], "period": "30d"}', TRUE),
('Performance Technical Report', 'performance', 'Detailed performance analysis for technical teams',
 '{"sections": ["response_times", "distribution", "comparison", "alerts"], "charts": ["response_trend", "performance_dist", "target_comparison"], "period": "7d"}', TRUE),
('IP Blacklist Security Report', 'ip_blacklist', 'Security-focused IP blacklist analysis',
 '{"sections": ["summary", "high_risk", "geographic", "reports"], "charts": ["risk_distribution", "country_map", "timeline"], "period": "30d"}', TRUE),
('Comprehensive System Report', 'custom', 'Complete system overview with all metrics',
 '{"sections": ["sla", "performance", "security", "alerts"], "charts": ["all"], "period": "30d"}', TRUE);
