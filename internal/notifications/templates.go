package notifications

import (
	"bytes"
	"html/template"
)

// EmailTemplateManager email sablon yoneticisi
type EmailTemplateManager struct {
	templates map[NotificationType]string
}

// NewEmailTemplateManager yeni sablon yoneticisi olustur
func NewEmailTemplateManager() *EmailTemplateManager {
	tm := &EmailTemplateManager{
		templates: make(map[NotificationType]string),
	}
	tm.loadTemplates()
	return tm
}

// loadTemplates tum sablonlari yukle
func (tm *EmailTemplateManager) loadTemplates() {
	tm.templates[TargetStatusChange] = tm.getTargetStatusTemplate()
	tm.templates[SLAViolation] = tm.getSLAViolationTemplate()
	tm.templates[AlertOpened] = tm.getAlertOpenedTemplate()
	tm.templates[AlertClosed] = tm.getAlertClosedTemplate()
	tm.templates[SystemHealth] = tm.getSystemHealthTemplate()
	tm.templates[DailyReport] = tm.getDailyReportTemplate()
}

// GetBulkTemplate bulk notification icin template dondur
func (tm *EmailTemplateManager) GetBulkTemplate(notificationType NotificationType) string {
	switch notificationType {
	case TargetStatusChange:
		return tm.getBulkTargetStatusTemplate()
	case SLAViolation:
		return tm.getBulkSLAViolationTemplate()
	default:
		return tm.getBulkTargetStatusTemplate()
	}
}

// GenerateHTML sablon turune gore HTML olustur
func (tm *EmailTemplateManager) GenerateHTML(notification *Notification, data *NotificationData) (string, error) {
	templateStr, exists := tm.templates[notification.Type]
	if !exists {
		// Varsayilan sablon
		templateStr = tm.getDefaultTemplate()
	}

	// Template verisi
	templateData := map[string]interface{}{
		"Title":         notification.Title,
		"Message":       notification.Message,
		"Type":          notification.Type,
		"Priority":      notification.Priority,
		"Timestamp":     notification.CreatedAt.Format("2006-01-02 15:04:05"),
		"Status":        notification.Status,
		"TargetName":    data.TargetName,
		"TargetURL":     data.TargetURL,
		"ResponseTime":  data.ResponseTime,
		"UptimePercent": data.UptimePercent,
		"AlertMessage":  data.AlertMessage,
		"AlertLevel":    data.AlertLevel,
		"SLATarget":     data.SLATarget,
		"SLABreach":     data.SLABreach,
	}

	tmpl, err := template.New("email").Funcs(template.FuncMap{
		"sub": func(a, b interface{}) float64 {
			aVal, ok := a.(float64)
			if !ok {
				return 0
			}
			bVal, ok := b.(float64)
			if !ok {
				return 0
			}
			return aVal - bVal
		},
	}).Parse(templateStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, templateData)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// getTargetStatusTemplate hedef durum sablonu
func (tm *EmailTemplateManager) getTargetStatusTemplate() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
</head>
<body style="margin:0;padding:24px;background:#0f172a;font-family:'Segoe UI',Arial,sans-serif;color:#0f172a;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        <tr>
            <td align="center">
                <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:640px;border-radius:24px;background:#ffffff;overflow:hidden;box-shadow:0 20px 50px rgba(15,23,42,0.25);">
                    <tr>
                        <td style="padding:30px;background:linear-gradient(135deg,#2563eb,#7c3aed);color:#ffffff;">
                            <div style="text-transform:uppercase;letter-spacing:0.18em;font-size:12px;opacity:0.8;">Hedef Durumu</div>
                            <div style="font-size:26px;font-weight:600;margin:10px 0;">{{.Title}}</div>
                            <div style="opacity:0.9;font-size:14px;line-height:1.6;">{{.Message}}</div>
                            <div style="margin-top:16px;display:inline-block;padding:6px 14px;border:1px solid rgba(255,255,255,0.5);border-radius:999px;font-size:12px;letter-spacing:0.14em;">{{.Timestamp}}</div>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:28px;">
                            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-collapse:separate;border-spacing:0 10px;">
                                <tr>
                                    <td style="padding:16px;border-radius:16px;border:1px solid #e5e7eb;background:linear-gradient(180deg,#f8fafc,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#94a3b8;margin-bottom:6px;">Hedef</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;">{{.TargetName}}</div>
                                    </td>
                                    <td style="padding:16px;border-radius:16px;border:1px solid #e5e7eb;background:linear-gradient(180deg,#f8fafc,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#94a3b8;margin-bottom:6px;">Adres</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;">{{.TargetURL}}</div>
                                    </td>
                                </tr>
                                <tr>
                                    <td style="padding:16px;border-radius:16px;border:1px solid #e5e7eb;background:linear-gradient(180deg,#f8fafc,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#94a3b8;margin-bottom:6px;">Uptime</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;">{{printf "%.1f" .UptimePercent}}%</div>
                                    </td>
                                    <td style="padding:16px;border-radius:16px;border:1px solid #e5e7eb;background:linear-gradient(180deg,#f8fafc,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#94a3b8;margin-bottom:6px;">Yanıt</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;">{{printf "%.0f" .ResponseTime}} ms</div>
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:24px;background:#f8fafc;text-align:center;font-size:12px;color:#94a3b8;">Bu bildirim SysTrack tarafından otomatik olarak gönderildi.</td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`
}

