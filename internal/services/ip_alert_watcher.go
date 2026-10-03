package services

import (
	"context"
	"database/sql"
	"log"
	"net/netip"
	"strings"
	"time"

	"systrack/internal/network/scanner"
)

type IPAlertWatcher struct {
	db       *sql.DB
	interval time.Duration
	stopChan chan struct{}
}

func NewIPAlertWatcher(db *sql.DB, interval time.Duration) *IPAlertWatcher {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &IPAlertWatcher{
		db:       db,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

func (w *IPAlertWatcher) Start() {
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()

		w.checkOnce()

		for {
			select {
			case <-ticker.C:
				w.checkOnce()
			case <-w.stopChan:
				log.Println("IP alert watcher stopped")
				return
			}
		}
	}()

	log.Printf("IP alert watcher started (%s interval)", w.interval.String())
}

func (w *IPAlertWatcher) Stop() {
	close(w.stopChan)
}

func (w *IPAlertWatcher) checkOnce() {
	if w.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	baselineBySubnet, prefixBySubnet, err := w.loadBaselines(ctx)
	if err != nil {
		log.Printf("ip-alerts: baseline load failed: %v", err)
		return
	}
	if len(prefixBySubnet) == 0 {
		return
	}

	arp := scanner.SnapshotARPTable()
	if len(arp) == 0 {
		return
	}

	stmt, err := w.db.PrepareContext(ctx, "INSERT INTO ip_alerts (subnet, ip) VALUES (?, ?) ON DUPLICATE KEY UPDATE id = id")
	if err != nil {
		log.Printf("ip-alerts: prepare insert failed: %v", err)
		return
	}
	defer stmt.Close()

	for ipStr := range arp {
		addr, err := netip.ParseAddr(strings.TrimSpace(ipStr))
		if err != nil || !addr.Is4() {
			continue
		}

		for subnet, prefix := range prefixBySubnet {
			if !prefix.Contains(addr) {
				continue
			}

			if _, ok := baselineBySubnet[subnet][addr.String()]; ok {
				continue
			}

			if _, err := stmt.ExecContext(ctx, subnet, addr.String()); err != nil {
				log.Printf("ip-alerts: insert failed (subnet=%s ip=%s): %v", subnet, addr.String(), err)
			}
		}
	}
}

func (w *IPAlertWatcher) loadBaselines(ctx context.Context) (map[string]map[string]struct{}, map[string]netip.Prefix, error) {
	baselineBySubnet := make(map[string]map[string]struct{})
	prefixBySubnet := make(map[string]netip.Prefix)

	// 1. Load IPs from ip_results (scan results)
	rows, err := w.db.QueryContext(ctx, "SELECT subnet, ip FROM ip_results")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var subnet, ip string
		if err := rows.Scan(&subnet, &ip); err != nil {
			return nil, nil, err
		}

		subnet = strings.TrimSpace(subnet)
		ip = strings.TrimSpace(ip)
		if subnet == "" || ip == "" {
			continue
		}

		if _, ok := prefixBySubnet[subnet]; !ok {
			prefix, err := netip.ParsePrefix(subnet)
			if err != nil {
				continue
			}
			if !prefix.Addr().Is4() {
				continue
			}
			prefixBySubnet[subnet] = prefix.Masked()
		}

		set := baselineBySubnet[subnet]
		if set == nil {
			set = make(map[string]struct{})
			baselineBySubnet[subnet] = set
		}
		set[ip] = struct{}{}
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// 2. Load IPs from targets table (monitoring targets)
	targetsRows, err := w.db.QueryContext(ctx, "SELECT address FROM targets WHERE enabled = 1")
	if err != nil {
		log.Printf("ip-alerts: failed to load targets (continuing with scan results only): %v", err)
	} else {
		defer targetsRows.Close()

		for targetsRows.Next() {
			var address string
			if err := targetsRows.Scan(&address); err != nil {
				continue
			}

			address = strings.TrimSpace(address)
			if address == "" {
				continue
			}

			// Parse the address as IP
			targetIP, err := netip.ParseAddr(address)
			if err != nil || !targetIP.Is4() {
				continue
			}

			// Find which subnet this IP belongs to
			var matchedSubnet string
			for subnet, prefix := range prefixBySubnet {
				if prefix.Contains(targetIP) {
					matchedSubnet = subnet
					break
				}
			}

			// If no subnet matched from ip_results, try to infer subnet from target IP
			if matchedSubnet == "" {
				// Use /24 as default subnet for targets without explicit subnet
				subnetStr := targetIP.String() + "/24"
				prefix, err := netip.ParsePrefix(subnetStr)
				if err != nil {
					continue
				}
				prefix = prefix.Masked()

				// Use the network address as the subnet key
				matchedSubnet = prefix.String()
				prefixBySubnet[matchedSubnet] = prefix
			}

			// Add target IP to baseline
			set := baselineBySubnet[matchedSubnet]
			if set == nil {
				set = make(map[string]struct{})
				baselineBySubnet[matchedSubnet] = set
			}
			set[targetIP.String()] = struct{}{}
		}

		if err := targetsRows.Err(); err != nil {
			log.Printf("ip-alerts: error iterating targets: %v", err)
		}
	}

	return baselineBySubnet, prefixBySubnet, nil
}
