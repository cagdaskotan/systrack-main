package handlers

import (
	"regexp"
	"strings"
)

// crackToolIndicatorPattern is the single source of truth for crack / license-bypass
// tool name fragments. It is embedded as a PowerShell regex literal into every detection
// block in api_crack_scan.go (services, tasks, installed software, startup, file traces,
// Defender exclusions, Amcache, event log) so coverage stays consistent across all of
// them instead of drifting via separately maintained copies.
const crackToolIndicatorPattern = `` +
	// Windows / Office KMS crack araçları
	`kmspico|kmsauto|auto\s*kms|aact|aact[ -]?network|re-?loader|reloader|` +
	`microsoft\s*toolkit|office\s*toolkit|kmseldi|kms\s*tools|kms\s*activat|` +
	`kms38|kms_vl_all|ratiborus|hwidgen|massgrave|mas[_-]?aio|tsforge|ohook|` +
	`1activator|garbee|consoleact|removewat|chew[ -]?wga|windows\s*loader|` +
	`daz\s*loader|vnext\s*kms|irontech|py-?kms|` +
	// JetBrains (IntelliJ, PyCharm, WebStorm, Rider, GoLand, CLion, vb.) crack
	// ja-netfilter: Java network proxy agent; ana IDE exe'si yamalanmaz,
	// bunun yerine .vmoptions'a -javaagent: satırı eklenir.
	`ja[-_]?netfilter|janetfilter|netfilter[-_]proxy|jetbrains[-_\s]?agent|` +
	// Genel patch/keygen araçları (dosya izi, startup, Defender istisna taramalarında yakalanır)
	`keygen|key\s*gen|crack\s*patch|license\s*patch|serial\s*key|` +
	`activator.*pro|pro.*activator|patcher.*exe|exe.*patcher|` +
	// Adobe crack araçları (amtlib.dll'e ek)
	`adobe\s*zii|adobe\s*patcher|amtemu|adobe\s*activat|` +
	// AutoCAD / Autodesk crack araçları
	`autocad.*crack|autodesk.*crack|adsk.*crack|solidworks.*crack|solidworks.*keygen|` +
	// Mühendislik/tasarım yazılımı crack araçları
	`matlab.*crack|matlab.*keygen|ansys.*crack|catia.*crack|` +
	// Müzik üretim yazılımı crack araçları
	`ableton.*crack|fl.*studio.*crack|cubase.*crack|native.*instruments.*crack|` +
	// Oyun motorları / genel crack araçları
	`unity.*crack|unreal.*crack|` +
	// İndirici crack araçları (IDM vb.)
	`idm.*crack|idm.*patch|idm.*activat|` +
	// FlexNet / HASP tabanlı lisans sunucusu crack araçları
	// (Not: lmgrd/adskflex gerçek enterprise lisans sunucusunda meşru olabilir;
	// bu token'lar yalnızca dosya izi/startup/task adlarında anlamlıdır,
	// running process kontrolünde ayrıca değerlendirilir)
	`crack.*lmgrd|lmgrd.*crack|adskflex.*crack|hasp.*crack|hasp.*emu`

// crackToolIndicatorRegex is the Go-side equivalent of crackToolIndicatorPattern, used to
// validate the pattern and to classify text (e.g. event log messages) read back from
// remote PowerShell output without re-sending it through another remote regex pass.
var crackToolIndicatorRegex = regexp.MustCompile(`(?i)` + crackToolIndicatorPattern)

// matchesCrackToolIndicator reports whether text contains a known crack/license-bypass
// tool name fragment.
func matchesCrackToolIndicator(text string) bool {
	if text == "" {
		return false
	}
	return crackToolIndicatorRegex.MatchString(text)
}

// findingSignature builds the normalized identity used both for in-scan dedup and for
// matching a finding against a stored exception. Category+Title+Evidence is the same key
// enrichAndDedupeFindings already used; centralized here so both call sites agree.
func findingSignature(f CrackFinding) string {
	return strings.ToLower(strings.TrimSpace(f.Category + "|" + f.Title + "|" + f.Evidence))
}

// kmsHostClassification describes what we think about a configured KMS host.
type kmsHostClassification string

const (
	kmsHostNone          kmsHostClassification = ""               // no KMS host configured (MAK/digital license/unactivated)
	kmsHostLocalEmulator kmsHostClassification = "local_emulator" // points at itself - classic KMSAuto/KMSpico/py-kms pattern
	kmsHostUnknown       kmsHostClassification = "unknown"         // a real host that the domain's own DNS doesn't vouch for
	kmsHostTrusted       kmsHostClassification = "trusted"         // matches the domain's officially published KMS host (DNS SRV)
)