// getSLAViolationTemplate SLA ihlali sablonu
func (tm *EmailTemplateManager) getSLAViolationTemplate() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
</head>
<body style="margin:0;padding:24px;background:#0f172a;font-family:'Segoe UI',Arial,sans-serif;color:#0f172a;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        <tr>
            <td align="center">
                <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:660px;border-radius:26px;overflow:hidden;background:#ffffff;box-shadow:0 25px 55px rgba(15,23,42,0.25);">
                    <tr>
                        <td style="padding:34px;background:linear-gradient(135deg,#ef4444,#f97316);color:#ffffff;">
                            <div style="text-transform:uppercase;letter-spacing:0.2em;font-size:11px;opacity:0.8;">SLA Bildirimi</div>
                            <div style="font-size:26px;font-weight:600;margin:10px 0;">{{.Title}}</div>
                            <div style="opacity:0.92;font-size:14px;line-height:1.6;margin:0;">{{.Message}}</div>
                            <div style="margin-top:16px;display:inline-block;padding:6px 14px;border:1px solid rgba(255,255,255,0.5);border-radius:999px;font-size:12px;letter-spacing:0.16em;">{{.Timestamp}}</div>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:28px;">
                            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-collapse:separate;border-spacing:0 12px;">
                                <tr>
                                    <td style="padding:14px 16px;border-radius:16px;border:1px solid #fecaca;background:linear-gradient(180deg,#fef2f2,#ffffff);">
                                        <div style="font-size:12px;color:#b91c1c;text-transform:uppercase;letter-spacing:0.15em;margin-bottom:6px;">Hedef</div>
                                        <div style="font-size:15px;font-weight:600;color:#7f1d1d;">{{.TargetName}}</div>
                                    </td>
                                    <td style="padding:14px 16px;border-radius:16px;border:1px solid #fecaca;background:linear-gradient(180deg,#fef2f2,#ffffff);">
                                        <div style="font-size:12px;color:#b91c1c;text-transform:uppercase;letter-spacing:0.15em;margin-bottom:6px;">Adres</div>
                                        <div style="font-size:15px;font-weight:600;color:#7f1d1d;">{{.TargetURL}}</div>
                                    </td>
                                </tr>
                                <tr>
                                    <td style="padding:14px 16px;border-radius:16px;border:1px solid #fecaca;background:linear-gradient(180deg,#fef2f2,#ffffff);">
                                        <div style="font-size:12px;color:#b91c1c;text-transform:uppercase;letter-spacing:0.15em;margin-bottom:6px;">Mevcut Uptime</div>
                                        <div style="font-size:15px;font-weight:600;color:#b91c1c;">{{printf "%.1f" .UptimePercent}}%</div>
                                    </td>
                                    <td style="padding:14px 16px;border-radius:16px;border:1px solid #fecaca;background:linear-gradient(180deg,#fef2f2,#ffffff);">
                                        <div style="font-size:12px;color:#b91c1c;text-transform:uppercase;letter-spacing:0.15em;margin-bottom:6px;">SLA Hedefi</div>
                                        <div style="font-size:15px;font-weight:600;color:#b91c1c;">{{printf "%.1f" .SLATarget}}%</div>
                                    </td>
                                </tr>
                                <tr>
                                    <td style="padding:14px 16px;border-radius:16px;border:1px solid #fecaca;background:linear-gradient(180deg,#fef2f2,#ffffff);">
                                        <div style="font-size:12px;color:#b91c1c;text-transform:uppercase;letter-spacing:0.15em;margin-bottom:6px;">Ortalama Yanıt</div>
                                        <div style="font-size:15px;font-weight:600;color:#b91c1c;">{{printf "%.2f" .ResponseTime}} ms</div>
                                    </td>
                                    <td style="padding:14px 16px;border-radius:16px;border:1px solid #fecaca;background:linear-gradient(180deg,#fef2f2,#ffffff);">
                                        <div style="font-size:12px;color:#b91c1c;text-transform:uppercase;letter-spacing:0.15em;margin-bottom:6px;">Fark</div>
                                        <div style="font-size:15px;font-weight:600;color:#b91c1c;">{{if .SLABreach}}-{{printf "%.1f" (sub .SLATarget .UptimePercent)}}%{{else}}+{{printf "%.1f" (sub .UptimePercent .SLATarget)}}%{{end}}</div>
                                    </td>
                                </tr>
                            </table>
                            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin-top:18px;border:1px dashed #fecaca;border-radius:14px;">
                                <tr>
                                    <td style="padding:16px;font-size:13px;color:#7f1d1d;line-height:1.6;">
                                        <strong style="display:block;margin-bottom:6px;">Olası Aksiyonlar</strong>
                                        <div>• Hedef servis kaynaklarını kontrol edin.</div>
                                        <div>• Ağ gecikmelerini ve arızaları doğrulayın.</div>
                                        <div>• Gerekiyorsa yedek altyapıya geçiş yapın.</div>
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:22px;background:#fef2f2;text-align:center;font-size:12px;color:#b91c1c;">Bu bildirim SysTrack tarafından otomatik olarak gönderildi.</td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`
}

