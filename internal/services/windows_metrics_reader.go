package services

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

// WindowsMetricsReader collects metrics from Windows hosts via PowerShell/WMI.
type WindowsMetricsReader struct{}

func NewWindowsMetricsReader() *WindowsMetricsReader {
	return &WindowsMetricsReader{}
}

type WindowsMetricsResult struct {
	CPUPercent    *float64
	CPUCores      *int
	RAMTotalMB    *int64
	RAMUsedMB     *int64
	DiskTotalGB   *int64
	DiskUsedGB    *int64
	UptimeSeconds *int64
}

func (r *WindowsMetricsReader) ReadMetrics(ctx context.Context, address, username, password, domain string) (WindowsMetricsResult, error) {
	var result WindowsMetricsResult

	credential := username
	if domain != "" {
		credential = domain + `\` + username
	}

	escapedPassword := strings.ReplaceAll(password, "'", "''")

	psScript := fmt.Sprintf(`
		$ErrorActionPreference = 'Stop'
		[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
		$OutputEncoding = [System.Text.Encoding]::UTF8
		chcp 65001 | Out-Null

		$secPassword = ConvertTo-SecureString '%s' -AsPlainText -Force
		$credential = New-Object System.Management.Automation.PSCredential('%s', $secPassword)

		function Write-Metrics($cpuPercent, $cpuCores, $ramTotalMb, $ramUsedMb, $diskTotalGb, $diskUsedGb, $uptimeSeconds) {
			Write-Output ("cpu_percent=" + [math]::Round($cpuPercent, 1))
			Write-Output ("cpu_cores=" + $cpuCores)
			Write-Output ("ram_total_mb=" + $ramTotalMb)
			Write-Output ("ram_used_mb=" + $ramUsedMb)
			Write-Output ("disk_total_gb=" + $diskTotalGb)
			Write-Output ("disk_used_gb=" + $diskUsedGb)
			Write-Output ("uptime_seconds=" + $uptimeSeconds)
		}

		try {
			$sessionOption = New-CimSessionOption -Protocol Dcom
			$session = New-CimSession -ComputerName '%s' -Credential $credential -SessionOption $sessionOption

			$cpu = Get-CimInstance -ClassName Win32_Processor -CimSession $session
			$cpuPercent = ($cpu | Measure-Object -Property LoadPercentage -Average).Average
			$cpuCores = ($cpu | Measure-Object -Property NumberOfLogicalProcessors -Sum).Sum

			$os = Get-CimInstance -ClassName Win32_OperatingSystem -CimSession $session
			$ramTotalMb = [int64]([math]::Round($os.TotalVisibleMemorySize / 1024))
			$ramFreeMb = [int64]([math]::Round($os.FreePhysicalMemory / 1024))
			$ramUsedMb = $ramTotalMb - $ramFreeMb

			$disks = Get-CimInstance -ClassName Win32_LogicalDisk -Filter "DriveType=3" -CimSession $session
			$diskTotalGb = [int64]([math]::Round((($disks | Measure-Object -Property Size -Sum).Sum) / 1GB))
			$diskFreeGb = [int64]([math]::Round((($disks | Measure-Object -Property FreeSpace -Sum).Sum) / 1GB))
			$diskUsedGb = $diskTotalGb - $diskFreeGb

			$uptimeSeconds = [int64]([math]::Round((New-TimeSpan -Start $os.LastBootUpTime -End (Get-Date)).TotalSeconds))

			Remove-CimSession $session
			Write-Metrics $cpuPercent $cpuCores $ramTotalMb $ramUsedMb $diskTotalGb $diskUsedGb $uptimeSeconds
			exit 0
		} catch {
		}

		# Fallback to WMI
		$cpu = Get-WmiObject -Class Win32_Processor -ComputerName '%s' -Credential $credential
		$cpuPercent = ($cpu | Measure-Object -Property LoadPercentage -Average).Average
		$cpuCores = ($cpu | Measure-Object -Property NumberOfLogicalProcessors -Sum).Sum

		$os = Get-WmiObject -Class Win32_OperatingSystem -ComputerName '%s' -Credential $credential
		$ramTotalMb = [int64]([math]::Round($os.TotalVisibleMemorySize / 1024))
		$ramFreeMb = [int64]([math]::Round($os.FreePhysicalMemory / 1024))
		$ramUsedMb = $ramTotalMb - $ramFreeMb

		$disks = Get-WmiObject -Class Win32_LogicalDisk -Filter "DriveType=3" -ComputerName '%s' -Credential $credential
		$diskTotalGb = [int64]([math]::Round((($disks | Measure-Object -Property Size -Sum).Sum) / 1GB))
		$diskFreeGb = [int64]([math]::Round((($disks | Measure-Object -Property FreeSpace -Sum).Sum) / 1GB))
		$diskUsedGb = $diskTotalGb - $diskFreeGb

		$uptimeSeconds = [int64]([math]::Round((New-TimeSpan -Start $os.LastBootUpTime -End (Get-Date)).TotalSeconds))

		Write-Metrics $cpuPercent $cpuCores $ramTotalMb $ramUsedMb $diskTotalGb $diskUsedGb $uptimeSeconds
	`, escapedPassword, credential, address, address, address, address)

	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psScript)
	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[WindowsMetrics] powershell failed on %s (user:%s): %v", address, username, err)
		return result, fmt.Errorf("powershell failed: %w", err)
	}

	output := decodeWindowsOutput(rawOutput)
	parsed := parseKeyValueLines(output)
	if len(parsed) == 0 {
		log.Printf("[WindowsMetrics] empty output on %s", address)
	}

	if v, ok := parsed["cpu_percent"]; ok {
		if num, err := strconv.ParseFloat(v, 64); err == nil {
			result.CPUPercent = &num
		}
	}
	if v, ok := parsed["cpu_cores"]; ok {
		if num, err := strconv.Atoi(v); err == nil {
			result.CPUCores = &num
		}
	}
	if v, ok := parsed["ram_total_mb"]; ok {
		if num, err := strconv.ParseInt(v, 10, 64); err == nil {
			result.RAMTotalMB = &num
		}
	}
	if v, ok := parsed["ram_used_mb"]; ok {
		if num, err := strconv.ParseInt(v, 10, 64); err == nil {
			result.RAMUsedMB = &num
		}
	}
	if v, ok := parsed["disk_total_gb"]; ok {
		if num, err := strconv.ParseInt(v, 10, 64); err == nil {
			result.DiskTotalGB = &num
		}
	}
	if v, ok := parsed["disk_used_gb"]; ok {
		if num, err := strconv.ParseInt(v, 10, 64); err == nil {
			result.DiskUsedGB = &num
		}
	}
	if v, ok := parsed["uptime_seconds"]; ok {
		if num, err := strconv.ParseInt(v, 10, 64); err == nil {
			result.UptimeSeconds = &num
		}
	}

	return result, nil
}
