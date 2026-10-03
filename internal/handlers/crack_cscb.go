package handlers

import (
	"bufio"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CSCB = abuse.ch MalwareBazaar Code Signing Certificate Blocklist. It lists thumbprints
// of code-signing certificates known to have been used to sign malware, independent of
// any particular file name or hash - a binary signed by a blocklisted cert is a strong
// supply-chain/stolen-certificate signal even if the file itself has never been seen.
// https://bazaar.abuse.ch/export/csv/cscb/ (public CSV, no Auth-Key required)
const cscbURL = "https://bazaar.abuse.ch/export/csv/cscb/"

const cscbCacheTTL = 12 * time.Hour

var cscbCache = struct {
	sync.RWMutex
	thumbprints map[string]bool
	fetchedAt   time.Time
}{}

// isCertBlocklisted reports whether thumbprint appears in the cached CSCB list,
// refreshing the cache from abuse.ch if it is empty or older than cscbCacheTTL.
// Network/parse failures degrade to "not blocklisted" (fail-open) rather than blocking
// the scan - CSCB is a supporting signal, not the only one.
func isCertBlocklisted(thumbprint string) bool {
	thumbprint = normalizeThumbprint(thumbprint)
	if thumbprint == "" {
		return false
	}

	cscbCache.RLock()
	stale := time.Since(cscbCache.fetchedAt) > cscbCacheTTL || cscbCache.thumbprints == nil
	set := cscbCache.thumbprints
	cscbCache.RUnlock()

	if stale {
		refreshCSCBCache()
		cscbCache.RLock()
		set = cscbCache.thumbprints
		cscbCache.RUnlock()
	}
	return set != nil && set[thumbprint]
}

func normalizeThumbprint(t string) string {
	t = strings.ToUpper(strings.TrimSpace(t))
	t = strings.ReplaceAll(t, ":", "")
	t = strings.ReplaceAll(t, " ", "")
	return t
}

func refreshCSCBCache() {
	cscbCache.Lock()
	defer cscbCache.Unlock()
	// Another goroutine may have refreshed it while we waited for the lock.
	if time.Since(cscbCache.fetchedAt) <= cscbCacheTTL && cscbCache.thumbprints != nil {
		return
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(cscbURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}

	thumbprints, err := parseCSCBCSV(resp.Body)
	if err != nil || len(thumbprints) == 0 {
		return
	}
	cscbCache.thumbprints = thumbprints
	cscbCache.fetchedAt = time.Now()
}

// parseCSCBCSV parses the abuse.ch CSCB export. The export prefixes the header with
// "#"-commented metadata lines, same as MalwareBazaar's other CSV exports, so a strict
// encoding/csv reader is avoided in favor of manual comma-splitting on the header/data
// rows actually expected here (quoted commas do not occur in thumbprint/serial fields).
func parseCSCBCSV(body io.Reader) (map[string]bool, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var header []string
	thumbprintIdx := -1
	out := make(map[string]bool)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := splitCSVLine(line)
		if header == nil {
			header = fields
			for i, h := range header {
				if strings.EqualFold(strings.TrimSpace(h), "thumbprint") {
					thumbprintIdx = i
					break
				}
			}
			continue
		}
		if thumbprintIdx < 0 || thumbprintIdx >= len(fields) {
			continue
		}
		if tp := normalizeThumbprint(fields[thumbprintIdx]); tp != "" {
			out[tp] = true
		}
	}
	return out, scanner.Err()
}

// splitCSVLine splits a simple quoted-or-unquoted CSV line. Good enough for CSCB's flat
// fields (hex thumbprints, short CNs) without pulling in a full CSV state machine.
func splitCSVLine(line string) []string {
	var fields []string
	var cur strings.Builder
	inQuotes := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch {
		case ch == '"':
			inQuotes = !inQuotes
		case ch == ',' && !inQuotes:
			fields = append(fields, strings.Trim(cur.String(), `" `))
			cur.Reset()
		default:
			cur.WriteByte(ch)
		}
	}
	fields = append(fields, strings.Trim(cur.String(), `" `))
	return fields
}

// cscbCacheSize is exposed for diagnostics/tests only.
func cscbCacheSize() int {
	cscbCache.RLock()
	defer cscbCache.RUnlock()
	return len(cscbCache.thumbprints)
}