// getAlertOpenedTemplate yeni uyari sablonu
func (tm *EmailTemplateManager) getAlertOpenedTemplate() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
</head>
<body style="margin:0;padding:24px;background:#0f172a;font-family:'Segoe UI',Arial,sans-serif;color:#0f172a;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        <tr>
            <td align="center">
                <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:620px;background:#ffffff;border-radius:24px;overflow:hidden;box-shadow:0 25px 55px rgba(15,23,42,0.25);">
                    <tr>
                        <td style="padding:30px;background:linear-gradient(135deg,#f97316,#ec4899);color:#ffffff;">
                            <div style="text-transform:uppercase;letter-spacing:0.2em;font-size:11px;opacity:0.8;">Uyari Kaydi</div>
                            <div style="font-size:24px;font-weight:600;margin:8px 0;">{{.Title}}</div>
                            <div style="opacity:0.9;font-size:14px;line-height:1.6;margin:0;">{{.Message}}</div>
                            <div style="margin-top:14px;display:inline-block;padding:6px 13px;border:1px solid rgba(255,255,255,0.45);border-radius:999px;font-size:12px;letter-spacing:0.14em;">{{.Timestamp}}</div>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:26px;">
                            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-collapse:separate;border-spacing:0 10px;">
                                <tr>
                                    <td style="padding:14px;border-radius:14px;border:1px solid #fed7aa;background:linear-gradient(180deg,#fff7ed,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#c14d0f;margin-bottom:5px;">Hedef</div>
                                        <div style="font-size:15px;font-weight:600;color:#be123c;">{{.TargetName}}</div>
                                    </td>
                                    <td style="padding:14px;border-radius:14px;border:1px solid #fed7aa;background:linear-gradient(180deg,#fff7ed,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#c14d0f;margin-bottom:5px;">Seviye</div>
                                        <div style="font-size:15px;font-weight:600;color:#be123c;">{{.AlertLevel}}</div>
                                    </td>
                                </tr>
                                <tr>
                                    <td style="padding:14px;border-radius:14px;border:1px solid #fed7aa;background:linear-gradient(180deg,#fff7ed,#ffffff);" colspan="2">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#c14d0f;margin-bottom:5px;">Mesaj</div>
                                        <div style="font-size:14px;font-weight:500;color:#7c2d12;">{{.AlertMessage}}</div>
                                    </td>
                                </tr>
                                <tr>
                                    <td style="padding:14px;border-radius:14px;border:1px solid #fed7aa;background:linear-gradient(180deg,#fff7ed,#ffffff);" colspan="2">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#c14d0f;margin-bottom:5px;">Durum</div>
                                        <div style="font-size:15px;font-weight:600;color:#7c2d12;">{{.Status}}</div>
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:22px;background:#fff7ed;text-align:center;font-size:12px;color:#be123c;">Bu bildirim SysTrack tarafindan otomatik olarak gonderildi.</td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`
}

