package services

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// LinuxMetricsReader collects metrics from Linux hosts via SSH.
type LinuxMetricsReader struct{}

func NewLinuxMetricsReader() *LinuxMetricsReader {
	return &LinuxMetricsReader{}
}

const linuxMetricsScriptVersion = "linux-metrics-v5"

var linuxMetricsLogOnce sync.Once

type LinuxMetricsResult struct {
	CPUPercent    *float64
	CPUCores      *int
	RAMTotalMB    *int64
	RAMUsedMB     *int64
	DiskTotalGB   *int64
	DiskUsedGB    *int64
	UptimeSeconds *int64
	TemperatureC  *float64
}

func (r *LinuxMetricsReader) ReadMetrics(ctx context.Context, address, username, password string, port int) (LinuxMetricsResult, error) {
	var result LinuxMetricsResult

	linuxMetricsLogOnce.Do(func() {
		log.Printf("[LinuxMetrics] script version: %s", linuxMetricsScriptVersion)
	})

	if port == 0 {
		port = 22
	}

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	addr := address
	if !strings.Contains(addr, ":") {
		addr = fmt.Sprintf("%s:%d", address, port)
	}

	dialer := &netDialer{}
	client, err := dialer.DialSSH(ctx, "tcp", addr, config)
	if err != nil {
		log.Printf("[LinuxMetrics] SSH dial failed: %s (user:%s) %v", addr, username, err)
		return result, err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return result, err
	}
	defer session.Close()

	script := strings.Join([]string{
		`echo __STAT1__`,
		`cat /proc/stat 2>/dev/null`,
		`sleep 1`,
		`echo __STAT2__`,
		`cat /proc/stat 2>/dev/null`,
		`echo __MEM__`,
		`cat /proc/meminfo 2>/dev/null`,
		`echo __DF__`,
		`df -kP / 2>/dev/null`,
		`echo __UP__`,
		`cat /proc/uptime 2>/dev/null`,
		`echo __TEMP__`,
		`cat /sys/class/thermal/thermal_zone*/temp 2>/dev/null`,
		`cat /sys/class/hwmon/hwmon*/temp*_input 2>/dev/null`,
		`/opt/vc/bin/vcgencmd measure_temp 2>/dev/null`,
		`vcgencmd measure_temp 2>/dev/null`,
		`sensors 2>/dev/null`,
		`echo __NPROC__`,
		`nproc 2>/dev/null || getconf _NPROCESSORS_ONLN 2>/dev/null || grep -c '^processor' /proc/cpuinfo 2>/dev/null || echo 1`,
	}, " ; ")

	cmd := "sh -lc " + strconv.Quote(script)

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		log.Printf("[LinuxMetrics] command failed on %s: %v", addr, err)
		if len(output) > 0 {
			log.Printf("[LinuxMetrics] command output on %s: %s", addr, truncateOutput(string(output), 400))
		}
	}

	parsed := parseLinuxMetricsOutput(string(output))
	if len(parsed) == 0 {
		log.Printf("[LinuxMetrics] empty output on %s", addr)
		if len(output) > 0 {
			log.Printf("[LinuxMetrics] raw output on %s: %s", addr, truncateOutput(string(output), 800))
		}
		if err != nil {
			return result, fmt.Errorf("ssh command failed: %w", err)
		}
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
	if v, ok := parsed["temperature_c"]; ok {
		if num, err := strconv.ParseFloat(v, 64); err == nil {
			result.TemperatureC = &num
		}
	}

	return result, nil
}

func truncateOutput(value string, max int) string {
	if len(value) <= max {
		return strings.TrimSpace(value)
	}
	start := len(value) - max
	if start < 0 {
		start = 0
	}
	return strings.TrimSpace(value[start:])
}

func parseKeyValueLines(output string) map[string]string {
	lines := strings.Split(output, "\n")
	result := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key != "" {
			result[key] = val
		}
	}
	return result
}

func parseLinuxMetricsOutput(output string) map[string]string {
	result := make(map[string]string)

	sections := splitByMarker(output, []string{"__STAT1__", "__STAT2__", "__MEM__", "__DF__", "__UP__", "__TEMP__", "__NPROC__"})
	if stat1, ok := sections["__STAT1__"]; ok {
		if stat2, ok2 := sections["__STAT2__"]; ok2 {
			if cpu := calcCPUPercent(stat1, stat2); cpu >= 0 {
				result["cpu_percent"] = strconv.Itoa(cpu)
			}
		}
	}
	if mem, ok := sections["__MEM__"]; ok {
		memTotal, memAvail := parseMeminfo(mem)
		if memTotal > 0 {
			result["ram_total_mb"] = strconv.FormatInt(memTotal, 10)
			used := memTotal - memAvail
			if used < 0 {
				used = 0
			}
			result["ram_used_mb"] = strconv.FormatInt(used, 10)
		}
	}
	if df, ok := sections["__DF__"]; ok {
		total, used := parseDfRoot(df)
		if total > 0 {
			result["disk_total_gb"] = strconv.FormatInt(total, 10)
			result["disk_used_gb"] = strconv.FormatInt(used, 10)
		}
	}
	if up, ok := sections["__UP__"]; ok {
		if seconds := parseUptime(up); seconds >= 0 {
			result["uptime_seconds"] = strconv.FormatInt(seconds, 10)
		}
	}
	if temp, ok := sections["__TEMP__"]; ok {
		if temperature, ok := parseTemperature(temp); ok {
			result["temperature_c"] = strconv.FormatFloat(temperature, 'f', 1, 64)
		}
	}
	if np, ok := sections["__NPROC__"]; ok {
		if cores := parseNproc(np); cores > 0 {
			result["cpu_cores"] = strconv.Itoa(cores)
		}
	}

	return result
}

