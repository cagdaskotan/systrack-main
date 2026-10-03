package auth

import (
	"database/sql"
	"fmt"
)

// Permission represents a system permission
type Permission struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Resource    string `json:"resource"`
	Action      string `json:"action"`
}

// RolePermission represents the relationship between roles and permissions
type RolePermission struct {
	RoleID       int `json:"role_id"`
	PermissionID int `json:"permission_id"`
}

// Permission constants
const (
	// Dashboard permissions
	PermissionDashboardView = "dashboard.view"

	// Target permissions
	PermissionTargetView   = "target.view"
	PermissionTargetCreate = "target.create"
	PermissionTargetUpdate = "target.update"
	PermissionTargetDelete = "target.delete"
	PermissionTargetPing   = "target.ping"

	// User permissions
	PermissionUserView   = "user.view"
	PermissionUserCreate = "user.create"
	PermissionUserUpdate = "user.update"
	PermissionUserDelete = "user.delete"

	// Alert permissions
	PermissionAlertView   = "alert.view"
	PermissionAlertClose  = "alert.close"
	PermissionAlertCreate = "alert.create"

	// Report permissions
	PermissionReportView   = "report.view"
	PermissionReportCreate = "report.create"
	PermissionReportExport = "report.export"
	PermissionReportDelete = "report.delete"

	// Settings permissions
	PermissionSettingsView   = "settings.view"
	PermissionSettingsUpdate = "settings.update"

	// Notification permissions
	PermissionNotificationView   = "notification.view"
	PermissionNotificationUpdate = "notification.update"
	PermissionNotificationTest   = "notification.test"

	// Analytics permissions
	PermissionAnalyticsView = "analytics.view"
	PermissionSLAView       = "sla.view"
	PermissionNetworkScan   = "network.scan"
)

// Role permissions mapping
var RolePermissions = map[string][]string{
	"admin": {
		PermissionDashboardView,
		PermissionTargetView,
		PermissionTargetCreate,
		PermissionTargetUpdate,
		PermissionTargetDelete,
		PermissionTargetPing,
		PermissionUserView,
		PermissionUserCreate,
		PermissionUserUpdate,
		PermissionUserDelete,
		PermissionAlertView,
		PermissionAlertClose,
		PermissionAlertCreate,
		PermissionReportView,
		PermissionReportCreate,
		PermissionReportExport,
		PermissionReportDelete,
		PermissionSettingsView,
		PermissionSettingsUpdate,
		PermissionNotificationView,
		PermissionNotificationUpdate,
		PermissionNotificationTest,
		PermissionAnalyticsView,
		PermissionSLAView,
		PermissionNetworkScan,
	},
	"user": {
		PermissionDashboardView,
		PermissionTargetView,
		PermissionTargetCreate,
		PermissionTargetUpdate,
		PermissionTargetPing,
		PermissionAlertView,
		PermissionAlertClose,
		PermissionReportView,
		PermissionAnalyticsView,
		PermissionSLAView,
	},
	"viewer": {
		PermissionDashboardView,
		PermissionTargetView,
		PermissionAlertView,
		PermissionReportView,
		PermissionAnalyticsView,
		PermissionSLAView,
		PermissionUserView,
	},
}