// getAlertClosedTemplate uyari kapanan sablonu
func (tm *EmailTemplateManager) getAlertClosedTemplate() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
</head>
<body style="margin:0;padding:24px;background:#0f172a;font-family:'Segoe UI',Arial,sans-serif;color:#0f172a;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        <tr>
            <td align="center">
                <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:620px;background:#ffffff;border-radius:24px;overflow:hidden;box-shadow:0 25px 55px rgba(15,23,42,0.25);">
                    <tr>
                        <td style="padding:30px;background:linear-gradient(135deg,#22c55e,#0f9d58);color:#ffffff;">
                            <div style="text-transform:uppercase;letter-spacing:0.2em;font-size:11px;opacity:0.8;">Uyari Guncellemesi</div>
                            <div style="font-size:24px;font-weight:600;margin:8px 0;">{{.Title}}</div>
                            <div style="opacity:0.9;font-size:14px;line-height:1.6;margin:0;">{{.Message}}</div>
                            <div style="margin-top:14px;display:inline-block;padding:6px 13px;border:1px solid rgba(255,255,255,0.45);border-radius:999px;font-size:12px;letter-spacing:0.14em;">{{.Timestamp}}</div>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:26px;">
                            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-collapse:separate;border-spacing:0 10px;">
                                <tr>
                                    <td style="padding:14px;border-radius:14px;border:1px solid #bbf7d0;background:linear-gradient(180deg,#ecfee8,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#15803d;margin-bottom:5px;">Hedef</div>
                                        <div style="font-size:15px;font-weight:600;color:#14532d;">{{.TargetName}}</div>
                                    </td>
                                    <td style="padding:14px;border-radius:14px;border:1px solid #bbf7d0;background:linear-gradient(180deg,#ecfee8,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#15803d;margin-bottom:5px;">Seviye</div>
                                        <div style="font-size:15px;font-weight:600;color:#14532d;">{{.AlertLevel}}</div>
                                    </td>
                                </tr>
                                <tr>
                                    <td style="padding:14px;border-radius:14px;border:1px solid #bbf7d0;background:linear-gradient(180deg,#ecfee8,#ffffff);" colspan="2">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#15803d;margin-bottom:5px;">Mesaj</div>
                                        <div style="font-size:14px;font-weight:500;color:#0f5132;">{{.AlertMessage}}</div>
                                    </td>
                                </tr>
                                <tr>
                                    <td style="padding:14px;border-radius:14px;border:1px solid #bbf7d0;background:linear-gradient(180deg,#ecfee8,#ffffff);" colspan="2">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#15803d;margin-bottom:5px;">Kapanma</div>
                                        <div style="font-size:15px;font-weight:600;color:#0f5132;">{{.Timestamp}}</div>
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:22px;background:#ecfee8;text-align:center;font-size:12px;color:#166534;">Bu bildirim SysTrack tarafindan otomatik olarak gonderildi.</td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`
}