func splitByMarker(output string, markers []string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(output, "\n")
	current := ""
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if isMarker(line, markers) {
			current = line
			result[current] = ""
			continue
		}
		if current != "" {
			if result[current] == "" {
				result[current] = line
			} else {
				result[current] += "\n" + line
			}
		}
	}
	return result
}

func isMarker(line string, markers []string) bool {
	for _, marker := range markers {
		if line == marker {
			return true
		}
	}
	return false
}

func calcCPUPercent(stat1, stat2 string) int {
	firstLine := func(stat string) string {
		line := strings.TrimSpace(stat)
		if idx := strings.Index(line, "\n"); idx >= 0 {
			return strings.TrimSpace(line[:idx])
		}
		return line
	}
	parse := func(stat string) (total int64, idle int64, ok bool) {
		fields := strings.Fields(firstLine(stat))
		if len(fields) < 5 {
			return 0, 0, false
		}
		if fields[0] != "cpu" {
			return 0, 0, false
		}
		var values []int64
		for _, f := range fields[1:] {
			val, err := strconv.ParseInt(f, 10, 64)
			if err != nil {
				return 0, 0, false
			}
			values = append(values, val)
		}
		for _, v := range values {
			total += v
		}
		idle = values[3]
		if len(values) > 4 {
			idle += values[4]
		}
		return total, idle, true
	}

	t1, i1, ok1 := parse(stat1)
	t2, i2, ok2 := parse(stat2)
	if !ok1 || !ok2 {
		return -1
	}
	totald := t2 - t1
	idled := i2 - i1
	if totald <= 0 {
		return 0
	}
	usage := int((100 * (totald - idled)) / totald)
	if usage < 0 {
		return 0
	}
	if usage > 100 {
		return 100
	}
	return usage
}

func parseMeminfo(meminfo string) (totalMB int64, availMB int64) {
	var memTotalKB, memAvailKB, memFreeKB, buffersKB, cachedKB int64
	for _, line := range strings.Split(meminfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		val, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			memTotalKB = val
		case "MemAvailable":
			memAvailKB = val
		case "MemFree":
			memFreeKB = val
		case "Buffers":
			buffersKB = val
		case "Cached":
			cachedKB = val
		}
	}
	if memAvailKB == 0 {
		memAvailKB = memFreeKB + buffersKB + cachedKB
	}
	totalMB = memTotalKB / 1024
	availMB = memAvailKB / 1024
	return totalMB, availMB
}

func parseDfRoot(df string) (totalGB int64, usedGB int64) {
	lines := strings.Split(df, "\n")
	if len(lines) < 2 {
		return 0, 0
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 3 {
		return 0, 0
	}
	totalKB, err1 := strconv.ParseInt(fields[1], 10, 64)
	usedKB, err2 := strconv.ParseInt(fields[2], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	totalGB = totalKB / (1024 * 1024)
	usedGB = usedKB / (1024 * 1024)
	return totalGB, usedGB
}

func parseUptime(uptime string) int64 {
	fields := strings.Fields(uptime)
	if len(fields) == 0 {
		return -1
	}
	parts := strings.Split(fields[0], ".")
	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return -1
	}
	return seconds
}

func parseNproc(nproc string) int {
	fields := strings.Fields(nproc)
	if len(fields) == 0 {
		return 0
	}
	val, err := strconv.Atoi(fields[0])
	if err != nil || val < 1 {
		return 0
	}
	return val
}

func parseTemperature(raw string) (float64, bool) {
	var best float64
	found := false
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		val, ok := extractFirstFloat(line)
		if !ok {
			continue
		}
		if val > 1000 {
			val = val / 1000.0
		}
		if val <= 0 || val > 200 {
			continue
		}
		if !found || val > best {
			best = val
			found = true
		}
	}
	if !found {
		return 0, false
	}
	return best, true
}

func extractFirstFloat(line string) (float64, bool) {
	start := -1
	end := -1
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch >= '0' && ch <= '9' {
			start = i
			break
		}
	}
	if start == -1 {
		return 0, false
	}
	end = start
	for end < len(line) {
		ch := line[end]
		if (ch >= '0' && ch <= '9') || ch == '.' {
			end++
			continue
		}
		break
	}
	if end <= start {
		return 0, false
	}
	val, err := strconv.ParseFloat(line[start:end], 64)
	if err != nil {
		return 0, false
	}
	return val, true
}

type netDialer struct{}

func (d *netDialer) DialSSH(ctx context.Context, network, address string, config *ssh.ClientConfig) (*ssh.Client, error) {
	conn, err := dialContext(ctx, network, address, config.Timeout)
	if err != nil {
		return nil, err
	}

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return ssh.NewClient(clientConn, chans, reqs), nil
}

func dialContext(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	return d.DialContext(ctx, network, address)
}
