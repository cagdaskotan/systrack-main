# Device Service Monitoring - Implementation Guide

## Overview

SysTrack now supports **agent-less device service monitoring** using WinRM (Windows) and SSH (Linux). This allows you to monitor services running inside devices without installing any agents.

## Features

- ✅ **Windows Service Monitoring** via WinRM (PowerShell Remoting)
- ✅ **Linux Service Monitoring** via SSH (systemd/systemctl)
- ✅ **Automatic Service Discovery** - Lists all services on the device
- ✅ **Status Tracking** - Running, Stopped, Unknown
- ✅ **10-Minute Background Polling** - Automatic updates every 10 minutes
- ✅ **AES-256 Encryption** - Credentials stored securely
- ✅ **Manual Scan On-Demand** - Trigger immediate service check
- ✅ **Critical Service Marking** - Mark important services for alerts (v2)
- ✅ **Service Status History** - Track status changes over time

## Database Schema

### New Tables

#### `target_credentials`
Stores encrypted credentials for accessing devices.

```sql
CREATE TABLE target_credentials (
  id INT AUTO_INCREMENT PRIMARY KEY,
  target_id INT NOT NULL UNIQUE,
  os_type ENUM('windows', 'linux') NOT NULL,
  protocol ENUM('winrm', 'ssh') NOT NULL,
  username VARCHAR(255) NOT NULL,
  password_encrypted TEXT NOT NULL,
  port INT DEFAULT NULL,
  domain VARCHAR(255) DEFAULT NULL,
  last_test_at DATETIME DEFAULT NULL,
  last_test_success BOOLEAN DEFAULT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE
);
```

#### `device_services`
Stores discovered services and their current status.

```sql
CREATE TABLE device_services (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  target_id INT NOT NULL,
  service_name VARCHAR(255) NOT NULL,
  display_name VARCHAR(500) DEFAULT NULL,
  description TEXT DEFAULT NULL,
  status ENUM('running', 'stopped', 'unknown') NOT NULL DEFAULT 'unknown',
  startup_type VARCHAR(50) DEFAULT NULL,
  pid INT DEFAULT NULL,
  last_checked DATETIME NOT NULL,
  is_monitored BOOLEAN DEFAULT FALSE,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
  UNIQUE KEY unique_service_per_target (target_id, service_name)
);
```

#### `device_service_history`
Tracks service status changes over time.

```sql
CREATE TABLE device_service_history (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  service_id BIGINT NOT NULL,
  target_id INT NOT NULL,
  service_name VARCHAR(255) NOT NULL,
  old_status ENUM('running', 'stopped', 'unknown') DEFAULT NULL,
  new_status ENUM('running', 'stopped', 'unknown') NOT NULL,
  changed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (service_id) REFERENCES device_services(id) ON DELETE CASCADE,
  FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE
);
```

## API Endpoints

### Credentials Management

- `GET /api/targets/:id/credentials` - Get credentials (without password)
- `POST /api/targets/:id/credentials` - Save/update credentials
- `POST /api/targets/:id/credentials/test` - Test connection
- `DELETE /api/targets/:id/credentials` - Delete credentials

### Service Management

- `GET /api/targets/:id/device-services` - List all services
  - Query params: `?status=running` `?monitored=true`
- `POST /api/targets/:id/device-services/scan` - Trigger immediate scan
- `PATCH /api/targets/:id/device-services/:serviceId` - Update monitoring status
- `GET /api/targets/:id/device-services/:serviceId/history` - Get status history

## User Workflow

### Step 1: Create User on Target Device

**Windows:**
```powershell
# Create a local user for SysTrack
$password = ConvertTo-SecureString "YourSecurePassword" -AsPlainText -Force
New-LocalUser "systrack" -Password $password -Description "SysTrack monitoring user"

# Enable WinRM (if not already enabled)
Enable-PSRemoting -Force

# Add user to Remote Management Users group (read-only)
Add-LocalGroupMember -Group "Remote Management Users" -Member "systrack"
```

**Linux:**
```bash
# Create a user for SysTrack
sudo useradd -m -s /bin/bash systrack

# Set password
sudo passwd systrack

# Allow passwordless sudo for systemctl (optional, for read-only)
sudo visudo
# Add: systrack ALL=(ALL) NOPASSWD: /bin/systemctl status *, /bin/systemctl is-active *
```

### Step 2: Add Credentials in SysTrack

1. Go to target details page
2. Click "Services" button
3. Fill in the credential form:
   - OS Type: Windows / Linux
   - Username: `systrack`
   - Password: `YourSecurePassword`
   - Domain: (optional, for Windows domain users)
   - Port: (optional, default: WinRM=5985, SSH=22)
