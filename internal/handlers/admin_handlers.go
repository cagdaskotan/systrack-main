package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func serveLanguageFile(c *gin.Context, basePath string) {
	// URL'den lang parametresini al
	lang := c.Query("lang")
	baseDir := "./static/admin/"

	// İngilizce ise admin_en klasöründen yükle
	if lang == "en" {
		baseDir = "./static/admin_en/"
	}
	if lang == "de" {
		baseDir = "./static/admin_de/"
	}

	c.File(baseDir + basePath)
}

type DashboardNotification struct {
	ID         int
	Type       string
	Channel    string
	Priority   string
	Title      sql.NullString
	Message    sql.NullString
	Recipients sql.NullString
	Status     string
	CreatedAt  time.Time
	SentAt     sql.NullTime
}

type DashboardTarget struct {
	ID             int
	Name           string
	Address        string
	MonitoringType string
	Type           string
	Status         string
	CreatedAt      time.Time
	LastCheck      time.Time
}

func AdminDashboard(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "dashboard.html")
	}
}

func AdminUsers(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "user_management.html")
	}
}

func AdminModulePermissions(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "module_permissions.html")
	}
}

func AdminTargets(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "targets.html")
	}
}

func AdminSensors(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "sensors.html")
	}
}

func AdminNotificationRules(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Notification rules HTML'i döndür
		c.Header("Content-Type", "text/html")
		c.String(http.StatusOK, `
			<div class="p-6">
				<div class="mb-6">
					<h1 class="text-2xl font-bold text-white mb-2">Bildirim Kuralları</h1>
					<p class="text-gray-400">Otomatik bildirim kurallarını yönetin ve yapılandırın</p>
				</div>

				<!-- İstatistikler -->
				<div class="grid grid-cols-1 md:grid-cols-4 gap-6 mb-6">
					<div class="bg-gray-800 rounded-lg border border-gray-700 p-6">
						<div class="flex items-center">
							<div class="w-12 h-12 bg-blue-600 rounded-lg flex items-center justify-center mr-4">
								<svg class="w-6 h-6 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path>
								</svg>
							</div>
							<div>
								<p class="text-sm font-medium text-gray-400">Aktif Kurallar</p>
								<p class="text-2xl font-bold text-white" id="active-rules-count">0</p>
							</div>
						</div>
					</div>

					<div class="bg-gray-800 rounded-lg border border-gray-700 p-6">
						<div class="flex items-center">
							<div class="w-12 h-12 bg-green-600 rounded-lg flex items-center justify-center mr-4">
								<svg class="w-6 h-6 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
								</svg>
							</div>
							<div>
								<p class="text-sm font-medium text-gray-400">Bu Ay Tetiklenen</p>
								<p class="text-2xl font-bold text-white" id="monthly-triggers">0</p>
							</div>
						</div>
					</div>

					<div class="bg-gray-800 rounded-lg border border-gray-700 p-6">
						<div class="flex items-center">
							<div class="w-12 h-12 bg-yellow-600 rounded-lg flex items-center justify-center mr-4">
								<svg class="w-6 h-6 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path>
								</svg>
							</div>
							<div>
								<p class="text-sm font-medium text-gray-400">Başarı Oranı</p>
								<p class="text-2xl font-bold text-white" id="success-rate">0%</p>
							</div>
						</div>
					</div>

					<div class="bg-gray-800 rounded-lg border border-gray-700 p-6">
						<div class="flex items-center">
							<div class="w-12 h-12 bg-red-600 rounded-lg flex items-center justify-center mr-4">
								<svg class="w-6 h-6 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z"></path>
								</svg>
							</div>
							<div>
								<p class="text-sm font-medium text-gray-400">Hata Oranı</p>
								<p class="text-2xl font-bold text-white" id="error-rate">0%</p>
							</div>
						</div>
					</div>
				</div>

				<!-- Kurallar Listesi -->
				<div class="bg-gray-800 rounded-lg border border-gray-700">
					<div class="p-6 border-b border-gray-700">
						<div class="flex justify-between items-center">
							<h2 class="text-xl font-semibold text-white">Bildirim Kuralları</h2>
							<button onclick="openCreateRuleModal()" class="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg flex items-center">
								<svg class="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
								</svg>
								Yeni Kural
							</button>
						</div>
					</div>

					<div class="p-6">
						<div class="overflow-x-auto">
							<table class="w-full text-left">
								<thead>
									<tr class="border-b border-gray-700">
										<th class="pb-3 text-sm font-medium text-gray-400">Kural Adı</th>
										<th class="pb-3 text-sm font-medium text-gray-400">Tür</th>
										<th class="pb-3 text-sm font-medium text-gray-400">Kanal</th>
										<th class="pb-3 text-sm font-medium text-gray-400">Öncelik</th>
										<th class="pb-3 text-sm font-medium text-gray-400">Durum</th>
										<th class="pb-3 text-sm font-medium text-gray-400">Son Tetiklenme</th>
										<th class="pb-3 text-sm font-medium text-gray-400">İşlemler</th>
									</tr>
								</thead>
								<tbody id="rules-table-body">
									<!-- Kurallar buraya yüklenecek -->
								</tbody>
							</table>
						</div>
					</div>
				</div>
			</div>

			<!-- Yeni Kural Oluşturma Modal -->
			<div id="createRuleModal" class="fixed inset-0 bg-black bg-opacity-50 hidden z-50">
				<div class="flex items-center justify-center min-h-screen p-4">
					<div class="bg-gray-800 rounded-lg border border-gray-700 w-full max-w-2xl">
						<div class="p-6 border-b border-gray-700">
							<h3 class="text-xl font-semibold text-white">Yeni Bildirim Kuralı</h3>
						</div>
						
						<form id="createRuleForm" class="p-6 space-y-6">
							<div class="grid grid-cols-1 md:grid-cols-2 gap-6">
								<div>
									<label class="block text-sm font-medium text-gray-300 mb-2">Kural Adı</label>
									<input type="text" id="ruleName" class="w-full bg-gray-600 text-white px-3 py-2 rounded-lg border border-gray-500 focus:border-blue-500 focus:outline-none" required>
								</div>
								
								<div>
									<label class="block text-sm font-medium text-gray-300 mb-2">Bildirim Türü</label>
									<select id="ruleType" class="w-full bg-gray-600 text-white px-3 py-2 rounded-lg border border-gray-500 focus:border-blue-500 focus:outline-none" required>
										<option value="target_status_change">Hedef Durum Değişikliği</option>
										<option value="sla_violation">SLA İhlali</option>
										<option value="alert_opened">Uyarı Açıldı</option>
										<option value="alert_closed">Uyarı Kapandı</option>
										<option value="system_health">Sistem Sağlığı</option>
										<option value="daily_report">Günlük Rapor</option>
									</select>
								</div>
								
								<div>
									<label class="block text-sm font-medium text-gray-300 mb-2">Bildirim Kanalı</label>
									<select id="ruleChannel" class="w-full bg-gray-600 text-white px-3 py-2 rounded-lg border border-gray-500 focus:border-blue-500 focus:outline-none" required>
										<option value="email">Email</option>
										<option value="telegram">Telegram</option>
										<option value="whatsapp">WhatsApp</option>
										<option value="webhook">Webhook</option>
									</select>
								</div>
								
								<div>
									<label class="block text-sm font-medium text-gray-300 mb-2">Öncelik</label>
									<select id="rulePriority" class="w-full bg-gray-600 text-white px-3 py-2 rounded-lg border border-gray-500 focus:border-blue-500 focus:outline-none" required>
										<option value="low">Düşük</option>
										<option value="medium">Orta</option>
										<option value="high">Yüksek</option>
										<option value="critical">Kritik</option>
									</select>
								</div>
							</div>
							
							<div>
								<label class="block text-sm font-medium text-gray-300 mb-2">Açıklama</label>
								<textarea id="ruleDescription" class="w-full bg-gray-600 text-white px-3 py-2 rounded-lg border border-gray-500 focus:border-blue-500 focus:outline-none" rows="3"></textarea>
							</div>
							
							<div>
								<label class="block text-sm font-medium text-gray-300 mb-2">Alıcılar (Email adresleri, virgülle ayırın)</label>
								<input type="text" id="ruleRecipients" class="w-full bg-gray-600 text-white px-3 py-2 rounded-lg border border-gray-500 focus:border-blue-500 focus:outline-none" placeholder="admin@example.com, user@example.com" required>
							</div>
							
							<div>
								<label class="block text-sm font-medium text-gray-300 mb-2">Cooldown (Dakika)</label>
								<input type="number" id="ruleCooldown" class="w-full bg-gray-600 text-white px-3 py-2 rounded-lg border border-gray-500 focus:border-blue-500 focus:outline-none" min="0" value="5">
							</div>
							
							<div class="flex justify-end space-x-4">
								<button type="button" onclick="closeCreateRuleModal()" class="px-4 py-2 text-gray-400 hover:text-white">İptal</button>
								<button type="submit" class="bg-blue-600 hover:bg-blue-700 text-white px-6 py-2 rounded-lg">Kural Oluştur</button>
							</div>
						</form>
					</div>
				</div>
			</div>
		`)
	}
}

