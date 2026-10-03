-- 041_standalone_winrm_hosts.sql
-- Domain dışı Windows cihazlar için bireysel WinRM hedef listesi.
-- Her satır tek bir cihazı temsil eder; keşif sırasında bu cihazlar LDAP'tan bağımsız olarak
-- WinRM üzerinden taranır. Şifreler diğer tablolarla aynı şifreleme yöntemiyle saklanır.
CREATE TABLE IF NOT EXISTS winrm_standalone_hosts (
    id                  INT            NOT NULL AUTO_INCREMENT,
    ip_address          VARCHAR(64)    NOT NULL COMMENT 'Hedef IP veya hostname',
    label               VARCHAR(255)   NULL     COMMENT 'Kullanıcı tanımlı açıklama (opsiyonel)',
    username            VARCHAR(255)   NOT NULL,
    password_encrypted  TEXT           NOT NULL,
    port                INT            NOT NULL DEFAULT 5985,
    use_ssl             TINYINT(1)     NOT NULL DEFAULT 0,
    is_enabled          TINYINT(1)     NOT NULL DEFAULT 1,
    last_seen_at        DATETIME       NULL     COMMENT 'Son başarılı bağlantı zamanı',
    created_at          DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