4. Click "Test Connection" to verify
5. Click "Save"

### Step 3: Scan Services

1. Click "Scan Services Now" to trigger immediate scan
2. All services will be listed automatically
3. Services auto-update every 10 minutes in the background

### Step 4: Mark Critical Services (Optional)

1. Find important services (e.g., nginx, mysql, docker)
2. Click "Monitor" to mark as critical
3. (v2: Alerts will be triggered when critical services stop)

## Security Considerations

### Encryption
- All passwords are encrypted using **AES-256-GCM**
- Encryption key can be set via `AES_ENCRYPTION_KEY` environment variable
- Default key is provided but **should be changed in production**

### Recommended: Use Dedicated Account
- Create a separate user for SysTrack monitoring
- Grant **read-only** permissions
- Use **principle of least privilege**
- Rotate credentials periodically

### Windows Security
- User should be in "Remote Management Users" group
- Do NOT add to Administrators group
- WinRM uses port 5985 (HTTP) or 5986 (HTTPS)
- Consider using HTTPS for WinRM in production

### Linux Security
- Use SSH key authentication instead of password (future improvement)
- Restrict sudo permissions to only `systemctl status` and `systemctl is-active`
- Use firewall rules to limit SSH access

## Environment Variables

```bash
# Set custom encryption key (32 bytes for AES-256)
export AES_ENCRYPTION_KEY="your-32-byte-encryption-key-here"
```

## Migration

Run the migration file:

```bash
mysql -u root -p systrack < migrations/024_device_services.sql
```

This will:
1. Drop old `target_services` and `target_credentials` tables (port-scanning approach)
2. Create new tables for device service monitoring
3. Set up proper indexes and constraints

## Technical Implementation

### Modules

1. **credential_crypto.go** - AES-256 encryption/decryption
2. **windows_service_reader.go** - WinRM service reader
3. **linux_service_reader.go** - SSH service reader
4. **device_service_collector.go** - Background collector (10min polling)
5. **wmi_discovery.go** - Windows WMI/PowerShell integration
6. **ssh_discovery.go** - Linux SSH integration
7. **api_device_services.go** - REST API handlers

### Background Collector

The collector runs every 10 minutes and:
1. Queries all targets with credentials
2. Connects to each device (Windows/Linux)
3. Reads all services using WinRM/SSH
4. Saves service status to database
5. Records status changes in history table

### Windows Implementation

Uses PowerShell Remoting via WMI:
```powershell
Get-WmiObject -Class Win32_Service -ComputerName <IP> -Credential <creds>
```

Returns:
- Service Name
- Display Name
- State (Running/Stopped)
- Start Mode (Auto/Manual/Disabled)
- Process ID
- Description

### Linux Implementation

Uses SSH to run systemctl:
```bash
systemctl list-units --type=service --all --no-pager --no-legend --plain
```

Returns:
- Service Name
- Display Name (description)
- State (active/inactive/failed)
- Sub-state (running/exited/dead)

## Future Enhancements (v2)

- [ ] Alert system for critical service failures
- [ ] Docker container monitoring
- [ ] Custom command execution
- [ ] SSH key authentication (instead of password)
- [ ] Service restart capability
- [ ] Multi-device service grouping
- [ ] Service dependency mapping
- [ ] Performance metrics (CPU, Memory per service)

## Troubleshooting

### Windows WinRM Connection Failed

**Error:** "WMI connection failed"

**Solution:**
1. Ensure WinRM is enabled: `Enable-PSRemoting -Force`
2. Check firewall allows port 5985
3. Verify user is in "Remote Management Users" group
4. Test manually: `Test-WSMan -ComputerName <IP> -Credential (Get-Credential)`

### Linux SSH Connection Failed

**Error:** "SSH connection failed"

**Solution:**
1. Ensure SSH server is running: `systemctl status sshd`
2. Check firewall allows port 22
3. Verify user can login: `ssh systrack@<IP>`
4. Check SSH config allows password authentication

### No Services Found

**Possible causes:**
1. Incorrect credentials
2. User doesn't have permission to read services
3. Network connectivity issues
4. Firewall blocking WinRM/SSH

**Solution:**
1. Click "Test Connection" to verify credentials
2. Check user permissions
3. Test connectivity manually from SysTrack server

## Support

For issues or questions:
- Check logs: `logs/systrack.log`
- Look for: `[Windows Service Reader]` or `[Linux Service Reader]` entries
- Enable debug mode for detailed logging

---

**Implementation Date:** 2025-12-17
**Version:** 1.0.0
**Author:** SysTrack Development Team
