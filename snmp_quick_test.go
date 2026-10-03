package main
import (
	"fmt"
	"time"
	"github.com/gosnmp/gosnmp"
)
func main() {
	targets := []string{"10.20.10.40", "10.20.10.50", "10.20.11.28"}
	for _, ip := range targets {
		params := &gosnmp.GoSNMP{
			Target: ip, Port: 161, Community: "public",
			Version: gosnmp.Version2c, Timeout: 3 * time.Second, Retries: 1,
		}
		if err := params.Connect(); err != nil {
			fmt.Printf("❌ %s: Bağlantı hatası - %v\n", ip, err)
			continue
		}
		result, err := params.Get([]string{"1.3.6.1.2.1.1.1.0"})
		params.Conn.Close()
		if err == nil && len(result.Variables) > 0 {
			fmt.Printf("✅ %s: SNMP ÇALIŞIYOR - %v\n", ip, result.Variables[0].Value)
		} else {
			fmt.Printf("❌ %s: SNMP yanıt yok - %v\n", ip, err)
		}
	}
}