// classifyKMSHost decides whether a SoftwareLicensingService-reported KMS host looks like
// a legitimate corporate KMS, an unverified one, or a local activation-bypass emulator.
// hostname/ip are the scanned machine's own identifiers (so "points at itself" can be
// detected regardless of which name/IP form Windows reported). srvPublishedHosts comes
// from the domain's own DNS SRV record for KMS auto-discovery (_vlmcs._tcp.<domain>) -
// this is how Windows itself decides which KMS host to trust, so a match requires zero
// manual configuration: if the domain admin published it via DNS, it's trusted; if not,
// it's merely "unknown" rather than "trusted" by default. There is deliberately no
// manually-maintained allowlist here - DNS SRV is the single source of truth.
func classifyKMSHost(kmsHost string, machineHostname string, machineIPs []string, srvPublishedHosts []string) kmsHostClassification {
	h := strings.ToLower(strings.TrimSpace(kmsHost))
	if h == "" {
		return kmsHostNone
	}
	// Strip a trailing :port if present.
	if idx := strings.LastIndex(h, ":"); idx > 0 {
		h = h[:idx]
	}

	loopback := map[string]bool{
		"127.0.0.1": true,
		"::1":       true,
		"localhost": true,
		"0.0.0.0":   true,
	}
	if loopback[h] {
		return kmsHostLocalEmulator
	}
	if mh := strings.ToLower(strings.TrimSpace(machineHostname)); mh != "" && (h == mh || strings.HasPrefix(h, mh+".")) {
		return kmsHostLocalEmulator
	}
	for _, ip := range machineIPs {
		if strings.ToLower(strings.TrimSpace(ip)) == h {
			return kmsHostLocalEmulator
		}
	}

	for _, srv := range srvPublishedHosts {
		s := strings.ToLower(strings.TrimSpace(srv))
		s = strings.TrimSuffix(s, ".") // DNS SRV NameTarget is FQDN-canonical, often trailing-dot terminated
		if s == "" {
			continue
		}
		if s == h || strings.TrimSuffix(h, ".") == s {
			return kmsHostTrusted
		}
	}
	return kmsHostUnknown
}

// licenseValidationDomains are the official Microsoft/Adobe activation and license
// validation endpoints (per Microsoft Learn's KMS endpoint docs and Adobe's own
// network-endpoints documentation). Crack tools commonly redirect these to a loopback/
// blackhole address in the hosts file so the OS/app can never reach the real validation
// server and reports itself as activated/genuine. A normal machine has no legitimate
// reason to blackhole these specific hosts (unlike general telemetry domains, which IT
// may block for privacy reasons without that being a piracy signal) - the specificity is
// what keeps this check low false-positive.
var licenseValidationDomains = []string{
	// Microsoft activation/licensing
	"activation.sls.microsoft.com",
	"validation.sls.microsoft.com",
	"activation-v2.sls.microsoft.com",
	"validation-v2.sls.microsoft.com",
	"displaycatalog.mp.microsoft.com",
	"licensing.mp.microsoft.com",
	"licensing.md.mp.microsoft.com",
	"purchase.mp.microsoft.com",
	// Adobe activation/licensing
	"lm.licenses.adobe.com",
	"resources.licenses.adobe.com",
	"cs.licenses.adobe.com",
	"exception.licenses.adobe.com",
	"pubcerts.licenses.adobe.com",
	"workflow.licenses.adobe.com",
	"auth.services.adobe.com",
	"practivate.adobe.com",
	"activate.adobe.com",
	"ereg.adobe.com",
	"lmlicenses.wip4.adobe.com",
	// JetBrains (IntelliJ/PyCharm/WebStorm/Rider/GoLand vb.) lisans doğrulaması
	"account.jetbrains.com",
	"accounts.jetbrains.com",
	"license.jetbrains.com",
	// Autodesk (AutoCAD, Revit, Maya, 3ds Max, Inventor vb.) lisans doğrulaması
	"activate.autodesk.com",
	"genuine.autodesk.com",
	"registeronce.autodesk.com",
	"autodesk.com.edgesuite.net",
	// Corel (CorelDRAW, Painter vb.) lisans doğrulaması
	"iws.corel.com",
	"mcd.corel.com",
	// Steinberg (Cubase, Nuendo, WaveLab) lisans doğrulaması
	"elicenser.net",
	"activation.steinberg.net",
	// Ableton (Ableton Live) lisans doğrulaması
	"register.ableton.com",
	"authorize.ableton.com",
	// Image-Line (FL Studio) lisans doğrulaması
	"license.image-line.com",
	"install.image-line.com",
	// Native Instruments (Komplete, Traktor vb.) lisans doğrulaması
	"reg.native-instruments.com",
	"license.native-instruments.com",
	// Maxon (Cinema 4D, Red Giant vb.) lisans doğrulaması
	"maxon.net",
	// DaVinci Resolve — lisanssız (Studio) sürümün doğrulaması
	"licensing.blackmagicdesign.com",
}

// licenseValidationDomainsPSArray renders licenseValidationDomains as a PowerShell array
// literal, e.g. 'a','b','c' - for embedding into the hosts-file tampering check script.
func licenseValidationDomainsPSArray() string {
	quoted := make([]string, len(licenseValidationDomains))
	for i, d := range licenseValidationDomains {
		quoted[i] = "'" + d + "'"
	}
	return strings.Join(quoted, ",")
}