// HasPermission checks if a role has a specific permission
func HasPermission(role, permission string) bool {
	permissions, exists := RolePermissions[role]
	if !exists {
		return false
	}

	for _, p := range permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// GetRolePermissions returns all permissions for a role
func GetRolePermissions(role string) []string {
	permissions, exists := RolePermissions[role]
	if !exists {
		return []string{}
	}
	return permissions
}

// GetAllPermissions returns all available permissions
func GetAllPermissions() []Permission {
	return []Permission{
		{Name: PermissionDashboardView, Description: "Ana dashboard sayfasını görüntüleme yetkisi. Sistem genel durumu, istatistikler ve özet bilgileri görme.", Resource: "dashboard", Action: "view"},
		{Name: PermissionTargetView, Description: "İzleme hedeflerini listeleme ve detaylarını görüntüleme yetkisi. Hedef durumları, ping sonuçları ve geçmiş verileri görme.", Resource: "target", Action: "view"},
		{Name: PermissionTargetCreate, Description: "Yeni izleme hedefi oluşturma yetkisi. IP adresi, domain, port ve izleme parametrelerini belirleme.", Resource: "target", Action: "create"},
		{Name: PermissionTargetUpdate, Description: "Mevcut izleme hedeflerini düzenleme yetkisi. Hedef bilgilerini, izleme sıklığını ve parametrelerini güncelleme.", Resource: "target", Action: "update"},
		{Name: PermissionTargetDelete, Description: "İzleme hedeflerini silme yetkisi. Hedefi sistemden tamamen kaldırma ve ilgili verileri temizleme.", Resource: "target", Action: "delete"},
		{Name: PermissionTargetPing, Description: "Hedeflere manuel ping gönderme yetkisi. Anlık durum kontrolü ve test ping işlemleri yapma.", Resource: "target", Action: "ping"},
		{Name: PermissionUserView, Description: "Sistem kullanıcılarını listeleme ve görüntüleme yetkisi. Kullanıcı bilgileri, roller ve durumları görme.", Resource: "user", Action: "view"},
		{Name: PermissionUserCreate, Description: "Yeni sistem kullanıcısı oluşturma yetkisi. Kullanıcı hesabı, rol atama ve lisans süresi belirleme.", Resource: "user", Action: "create"},
		{Name: PermissionUserUpdate, Description: "Kullanıcı bilgilerini güncelleme yetkisi. Rol değiştirme, lisans uzatma ve hesap durumu yönetimi.", Resource: "user", Action: "update"},
		{Name: PermissionUserDelete, Description: "Sistem kullanıcısını silme yetkisi. Kullanıcı hesabını tamamen kaldırma ve ilgili verileri temizleme.", Resource: "user", Action: "delete"},
		{Name: PermissionAlertView, Description: "Sistem uyarılarını görüntüleme yetkisi. Açık/kapalı uyarıları, uyarı geçmişi ve detaylarını görme.", Resource: "alert", Action: "view"},
		{Name: PermissionAlertClose, Description: "Açık uyarıları kapatma yetkisi. Sorun çözüldüğünde uyarı durumunu güncelleme ve kapatma.", Resource: "alert", Action: "close"},
		{Name: PermissionAlertCreate, Description: "Manuel uyarı oluşturma yetkisi. Sistem tarafından otomatik oluşturulmayan özel uyarılar ekleme.", Resource: "alert", Action: "create"},
		{Name: PermissionReportView, Description: "Sistem raporlarını görüntüleme yetkisi. Mevcut raporları listeleme, detaylarını görme ve indirme.", Resource: "report", Action: "view"},
		{Name: PermissionReportCreate, Description: "Yeni rapor oluşturma yetkisi. Özel rapor parametreleri belirleme ve rapor üretim sürecini başlatma.", Resource: "report", Action: "create"},
		{Name: PermissionReportDelete, Description: "Olusturulmus rapor kayitlarini silme yetkisi. Gereksiz raporlari temizleme ve depolama alanini yonetme.", Resource: "report", Action: "delete"},
		{Name: PermissionSettingsView, Description: "Sistem ayarlarını görüntüleme yetkisi. Genel konfigürasyon, bildirim ayarları ve sistem parametrelerini görme.", Resource: "settings", Action: "view"},
		{Name: PermissionSettingsUpdate, Description: "Sistem ayarlarını güncelleme yetkisi. Konfigürasyon değişiklikleri, bildirim ayarları ve sistem parametrelerini düzenleme.", Resource: "settings", Action: "update"},
		{Name: PermissionNotificationView, Description: "Bildirim ayarlarını görüntüleme yetkisi. E-posta, SMS, webhook ve diğer bildirim kanallarını görme.", Resource: "notification", Action: "view"},
		{Name: PermissionNotificationUpdate, Description: "Bildirim ayarlarını güncelleme yetkisi. Bildirim kanallarını yapılandırma, şablonları düzenleme ve test etme.", Resource: "notification", Action: "update"},
		{Name: PermissionNotificationTest, Description: "Bildirim kanallarını test etme yetkisi. E-posta, SMS ve webhook bağlantılarını doğrulama ve test mesajları gönderme.", Resource: "notification", Action: "test"},
		{Name: PermissionAnalyticsView, Description: "Sistem analitiklerini görüntüleme yetkisi. Performans metrikleri, trend analizleri ve istatistiksel verileri görme.", Resource: "analytics", Action: "view"},
		{Name: PermissionSLAView, Description: "SLA (Service Level Agreement) raporlarını görüntüleme yetkisi. Uptime oranları, SLA uyumluluk ve performans metriklerini görme.", Resource: "sla", Action: "view"},
		{Name: PermissionNetworkScan, Description: "Yerel ağda subnet taraması başlatma ve sonuçlarını görüntüleme yetkisi.", Resource: "network", Action: "scan"},
	}
}

// CheckUserPermission checks if a user has a specific permission
func CheckUserPermission(db *sql.DB, userID int, permission string) (bool, error) {
	// Get user role
	var role string
	query := `SELECT role FROM users WHERE id = ?`
	err := db.QueryRow(query, userID).Scan(&role)
	if err != nil {
		return false, fmt.Errorf("failed to get user role: %w", err)
	}

	return HasPermission(role, permission), nil
}

// GetUserPermissions returns all permissions for a user
func GetUserPermissions(db *sql.DB, userID int) ([]string, error) {
	// Get user role
	var role string
	query := `SELECT role FROM users WHERE id = ?`
	err := db.QueryRow(query, userID).Scan(&role)
	if err != nil {
		return nil, fmt.Errorf("failed to get user role: %w", err)
	}

	return GetRolePermissions(role), nil
}

// ValidatePermission validates if a permission string is valid
func ValidatePermission(permission string) bool {
	allPermissions := GetAllPermissions()
	for _, p := range allPermissions {
		if p.Name == permission {
			return true
		}
	}
	return false
}

// GetPermissionsByResource returns all permissions for a specific resource
func GetPermissionsByResource(resource string) []Permission {
	allPermissions := GetAllPermissions()
	var result []Permission
	for _, p := range allPermissions {
		if p.Resource == resource {
			result = append(result, p)
		}
	}
	return result
}

// GetPermissionsByAction returns all permissions for a specific action
func GetPermissionsByAction(action string) []Permission {
	allPermissions := GetAllPermissions()
	var result []Permission
	for _, p := range allPermissions {
		if p.Action == action {
			result = append(result, p)
		}
	}
	return result
}

// GetRoleDescription returns a description for a role
func GetRoleDescription(role string) string {
	descriptions := map[string]string{
		"admin":  "Tam sistem yöneticisi. Tüm özelliklere erişim, kullanıcı yönetimi, sistem ayarları ve tüm izleme işlemlerini gerçekleştirebilir.",
		"user":   "Standart kullanıcı. İzleme hedeflerini yönetebilir, uyarıları görüntüleyebilir ve raporları oluşturabilir. Kullanıcı yönetimi ve sistem ayarlarına erişimi yoktur.",
		"viewer": "Sadece görüntüleme yetkisi. Sistem durumunu, hedefleri, uyarıları ve raporları görüntüleyebilir ancak hiçbir değişiklik yapamaz.",
	}

	if desc, exists := descriptions[role]; exists {
		return desc
	}
	return "Bilinmeyen rol"
}

// GetPermissionCategories returns permissions grouped by category
func GetPermissionCategories() map[string][]Permission {
	allPermissions := GetAllPermissions()
	categories := make(map[string][]Permission)

	for _, perm := range allPermissions {
		category := getCategoryForResource(perm.Resource)
		categories[category] = append(categories[category], perm)
	}

	return categories
}

// getCategoryForResource returns the category name for a resource
func getCategoryForResource(resource string) string {
	categories := map[string]string{
		"dashboard":    "Genel",
		"target":       "Hedef Yönetimi",
		"user":         "Kullanıcı Yönetimi",
		"alert":        "Uyarı Yönetimi",
		"report":       "Raporlama",
		"settings":     "Sistem Ayarları",
		"notification": "Bildirim Yönetimi",
		"analytics":    "Analitik",
		"sla":          "SLA Yönetimi",
		"network":      "Ağ Araçları",
	}

	if category, exists := categories[resource]; exists {
		return category
	}
	return "Diğer"
}

// GetRoleSummary returns a summary of what each role can do
func GetRoleSummary() map[string]map[string]interface{} {
	return map[string]map[string]interface{}{
		"admin": {
			"description": "Tam sistem yöneticisi",
			"capabilities": []string{
				"Tüm sistem özelliklerine erişim",
				"Kullanıcı oluşturma, düzenleme ve silme",
				"İzleme hedeflerini yönetme",
				"Sistem ayarlarını değiştirme",
				"Bildirim kanallarını yapılandırma",
				"Raporları oluşturma ve dışa aktarma",
				"Uyarıları yönetme",
				"SLA raporlarını görüntüleme",
			},
			"restrictions": []string{
				"Kendi hesabını silemez",
			},
		},
		"user": {
			"description": "Standart kullanıcı",
			"capabilities": []string{
				"İzleme hedeflerini görüntüleme ve yönetme",
				"Uyarıları görüntüleme ve kapatma",
				"Raporları görüntüleme",
				"Analitik verilerini görüntüleme",
				"SLA raporlarını görüntüleme",
				"Manuel ping gönderme",
			},
			"restrictions": []string{
				"Kullanıcı yönetimi yapamaz",
				"Sistem ayarlarını değiştiremez",
				"Bildirim ayarlarını yönetemez",
				"Rapor oluşturamaz",
			},
		},
		"viewer": {
			"description": "Sadece görüntüleme yetkisi",
			"capabilities": []string{
				"Dashboard'u görüntüleme",
				"İzleme hedeflerini görüntüleme",
				"Uyarıları görüntüleme",
				"Raporları görüntüleme",
				"Analitik verilerini görüntüleme",
				"SLA raporlarını görüntüleme",
			},
			"restrictions": []string{
				"Hiçbir değişiklik yapamaz",
				"Hedef ekleyemez, düzenleyemez veya silemez",
				"Uyarıları kapatamaz",
				"Rapor oluşturamaz",
				"Kullanıcı yönetimi yapamaz",
				"Sistem ayarlarına erişemez",
			},
		},
	}
}
