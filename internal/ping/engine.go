package ping

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-ping/ping"
)

// Target represents a monitoring target
type Target struct {
	ID                 int
	Name               string
	Address            string
	Type               string
	MonitoringType     string
	HTTPMethod         string
	HTTPPath           string
	HTTPHeaders        string
	ExpectedStatusCode int
	ExpectedContent    string
	SSLCheck           bool
	FollowRedirects    bool
	TimeoutSec         int
	IntervalSec        int
	Port               *int
	Path               *string
	TimeoutMs          int
	Enabled            bool
	Tags               *string
}

// PingConfig holds ping configuration
type PingConfig struct {
	TimeoutMs int
	Retry     int
	PacingMs  int
}

// PingResult represents a single ping result
type PingResult struct {
	Host     string
	Success  bool
	Duration time.Duration
	Error    string
	// HTTP-specific fields
	StatusCode        int
	ResponseSizeBytes int
	SSLExpiryDate     *time.Time
	ResponseHeaders   map[string]string
}

// PingEngine interface for different ping implementations
type PingEngine interface {
	Probe(hosts []string, cfg PingConfig) ([]PingResult, error)
	Name() string
	IsAvailable() bool
}

// fpingEngine uses external fping command
type fpingEngine struct{}

func (f *fpingEngine) Name() string {
	return "fping"
}

func (f *fpingEngine) IsAvailable() bool {
	_, err := exec.LookPath("fping")
	if err != nil {
		return false
	}
	return f.canProbe()
}

func (f *fpingEngine) Probe(hosts []string, cfg PingConfig) ([]PingResult, error) {
	if len(hosts) == 0 {
		return []PingResult{}, nil
	}

	// fping komutu: her host için tek probe (-C 1), timeout -t ms
	// Not: -i (pacing) ve -r (retry) çıkarıldı; basit ve deterministik tutuyoruz.
	args := []string{
		"-C", "1",
		"-t", strconv.Itoa(cfg.TimeoutMs),
	}
	args = append(args, hosts...)

	cmd := exec.Command("fping", args...)
	output, err := cmd.CombinedOutput()
	out := string(output)
	if err != nil && isFpingPermissionError(out) {
		return nil, fmt.Errorf("fping cannot create raw socket: %s", strings.TrimSpace(out))
	}

	// Örnek satırlar:
	// "8.8.8.8 : 16.4"
	// "1.1.1.1 : -"
	results := make([]PingResult, len(hosts))
	index := map[string]int{}
	for i, h := range hosts {
		index[h] = i
		results[i] = PingResult{Host: h, Success: false, Error: "no response"}
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		host := strings.TrimSpace(parts[0])
		payload := strings.TrimSpace(parts[1]) // "16.4" veya "-"

		i, ok := index[host]
		if !ok {
			continue
		}

		if payload == "-" {
			results[i] = PingResult{Host: host, Success: false, Error: "timeout"}
			continue
		}

		// payload bir float (ms). Örn "16.4"
		if v, err := strconv.ParseFloat(strings.Fields(payload)[0], 64); err == nil {
			results[i] = PingResult{
				Host:     host,
				Success:  true,
				Duration: time.Duration(v * float64(time.Millisecond)),
			}
		} else {
			results[i] = PingResult{Host: host, Success: false, Error: "parse error"}
		}
	}
	return results, nil
}

func (f *fpingEngine) canProbe() bool {
	cmd := exec.Command("fping", "-C", "1", "-t", "100", "127.0.0.1")
	output, err := cmd.CombinedOutput()
	out := string(output)
	if err != nil {
		if isFpingPermissionError(out) {
			log.Printf("⚠️ fping found but not usable without raw socket permissions: %s", strings.TrimSpace(out))
		} else {
			log.Printf("⚠️ fping found but probe test failed, falling back if possible: %s", strings.TrimSpace(out))
		}
		return false
	}
	return strings.Contains(out, "127.0.0.1")
}

func isFpingPermissionError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "operation not permitted") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "must run as root") ||
		strings.Contains(lower, "can't create") ||
		strings.Contains(lower, "cannot create") ||
		strings.Contains(lower, "socket")
}

// nativeEngine uses Go native ping
type nativeEngine struct{}

func (n *nativeEngine) Name() string {
	return "native"
}

func (n *nativeEngine) IsAvailable() bool {
	return true // Always available
}