// getSystemHealthTemplate sistem saglik sablonu
func (tm *EmailTemplateManager) getSystemHealthTemplate() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
</head>
<body style="margin:0;padding:24px;background:#0f172a;font-family:'Segoe UI',Arial,sans-serif;color:#0f172a;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        <tr>
            <td align="center">
                <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:620px;background:#ffffff;border-radius:24px;overflow:hidden;box-shadow:0 25px 55px rgba(15,23,42,0.25);">
                    <tr>
                        <td style="padding:32px;background:linear-gradient(135deg,#0ea5e9,#2563eb);color:#ffffff;">
                            <div style="text-transform:uppercase;letter-spacing:0.2em;font-size:11px;opacity:0.8;">Sistem Saglik Ozeti</div>
                            <div style="font-size:24px;font-weight:600;margin:8px 0;">{{.Title}}</div>
                            <div style="opacity:0.9;font-size:14px;line-height:1.6;margin:0;">{{.Message}}</div>
                            <div style="margin-top:14px;display:inline-block;padding:6px 13px;border:1px solid rgba(255,255,255,0.45);border-radius:999px;font-size:12px;letter-spacing:0.14em;">{{.Timestamp}}</div>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:28px;">
                            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-collapse:separate;border-spacing:0 12px;">
                                <tr>
                                    <td style="padding:16px;border-radius:16px;border:1px solid #bae6fd;background:linear-gradient(180deg,#e0f2fe,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#0369a1;margin-bottom:6px;">Genel Durum</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;">{{.Status}}</div>
                                    </td>
                                    <td style="padding:16px;border-radius:16px;border:1px solid #bae6fd;background:linear-gradient(180deg,#e0f2fe,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#0369a1;margin-bottom:6px;">Ortalama Uptime</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;">{{printf "%.1f" .UptimePercent}}%</div>
                                    </td>
                                </tr>
                                <tr>
                                    <td style="padding:16px;border-radius:16px;border:1px solid #bae6fd;background:linear-gradient(180deg,#e0f2fe,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#0369a1;margin-bottom:6px;">Ortalama Yanit</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;">{{printf "%.2f" .ResponseTime}} ms</div>
                                    </td>
                                    <td style="padding:16px;border-radius:16px;border:1px solid #bae6fd;background:linear-gradient(180deg,#e0f2fe,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#0369a1;margin-bottom:6px;">Rapor Zaman?</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;">{{.Timestamp}}</div>
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:22px;background:#e0f2fe;text-align:center;font-size:12px;color:#0369a1;">Bu bildirim SysTrack tarafindan otomatik olarak gonderildi.</td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`
}

