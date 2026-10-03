#Requires -RunAsAdministrator
# SysTrack WinRM Kurulum Scripti
# Tek seferlik calistirilir. Mevcut WinRM yapilandirmasina dokunmadan
# yalnizca gerekli ayarlari acar.

$ErrorActionPreference = "Stop"
$host.UI.RawUI.WindowTitle = "SysTrack WinRM Kurulum"

Write-Host ""
Write-Host "=== SysTrack WinRM Kurulum Scripti ===" -ForegroundColor Cyan
Write-Host ""

# 1. WinRM servisini baslat
Write-Host "[1/5] WinRM servisi baslatiliyor..." -ForegroundColor Yellow
try {
    Set-Service -Name WinRM -StartupType Automatic
    Start-Service -Name WinRM -ErrorAction SilentlyContinue
    # Servis baslayinca kisa bekleme
    Start-Sleep -Milliseconds 500
    Write-Host "      OK - WinRM servisi calisiyor." -ForegroundColor Green
} catch {
    Write-Host "      HATA: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

# 2. Listener kontrol / olustur
Write-Host "[2/5] WinRM HTTP listener ayarlaniyor (port 5985)..." -ForegroundColor Yellow
try {
    # Mevcut listener'i kontrol et
    $listeners = & winrm enumerate winrm/config/listener 2>$null
    if ($listeners -match "Transport = HTTP") {
        Write-Host "      OK - HTTP listener zaten mevcut." -ForegroundColor Green
    } else {
        & winrm create winrm/config/listener?Address=*+Transport=HTTP | Out-Null
        Write-Host "      OK - HTTP listener olusturuldu." -ForegroundColor Green
    }
    # Buyuk yazilim listesi icin envelope boyutunu artir
    & winrm set winrm/config "@{MaxEnvelopeSizekb=`"8192`"}" | Out-Null
} catch {
    Write-Host "      UYARI: $($_.Exception.Message)" -ForegroundColor DarkYellow
}

# 3. Kimlik dogrulama - winrm CLI kullan (WSMan PS drive bazen yok)
Write-Host "[3/5] Kimlik dogrulama ayarlaniyor..." -ForegroundColor Yellow
try {
    & winrm set winrm/config/service/auth "@{Basic=`"true`"}"          | Out-Null
    & winrm set winrm/config/service/auth "@{NTLM=`"true`"}"           | Out-Null
    & winrm set winrm/config/service       "@{AllowUnencrypted=`"true`"}" | Out-Null
    Write-Host "      OK - Basic + NTLM kimlik dogrulama etkin." -ForegroundColor Green
} catch {
    Write-Host "      HATA: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

# 4. Firewall - port 5985
Write-Host "[4/5] Guvenlik duvari kurali ekleniyor (TCP 5985)..." -ForegroundColor Yellow
try {
    $ruleName = "SysTrack-WinRM-HTTP-5985"
    $existing = Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue
    if (-not $existing) {
        New-NetFirewallRule `
            -DisplayName $ruleName `
            -Direction Inbound `
            -Protocol TCP `
            -LocalPort 5985 `
            -Action Allow `
            -Profile Any `
            -Description "SysTrack envanter taramasi icin WinRM HTTP erisimi" | Out-Null
        Write-Host "      OK - Guvenlik duvari kurali eklendi." -ForegroundColor Green
    } else {
        Write-Host "      OK - Kural zaten mevcut." -ForegroundColor Green
    }
} catch {
    Write-Host "      UYARI: $($_.Exception.Message)" -ForegroundColor DarkYellow
}

# 5. Dogrulama
Write-Host "[5/5] Kurulum dogrulanıyor..." -ForegroundColor Yellow
try {
    $result = Test-WSMan -ComputerName localhost -ErrorAction Stop
    Write-Host "      OK - WinRM yanit veriyor." -ForegroundColor Green
} catch {
    Write-Host "      HATA: WinRM dogrulamasi basarisiz: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

# Ozet
Write-Host ""
Write-Host "=== Kurulum Tamamlandi ===" -ForegroundColor Cyan
Write-Host ""

$ip = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object {
    $_.PrefixOrigin -in @("Manual","Dhcp") -and
    $_.IPAddress -notmatch "^(127\.|169\.254\.)"
} | Sort-Object InterfaceIndex | Select-Object -First 1).IPAddress

Write-Host "Bu makinenin IP adresi : $ip" -ForegroundColor White
Write-Host "WinRM portu            : 5985 (HTTP)" -ForegroundColor White
Write-Host ""
Write-Host "SysTrack panelinde bu cihazi su bilgilerle ekleyin:" -ForegroundColor White
Write-Host "  IP Adresi  : $ip"            -ForegroundColor Cyan
Write-Host "  Kullanici  : $env:USERNAME"  -ForegroundColor Cyan
Write-Host "  Port       : 5985"           -ForegroundColor Cyan
Write-Host "  SSL        : Hayir"          -ForegroundColor Cyan
Write-Host ""