func (n *nativeEngine) Probe(hosts []string, cfg PingConfig) ([]PingResult, error) {
	results := make([]PingResult, len(hosts))

	for i, host := range hosts {
		pinger, err := ping.NewPinger(host)
		if err != nil {
			results[i] = PingResult{
				Host:    host,
				Success: false,
				Error:   err.Error(),
			}
			continue
		}

		pinger.Count = 1
		pinger.Timeout = time.Duration(cfg.TimeoutMs) * time.Millisecond
		pinger.SetPrivileged(true) // Windows'ta privileged mode gerekli

		err = pinger.Run()
		if err != nil {
			results[i] = PingResult{
				Host:    host,
				Success: false,
				Error:   err.Error(),
			}
			continue
		}

		stats := pinger.Statistics()
		results[i] = PingResult{
			Host:     host,
			Success:  stats.PacketsRecv > 0,
			Duration: stats.AvgRtt,
		}
	}

	return results, nil
}

// windowsPingEngine uses Windows ping command
type windowsPingEngine struct{}

func (w *windowsPingEngine) Name() string {
	return "windows-ping"
}

func (w *windowsPingEngine) IsAvailable() bool {
	return true // Windows'ta her zaman mevcut
}

func (w *windowsPingEngine) Probe(hosts []string, cfg PingConfig) ([]PingResult, error) {
	results := make([]PingResult, len(hosts))

	for i, host := range hosts {
		// Windows ping komutu: ping -n 1 -w timeout host
		cmd := exec.Command("ping", "-n", "1", "-w", strconv.Itoa(cfg.TimeoutMs), host)
		output, err := cmd.CombinedOutput()

		if err != nil {
			results[i] = PingResult{
				Host:    host,
				Success: false,
				Error:   err.Error(),
			}
			continue
		}

		// Parse Windows ping output
		outputStr := string(output)
		if strings.Contains(outputStr, "Reply from") {
			// Success - extract time
			timeRegex := regexp.MustCompile(`time[<=](\d+)ms`)
			matches := timeRegex.FindStringSubmatch(outputStr)

			duration := time.Duration(0)
			if len(matches) > 1 {
				if timeMs, err := strconv.Atoi(matches[1]); err == nil {
					duration = time.Duration(timeMs) * time.Millisecond
				}
			}

			results[i] = PingResult{
				Host:     host,
				Success:  true,
				Duration: duration,
			}
		} else {
			results[i] = PingResult{
				Host:    host,
				Success: false,
				Error:   "no reply",
			}
		}
	}

	return results, nil
}

type autoEngine struct {
	engine PingEngine
}

func (a *autoEngine) Name() string {
	if a.engine != nil {
		return fmt.Sprintf("auto(%s)", a.engine.Name())
	}
	return "auto(none)"
}

func (a *autoEngine) IsAvailable() bool {
	return a.engine != nil && a.engine.IsAvailable()
}

func (a *autoEngine) Probe(hosts []string, cfg PingConfig) ([]PingResult, error) {
	if a.engine == nil {
		return nil, fmt.Errorf("no ping engine available")
	}
	return a.engine.Probe(hosts, cfg)
}

// NewPingEngine creates a new ping engine based on mode
func NewPingEngine(mode string) PingEngine {
	switch mode {
	case "fping":
		engine := &fpingEngine{}
		if engine.IsAvailable() {
			log.Printf("🌐 Ping engine: fping")
			return engine
		}
		log.Printf("⚠️ fping not available, falling back to native")
		return &nativeEngine{}

	case "native":
		log.Printf("🌐 Ping engine: native")
		return &nativeEngine{}

	case "auto", "":
		// 1) fping varsa onu kullan
		fping := &fpingEngine{}
		if fping.IsAvailable() {
			log.Printf("🌐 Ping engine: auto(fping)")
			return &autoEngine{engine: fping}
		}
		// 2) OS'e göre doğru fallback: Windows'ta windows-ping, diğerlerinde native
		if runtime.GOOS == "windows" {
			log.Printf("🌐 Ping engine: auto(windows-ping)")
			return &autoEngine{engine: &windowsPingEngine{}}
		}
		log.Printf("🌐 Ping engine: auto(native)")
		return &autoEngine{engine: &nativeEngine{}}

	default:
		log.Printf("⚠️ Unknown ping engine mode: %s, using auto", mode)
		return NewPingEngine("auto")
	}
}