// getDailyReportTemplate gunluk rapor sablonu
func (tm *EmailTemplateManager) getDailyReportTemplate() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
</head>
<body style="margin:0;padding:24px;background:#0f172a;font-family:'Segoe UI',Arial,sans-serif;color:#0f172a;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        <tr>
            <td align="center">
                <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:620px;background:#ffffff;border-radius:24px;overflow:hidden;box-shadow:0 25px 55px rgba(15,23,42,0.25);">
                    <tr>
                        <td style="padding:32px;background:linear-gradient(135deg,#7c3aed,#9333ea);color:#ffffff;">
                            <div style="text-transform:uppercase;letter-spacing:0.2em;font-size:11px;opacity:0.8;">Gunluk Ozet</div>
                            <div style="font-size:24px;font-weight:600;margin:8px 0;">{{.Title}}</div>
                            <div style="opacity:0.9;font-size:14px;line-height:1.6;margin:0;">{{.Message}}</div>
                            <div style="margin-top:14px;display:inline-block;padding:6px 13px;border:1px solid rgba(255,255,255,0.45);border-radius:999px;font-size:12px;letter-spacing:0.14em;">{{.Timestamp}}</div>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:28px;">
                            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-collapse:separate;border-spacing:0 12px;">
                                <tr>
                                    <td style="padding:16px;border-radius:16px;border:1px solid #d8b4fe;background:linear-gradient(180deg,#f5f3ff,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#5b21b6;margin-bottom:6px;">Rapor Tarihi</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;">{{.Timestamp}}</div>
                                    </td>
                                    <td style="padding:16px;border-radius:16px;border:1px solid #d8b4fe;background:linear-gradient(180deg,#f5f3ff,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#5b21b6;margin-bottom:6px;">Genel Durum</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;">{{.Status}}</div>
                                    </td>
                                </tr>
                                <tr>
                                    <td style="padding:16px;border-radius:16px;border:1px solid #d8b4fe;background:linear-gradient(180deg,#f5f3ff,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#5b21b6;margin-bottom:6px;">Genel Uptime</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;">{{printf "%.1f" .UptimePercent}}%</div>
                                    </td>
                                    <td style="padding:16px;border-radius:16px;border:1px solid #d8b4fe;background:linear-gradient(180deg,#f5f3ff,#ffffff);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.12em;color:#5b21b6;margin-bottom:6px;">Yanıt Ortalaması</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;">{{printf "%.2f" .ResponseTime}} ms</div>
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:22px;background:#f5f3ff;text-align:center;font-size:12px;color:#5b21b6;">Bu bildirim SysTrack tarafından otomatik olarak gönderildi.</td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`
}

