CREATE TABLE device_license (
  id INT PRIMARY KEY AUTO_INCREMENT,
  serial_number VARCHAR(50) NOT NULL UNIQUE,  -- Plain text: ST-2025-A001-TEST1
  mac_address VARCHAR(20) NOT NULL,            -- MAC address
  status TINYINT DEFAULT 0,                    -- 0=uninitialized, 2=active
  customer_email VARCHAR(255),
  customer_company VARCHAR(255),
  activated_at DATETIME NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
