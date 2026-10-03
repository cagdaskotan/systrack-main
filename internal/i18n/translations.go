package i18n

// Translation keys for Turkish and English
type Translations struct {
	// Common
	Edit   string
	Delete string
	Save   string
	Cancel string
	Close  string
	Search string
	Filter string
	Export string
	Import string
	Add    string
	Update string
	Create string
	Remove string

	// Dashboard
	NoUsersFound           string
	NoNotificationsFound   string
	NoOnlineTargetsFound   string
	NoOfflineTargetsFound  string
	Dashboard              string
	Statistics             string
	RecentActivity         string
	SystemHealth           string

	// Targets
	Targets                string
	OnlineTargets          string
	OfflineTargets         string
	AllTargets             string
	TargetName             string
	IPAddress              string
	Status                 string
	LastCheck              string
	ResponseTime           string

	// Users
	Users                  string
	UserManagement         string
	Email                  string
	Role                   string
	CreatedAt              string
	Actions                string
	Admin                  string
	User                   string

	// Notifications
	Notifications          string
	NotificationHistory    string
	NotificationSettings   string
	Type                   string
	Message                string
	CreatedDate            string
	ReadStatus             string

	// Time
	Seconds                string
	Minutes                string
	Hours                  string
	Days                   string
	Weeks                  string
	Months                 string
	Years                  string

	// Status
	Online                 string
	Offline                string
	Active                 string
	Inactive               string
	Success                string
	Failed                 string
	Pending                string
	Sent                   string
	Open                   string
	Closed                 string
	High                   string
	Low                    string

	// Monitoring Types
	HTTPMonitoring         string
	HTTPSMonitoring        string
	PingMonitoring         string
	GeneralMonitoring      string

	// Notification Types
	SLAViolation           string
	AlertOpened            string
	AlertClosed            string
	SystemHealthNotification string
	DailyReport            string

	// Health Status
	Excellent              string
	VeryGood               string
	Good                   string
	Fair                   string
	Critical               string
	AllSystemsRunning      string
	HighStability          string
	StableWithMinorIssues  string
	SlowdownOrErrors       string
	CriticalNeedsAttention string

	// History Titles
	HTTPHistory            string
	HTTPSHistory           string
	PingHistory            string
	MonitoringHistory      string

	// Placeholders
	SearchTargetPlaceholder string
	TargetNamePlaceholder   string
	AddressPlaceholder      string
	TagsPlaceholder         string
	SLAThresholdTitle       string

	// Buttons & Actions
	EditTarget             string
	UpdateButton           string

	// Error Messages
	InvalidTargetID        string
	TargetNotFound         string
	TargetDataLoadError    string

	// Page Titles
	TelegramSettingsTitle  string
}