// getDefaultTemplate varsayilan sablon
func (tm *EmailTemplateManager) getDefaultTemplate() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
</head>
<body>
    <table role="presentation" cellpadding="0" cellspacing="0" width="100%" style="margin:0;padding:32px 12px;background:#0f172a;font-family:'Segoe UI',Arial,sans-serif;color:#0f172a;">
        <tr>
            <td align="center">
                <table role="presentation" cellpadding="0" cellspacing="0" width="100%" style="max-width:660px;background:#ffffff;border-radius:28px;overflow:hidden;box-shadow:0 30px 60px rgba(15,23,42,0.25);">
                    <tr>
                        <td style="padding:36px 32px;background:linear-gradient(135deg,#475569,#0f172a);color:#ffffff;">
                            <div style="text-transform:uppercase;letter-spacing:0.28em;font-size:11px;opacity:0.8;margin-bottom:12px;">Bildirim</div>
                            <div style="font-size:26px;font-weight:600;margin:0 0 8px;">{{.Title}}</div>
                            <div style="opacity:0.9;font-size:15px;line-height:1.6;margin:0;">{{.Message}}</div>
                            <div style="margin-top:18px;display:inline-block;padding:6px 14px;border:1px solid rgba(255,255,255,0.5);border-radius:999px;font-size:12px;letter-spacing:0.18em;">{{.Timestamp}}</div>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:32px;background-color:#ffffff;">
                            <table role="presentation" cellpadding="0" cellspacing="0" width="100%" style="border-collapse:separate;border-spacing:0 12px;">
                                <tr>
                                    <td style="width:33.33%;padding:16px;border:1px solid #e5e7eb;border-radius:18px;background:linear-gradient(180deg,#f8fafc 0%,#ffffff 100%);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.14em;color:#94a3b8;margin-bottom:6px;">Tur</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;margin:0;">{{.Type}}</div>
                                    </td>
                                    <td style="width:33.33%;padding:16px;border:1px solid #e5e7eb;border-radius:18px;background:linear-gradient(180deg,#f8fafc 0%,#ffffff 100%);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.14em;color:#94a3b8;margin-bottom:6px;">Oncelik</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;margin:0;">{{.Priority}}</div>
                                    </td>
                                    <td style="width:33.33%;padding:16px;border:1px solid #e5e7eb;border-radius:18px;background:linear-gradient(180deg,#f8fafc 0%,#ffffff 100%);">
                                        <div style="font-size:12px;text-transform:uppercase;letter-spacing:0.14em;color:#94a3b8;margin-bottom:6px;">Durum</div>
                                        <div style="font-size:16px;font-weight:600;color:#0f172a;margin:0;">{{.Status}}</div>
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:24px 32px;background:#f8fafc;text-align:center;font-size:12px;color:#94a3b8;">
                            Bu bildirim SysTrack tarafindan otomatik olarak gonderildi.
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`
}

// getBulkTargetStatusTemplate toplu hedef sablonu
func (tm *EmailTemplateManager) getBulkTargetStatusTemplate() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
</head>
<body style="margin:0;padding:24px;background:#0f172a;font-family:'Segoe UI',Arial,sans-serif;color:#0f172a;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        <tr>
            <td align="center">
                <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:700px;background:#ffffff;border-radius:24px;overflow:hidden;box-shadow:0 25px 55px rgba(15,23,42,0.25);">
                    <tr>
                        <td style="padding:32px;background:linear-gradient(135deg,#2563eb,#1d4ed8);color:#ffffff;">
                            <div style="text-transform:uppercase;letter-spacing:0.2em;font-size:11px;opacity:0.8;">Toplu Durum</div>
                            <div style="font-size:24px;font-weight:600;margin:8px 0;">{{.Title}}</div>
                            <div style="opacity:0.9;font-size:14px;line-height:1.6;margin:0;">{{.Message}}</div>
                            <div style="margin-top:14px;display:inline-block;padding:6px 13px;border:1px solid rgba(255,255,255,0.45);border-radius:999px;font-size:12px;letter-spacing:0.14em;">{{.Timestamp}}</div>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:24px;">
                            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin-bottom:16px;border-collapse:separate;border-spacing:0 10px;">
                                <tr>
                                    <td style="padding:14px;border-radius:16px;border:1px solid #bfdbfe;background:linear-gradient(180deg,#e0f2ff,#ffffff);font-size:14px;font-weight:600;color:#0f172a;">Hedef Sayisi: {{.TargetCount}}</td>
                                    <td style="padding:14px;border-radius:16px;border:1px solid #bfdbfe;background:linear-gradient(180deg,#e0f2ff,#ffffff);font-size:14px;font-weight:600;color:#0f172a;">Rapor Zamani: {{.Timestamp}}</td>
                                </tr>
                            </table>
                            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;font-size:13px;">
                                <tr style="background:#0f172a;color:#ffffff;">
                                    <th style="padding:10px;text-align:left;">Hedef</th>
                                    <th style="padding:10px;text-align:left;">Adres</th>
                                    <th style="padding:10px;text-align:left;">Durum</th>
                                    <th style="padding:10px;text-align:left;">Uptime</th>
                                    <th style="padding:10px;text-align:left;">Yanıt</th>
                                </tr>
                                {{range .Targets}}
                                <tr style="background:#f8fafc;color:#0f172a;">
                                    <td style="padding:10px;border-bottom:1px solid #e5e7eb;">{{.Name}}</td>
                                    <td style="padding:10px;border-bottom:1px solid #e5e7eb;">{{.Address}}</td>
                                    <td style="padding:10px;border-bottom:1px solid #e5e7eb;">{{.Status}}</td>
                                    <td style="padding:10px;border-bottom:1px solid #e5e7eb;">{{if .UptimePercent}}{{printf "%.1f" .UptimePercent}}%{{else}}-{{end}}</td>
                                    <td style="padding:10px;border-bottom:1px solid #e5e7eb;">{{if .ResponseTime}}{{printf "%.0f" .ResponseTime}} ms{{else}}-{{end}}</td>
                                </tr>
                                {{end}}
                            </table>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:22px;background:#e0f2ff;text-align:center;font-size:12px;color:#1d4ed8;">Bu bildirim SysTrack tarafindan otomatik olarak gonderildi.</td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`
}

