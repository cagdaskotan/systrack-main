package handlers

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetNetworkInfo - Sunucunun ağ bilgilerini döndürür (IP aralığı tespiti için)
func GetNetworkInfo() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Sunucunun network interface'lerini al
		interfaces, err := net.Interfaces()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Network interfaces okunamadı"})
			return
		}

		// İlk fiziksel (non-loopback, non-docker) interface'i bul
		for _, iface := range interfaces {
			// Loopback ve docker interface'lerini atla
			if iface.Flags&net.FlagLoopback != 0 ||
				strings.HasPrefix(iface.Name, "docker") ||
				strings.HasPrefix(iface.Name, "br-") ||
				strings.HasPrefix(iface.Name, "veth") {
				continue
			}

			// Interface'in adreslerini al
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}

			// IPv4 adresini bul
			for _, addr := range addrs {
				ipNet, ok := addr.(*net.IPNet)
				if !ok {
					continue
				}

				ip := ipNet.IP
				// Sadece IPv4, private IP aralıklarını kullan
				if ip.To4() != nil && (ip.IsPrivate() || ip.IsLoopback() == false) {
					// Subnet mask'i al
					mask := ipNet.Mask
					maskSize, _ := mask.Size()

					// Network adresini hesapla (IP & Mask)
					network := ip.Mask(mask)

					// CIDR notasyonu oluştur
					cidrStr := network.String() + "/" + itoa(maskSize)

					// Broadcast adresini hesapla
					broadcast := make(net.IP, len(network))
					copy(broadcast, network)
					for i := range broadcast {
						broadcast[i] |= ^mask[i]
					}

					c.JSON(http.StatusOK, gin.H{
						"cidr":        cidrStr,
						"ip":          ip.String(),
						"network":     network.String(),
						"broadcast":   broadcast.String(),
						"subnet_mask": net.IP(mask).String(),
						"interface":   iface.Name,
					})
					return
				}
			}
		}

		// Hiçbir uygun interface bulunamadıysa, varsayılan öner
		c.JSON(http.StatusOK, gin.H{
			"cidr":    "192.168.1.0/24",
			"message": "Otomatik tespit başarısız, varsayılan aralık önerildi",
		})
	}
}

// itoa - int to string conversion helper
func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	// For numbers >= 10, use proper conversion
	var buf [12]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