var TR = Translations{
	// Common
	Edit:   "Düzenle",
	Delete: "Sil",
	Save:   "Kaydet",
	Cancel: "İptal",
	Close:  "Kapat",
	Search: "Ara",
	Filter: "Filtrele",
	Export: "Dışa Aktar",
	Import: "İçe Aktar",
	Add:    "Ekle",
	Update: "Güncelle",
	Create: "Oluştur",
	Remove: "Kaldır",

	// Dashboard
	NoUsersFound:           "Henüz kullanıcı bulunmuyor.",
	NoNotificationsFound:   "Henüz bildirim bulunmuyor.",
	NoOnlineTargetsFound:   "Henüz online hedef bulunmuyor.",
	NoOfflineTargetsFound:  "Henüz offline hedef bulunmuyor.",
	Dashboard:              "Kontrol Paneli",
	Statistics:             "İstatistikler",
	RecentActivity:         "Son Aktiviteler",
	SystemHealth:           "Sistem Sağlığı",

	// Targets
	Targets:                "Hedefler",
	OnlineTargets:          "Çevrimiçi Hedefler",
	OfflineTargets:         "Çevrimdışı Hedefler",
	AllTargets:             "Tüm Hedefler",
	TargetName:             "Hedef Adı",
	IPAddress:              "IP Adresi",
	Status:                 "Durum",
	LastCheck:              "Son Kontrol",
	ResponseTime:           "Yanıt Süresi",

	// Users
	Users:                  "Kullanıcılar",
	UserManagement:         "Kullanıcı Yönetimi",
	Email:                  "E-posta",
	Role:                   "Rol",
	CreatedAt:              "Oluşturulma Tarihi",
	Actions:                "İşlemler",
	Admin:                  "Yönetici",
	User:                   "Kullanıcı",

	// Notifications
	Notifications:          "Bildirimler",
	NotificationHistory:    "Bildirim Geçmişi",
	NotificationSettings:   "Bildirim Ayarları",
	Type:                   "Tür",
	Message:                "Mesaj",
	CreatedDate:            "Oluşturulma Tarihi",
	ReadStatus:             "Okunma Durumu",

	// Time
	Seconds:                "saniye",
	Minutes:                "dakika",
	Hours:                  "saat",
	Days:                   "gün",
	Weeks:                  "hafta",
	Months:                 "ay",
	Years:                  "yıl",

	// Status
	Online:                 "Çevrimiçi",
	Offline:                "Çevrimdışı",
	Active:                 "Aktif",
	Inactive:               "Pasif",
	Success:                "Başarılı",
	Failed:                 "Başarısız",
	Pending:                "Beklemede",
	Sent:                   "Gönderildi",
	Open:                   "Açık",
	Closed:                 "Kapalı",
	High:                   "Yüksek",
	Low:                    "Düşük",

	// Monitoring Types
	HTTPMonitoring:         "HTTP İzleme",
	HTTPSMonitoring:        "HTTPS İzleme",
	PingMonitoring:         "Ping İzleme",
	GeneralMonitoring:      "Genel İzleme",

	// Notification Types
	SLAViolation:           "SLA İhlali",
	AlertOpened:            "Uyarı Açıldı",
	AlertClosed:            "Uyarı Kapandı",
	SystemHealthNotification: "Sistem Sağlığı",
	DailyReport:            "Günlük Rapor",

	// Health Status
	Excellent:              "Mükemmel",
	VeryGood:               "Çok İyi",
	Good:                   "İyi",
	Fair:                   "Orta",
	Critical:               "Kritik",
	AllSystemsRunning:      "Tüm sistemler sorunsuz çalışıyor.",
	HighStability:          "Sistemler yüksek kararlılıkta çalışıyor.",
	StableWithMinorIssues:  "Genel durum stabil, küçük aksaklıklar olabilir.",
	SlowdownOrErrors:       "Bazı servislerde yavaşlama veya hatalar mevcut.",
	CriticalNeedsAttention: "Sistem sağlığı kritik, acil kontrol gerekli.",

	// History Titles
	HTTPHistory:            "HTTP Geçmişi",
	HTTPSHistory:           "HTTPS Geçmişi",
	PingHistory:            "Ping Geçmişi",
	MonitoringHistory:      "Monitoring Geçmişi",

	// Placeholders
	SearchTargetPlaceholder: "Hedef adı, IP veya etiket...",
	TargetNamePlaceholder:   "Örn: Web Sunucusu",
	AddressPlaceholder:      "Örn: 192.168.1.1 veya google.com",
	TagsPlaceholder:         "Örn: web,production,critical",
	SLAThresholdTitle:       "SLA Threshold değerini manuel olarak girin (0.1 - 100.0 arası)",

	// Buttons & Actions
	EditTarget:             "Hedef Düzenle",
	UpdateButton:           "Güncelle",

	// Error Messages
	InvalidTargetID:        "Geçersiz target_id",
	TargetNotFound:         "Hedef bulunamadı",
	TargetDataLoadError:    "Hedef verileri yüklenemedi",

	// Page Titles
	TelegramSettingsTitle:  "Telegram Ayarları - SysTrack",
}