// getBulkSLAViolationTemplate toplu SLA sablonu
func (tm *EmailTemplateManager) getBulkSLAViolationTemplate() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
</head>
<body style="margin:0;padding:24px;background:#0f172a;font-family:'Segoe UI',Arial,sans-serif;color:#0f172a;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        <tr>
            <td align="center">
                <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:700px;background:#ffffff;border-radius:24px;overflow:hidden;box-shadow:0 25px 55px rgba(15,23,42,0.25);">
                    <tr>
                        <td style="padding:32px;background:linear-gradient(135deg,#dc2626,#ea580c);color:#ffffff;">
                            <div style="text-transform:uppercase;letter-spacing:0.2em;font-size:11px;opacity:0.8;">Toplu SLA</div>
                            <div style="font-size:24px;font-weight:600;margin:8px 0;">{{.Title}}</div>
                            <div style="opacity:0.9;font-size:14px;line-height:1.6;margin:0;">{{.Message}}</div>
                            <div style="margin-top:14px;display:inline-block;padding:6px 13px;border:1px solid rgba(255,255,255,0.45);border-radius:999px;font-size:12px;letter-spacing:0.14em;">{{.Timestamp}}</div>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:24px;">
                            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin-bottom:16px;border-collapse:separate;border-spacing:0 10px;">
                                <tr>
                                    <td style="padding:14px;border-radius:16px;border:1px solid #fecaca;background:linear-gradient(180deg,#fef2f2,#ffffff);font-size:14px;font-weight:600;color:#0f172a;">Etkilenen Hedef: {{.TargetCount}}</td>
                                    <td style="padding:14px;border-radius:16px;border:1px solid #fecaca;background:linear-gradient(180deg,#fef2f2,#ffffff);font-size:14px;font-weight:600;color:#0f172a;">Rapor Zamani: {{.Timestamp}}</td>
                                </tr>
                            </table>
                            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;font-size:13px;">
                                <tr style="background:#7f1d1d;color:#ffffff;">
                                    <th style="padding:10px;text-align:left;">Hedef</th>
                                    <th style="padding:10px;text-align:left;">Adres</th>
                                    <th style="padding:10px;text-align:left;">Durum</th>
                                    <th style="padding:10px;text-align:left;">Uptime</th>
                                    <th style="padding:10px;text-align:left;">Yanıt</th>
                                </tr>
                                {{range .Targets}}
                                <tr style="background:#fef2f2;color:#7f1d1d;">
                                    <td style="padding:10px;border-bottom:1px solid #fecaca;">{{.Name}}</td>
                                    <td style="padding:10px;border-bottom:1px solid #fecaca;">{{.Address}}</td>
                                    <td style="padding:10px;border-bottom:1px solid #fecaca;">{{.Status}}</td>
                                    <td style="padding:10px;border-bottom:1px solid #fecaca;">{{if .UptimePercent}}{{printf "%.1f" .UptimePercent}}%{{else}}-{{end}}</td>
                                    <td style="padding:10px;border-bottom:1px solid #fecaca;">{{if .ResponseTime}}{{printf "%.0f" .ResponseTime}} ms{{else}}-{{end}}</td>
                                </tr>
                                {{end}}
                            </table>
                            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin-top:18px;border:1px dashed #fecaca;border-radius:14px;">
                                <tr>
                                    <td style="padding:16px;font-size:13px;color:#7f1d1d;line-height:1.6;">
                                        <strong style="display:block;margin-bottom:6px;">Olas? Aksiyonlar</strong>
                                        <div>? Sistem kaynaklar?n? ve loglar? inceleyin.</div>
                                        <div>? A? gecikmelerini azaltmak i?in ?nlem al?n.</div>
                                        <div>? Gerekiyorsa yedek altyap?ya ge?i? yap?n.</div>
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding:22px;background:#fef2f2;text-align:center;font-size:12px;color:#7f1d1d;">Bu bildirim SysTrack tarafindan otomatik olarak gonderildi.</td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`
}
