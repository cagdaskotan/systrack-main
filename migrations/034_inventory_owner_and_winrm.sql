-- Migration 034: Inventory Owner + WinRM Persistence

ALTER TABLE inventory
  ADD COLUMN owner_json JSON DEFAULT NULL
    COMMENT 'Sorumlu kisi bilgileri (display_name, mail, department, title, sam_account, upn, managed_by_dn)'
    AFTER ad_service_principal_names,
  ADD COLUMN winrm_json JSON DEFAULT NULL
    COMMENT 'WinRM donanim bilgileri (CPU, RAM, disk, GPU, cevre birimleri)'
    AFTER ssh_json;