var EN = Translations{
	// Common
	Edit:   "Edit",
	Delete: "Delete",
	Save:   "Save",
	Cancel: "Cancel",
	Close:  "Close",
	Search: "Search",
	Filter: "Filter",
	Export: "Export",
	Import: "Import",
	Add:    "Add",
	Update: "Update",
	Create: "Create",
	Remove: "Remove",

	// Dashboard
	NoUsersFound:           "No users found yet.",
	NoNotificationsFound:   "No notifications found yet.",
	NoOnlineTargetsFound:   "No online targets found yet.",
	NoOfflineTargetsFound:  "No offline targets found yet.",
	Dashboard:              "Dashboard",
	Statistics:             "Statistics",
	RecentActivity:         "Recent Activity",
	SystemHealth:           "System Health",

	// Targets
	Targets:                "Targets",
	OnlineTargets:          "Online Targets",
	OfflineTargets:         "Offline Targets",
	AllTargets:             "All Targets",
	TargetName:             "Target Name",
	IPAddress:              "IP Address",
	Status:                 "Status",
	LastCheck:              "Last Check",
	ResponseTime:           "Response Time",

	// Users
	Users:                  "Users",
	UserManagement:         "User Management",
	Email:                  "Email",
	Role:                   "Role",
	CreatedAt:              "Created At",
	Actions:                "Actions",
	Admin:                  "Admin",
	User:                   "User",

	// Notifications
	Notifications:          "Notifications",
	NotificationHistory:    "Notification History",
	NotificationSettings:   "Notification Settings",
	Type:                   "Type",
	Message:                "Message",
	CreatedDate:            "Created Date",
	ReadStatus:             "Read Status",

	// Time
	Seconds:                "seconds",
	Minutes:                "minutes",
	Hours:                  "hours",
	Days:                   "days",
	Weeks:                  "weeks",
	Months:                 "months",
	Years:                  "years",

	// Status
	Online:                 "Online",
	Offline:                "Offline",
	Active:                 "Active",
	Inactive:               "Inactive",
	Success:                "Success",
	Failed:                 "Failed",
	Pending:                "Pending",
	Sent:                   "Sent",
	Open:                   "Open",
	Closed:                 "Closed",
	High:                   "High",
	Low:                    "Low",

	// Monitoring Types
	HTTPMonitoring:         "HTTP Monitoring",
	HTTPSMonitoring:        "HTTPS Monitoring",
	PingMonitoring:         "Ping Monitoring",
	GeneralMonitoring:      "General Monitoring",

	// Notification Types
	SLAViolation:           "SLA Violation",
	AlertOpened:            "Alert Opened",
	AlertClosed:            "Alert Closed",
	SystemHealthNotification: "System Health",
	DailyReport:            "Daily Report",

	// Health Status
	Excellent:              "Excellent",
	VeryGood:               "Very Good",
	Good:                   "Good",
	Fair:                   "Fair",
	Critical:               "Critical",
	AllSystemsRunning:      "All systems running smoothly.",
	HighStability:          "Systems running with high stability.",
	StableWithMinorIssues:  "Overall stable, minor issues may occur.",
	SlowdownOrErrors:       "Some services experiencing slowdown or errors.",
	CriticalNeedsAttention: "System health critical, urgent attention required.",

	// History Titles
	HTTPHistory:            "HTTP History",
	HTTPSHistory:           "HTTPS History",
	PingHistory:            "Ping History",
	MonitoringHistory:      "Monitoring History",

	// Placeholders
	SearchTargetPlaceholder: "Target name, IP or tag...",
	TargetNamePlaceholder:   "e.g: Web Server",
	AddressPlaceholder:      "e.g: 192.168.1.1 or google.com",
	TagsPlaceholder:         "e.g: web,production,critical",
	SLAThresholdTitle:       "Enter SLA Threshold value manually (between 0.1 - 100.0)",

	// Buttons & Actions
	EditTarget:             "Edit Target",
	UpdateButton:           "Update",

	// Error Messages
	InvalidTargetID:        "Invalid target_id",
	TargetNotFound:         "Target not found",
	TargetDataLoadError:    "Failed to load target data",

	// Page Titles
	TelegramSettingsTitle:  "Telegram Settings - SysTrack",
}

// GetTranslations returns the appropriate translation based on language code
func GetTranslations(lang string) Translations {
	if lang == "en" {
		return EN
	}
	return TR
}