// AdminReporting renders the Reporting page
func AdminReporting(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "reporting.html")
	}
}

func AdminUserForm(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Query("id")
		c.HTML(http.StatusOK, "user_form.html", gin.H{
			"userID": userID,
		})
	}
}

func AdminTargetForm(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "partials/target_form.html")
	}
}

// AdminNotifications bildirim ayarları sayfasını render eder
func AdminNotifications(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "new_notifications.html")
	}
}

// AdminNotificationSettings bildirim ayarlari sayfasini render eder
func AdminNotificationSettings(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "notification_settings.html")
	}
}

// AdminBackup yedekleme sayfasını render eder
func AdminBackup(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "backup.html")
	}
}

// AdminNetworkSettings renders the network settings page
func AdminNetworkSettings(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "network_settings.html")
	}
}

// AdminNewNotificationTemplates renders new templates management page
func AdminNewNotificationTemplates(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "new_notification_templates.html")
	}
}

// AdminIPScanner renders the IP scanner page
func AdminIPScanner(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "ip_scanner.html")
	}
}

// AdminInventory renders the inventory page
func AdminInventory(db interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveLanguageFile(c, "inventory.html")
	}
}

func AdminSSHTerminal(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetIDStr := c.Query("target_id")
		if targetIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target_id gerekli"})
			return
		}

		targetID, err := strconv.Atoi(targetIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz target_id"})
			return
		}

		// Target bilgilerini al
		var targetAddr string
		err = db.QueryRow("SELECT address FROM targets WHERE id = ?", targetID).Scan(&targetAddr)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Hedef bulunamadı"})
			return
		}

		// SSH terminal sayfasını serve et
		c.Header("Content-Type", "text/html; charset=utf-8")
		serveLanguageFile(c, "ssh_terminal.html")
	}
}
