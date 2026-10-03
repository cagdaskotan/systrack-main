package handlers

import (
	"strings"
	"testing"
)

func TestCalcRiskLevel(t *testing.T) {
	cases := []struct {
		name     string
		findings []CrackFinding
		want     string
	}{
		{"no findings", nil, "clean"},
		{"single low", []CrackFinding{{Severity: "low"}}, "low"},
		{"single medium", []CrackFinding{{Severity: "medium"}}, "medium"},
		{"single high", []CrackFinding{{Severity: "high"}}, "high"},
		{"two mediums escalate to high", []CrackFinding{{Severity: "medium"}, {Severity: "medium"}}, "high"},
		{"excepted high is ignored", []CrackFinding{{Severity: "high", Excepted: true}}, "clean"},
		{"excepted high plus real medium", []CrackFinding{{Severity: "high", Excepted: true}, {Severity: "medium"}}, "medium"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := calcRiskLevel(tc.findings); got != tc.want {
				t.Errorf("calcRiskLevel() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMatchesCrackToolIndicator(t *testing.T) {
	shouldMatch := []string{
		"C:\\SysTrackTest\\KMSpico\\KMSpico.exe",
		"AutoKMS-SysTrack-Test",
		"KMSAuto Net Activation Service",
		"TSforge.exe",
		"Ohook.dll",
		"HWIDGEN_v52.exe",
		"MAS_AIO.cmd",
		"Microsoft Toolkit 2.6.7",
		"Re-Loader by Ratiborus",
		"py-kms server",
	}
	for _, s := range shouldMatch {
		if !matchesCrackToolIndicator(s) {
			t.Errorf("expected %q to match crack tool indicator pattern", s)
		}
	}

	shouldNotMatch := []string{
		"Microsoft Office Professional Plus 2021",
		"Google Chrome",
		"Windows Defender",
		"",
	}
	for _, s := range shouldNotMatch {
		if matchesCrackToolIndicator(s) {
			t.Errorf("expected %q NOT to match crack tool indicator pattern", s)
		}
	}
}

func TestClassifyKMSHost(t *testing.T) {
	// Bu liste artık manuel girilen bir "güvenilir host" listesi değil - domain'in kendi
	// DNS SRV kaydından (_vlmcs._tcp.<domain>) otomatik okunan host'ları temsil ediyor.
	srvPublished := []string{"kms.corp.local", "10.0.0.5", "dc1.corp.local."}

	cases := []struct {
		name    string
		kmsHost string
		machine string
		ips     []string
		want    kmsHostClassification
	}{
		{"empty host", "", "PC01", nil, kmsHostNone},
		{"loopback ip", "127.0.0.1", "PC01", nil, kmsHostLocalEmulator},
		{"loopback with port", "127.0.0.1:1688", "PC01", nil, kmsHostLocalEmulator},
		{"localhost literal", "localhost", "PC01", nil, kmsHostLocalEmulator},
		{"self hostname", "PC01", "pc01", nil, kmsHostLocalEmulator},
		{"self ip", "10.20.11.128", "PC01", []string{"10.20.11.128"}, kmsHostLocalEmulator},
		{"srv published host", "kms.corp.local", "PC01", []string{"10.20.11.128"}, kmsHostTrusted},
		{"srv published ip", "10.0.0.5", "PC01", []string{"10.20.11.128"}, kmsHostTrusted},
		{"srv published with trailing dot vs without", "dc1.corp.local", "PC01", []string{"10.20.11.128"}, kmsHostTrusted},
		{"unknown external host", "kms.evil.example", "PC01", []string{"10.20.11.128"}, kmsHostUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyKMSHost(tc.kmsHost, tc.machine, tc.ips, srvPublished); got != tc.want {
				t.Errorf("classifyKMSHost(%q) = %q, want %q", tc.kmsHost, got, tc.want)
			}
		})
	}
}

func TestEnrichAndDedupeFindingsFillsDefaultsAndDedupes(t *testing.T) {
	findings := []CrackFinding{
		{Category: "kms_tool", Severity: "high", Title: "X", Evidence: "Servis: foo"},
		{Category: "kms_tool", Severity: "high", Title: "X", Evidence: "Servis: foo"}, // duplicate
		{Category: "threat_intel", Severity: "high", Title: "Y", Evidence: "SHA256: abc"},
	}
	out := enrichAndDedupeFindings(findings)
	if len(out) != 2 {
		t.Fatalf("expected dedup to collapse to 2 findings, got %d", len(out))
	}
	for _, f := range out {
		if f.Confidence == 0 {
			t.Errorf("expected default confidence to be filled for %q", f.Title)
		}
		if f.Remediation == "" {
			t.Errorf("expected default remediation to be filled for %q", f.Title)
		}
		if f.Signal == "" {
			t.Errorf("expected default signal to be filled for %q", f.Title)
		}
	}
}

func TestAnnotateExceptedFindings(t *testing.T) {
	f1 := CrackFinding{Category: "kms_tool", Severity: "high", Title: "KMS Bypass Servisi Tespit Edildi", Evidence: "Servis: foo"}
	f2 := CrackFinding{Category: "activation", Severity: "medium", Title: "Lisans Durumu: Windows", Evidence: "LicenseStatus=3"}

	hashes := map[string]bool{
		sha256Hex(findingSignature(f1)): true,
	}

	out, risk := annotateExceptedFindings([]CrackFinding{f1, f2}, hashes)
	if !out[0].Excepted {
		t.Errorf("expected f1 to be marked excepted")
	}
	if out[1].Excepted {
		t.Errorf("expected f2 to remain non-excepted")
	}
	if risk != "medium" {
		t.Errorf("expected risk to be based only on non-excepted medium finding, got %q", risk)
	}
}

func TestParseCSCBCSV(t *testing.T) {
	csv := "# MalwareBazaar CSCB export\n" +
		"# generated 2026-01-01\n" +
		"time_stamp_utc,serial_number,thumbprint,thumbprint_algorithm,subject_cn,issuer_cn,valid_from,valid_to,Reason\n" +
		"\"2026-01-01 00:00:00\",\"01\",\"AA:BB:CC:DD\",\"sha1\",\"Evil Corp\",\"Some CA\",\"2020-01-01\",\"2030-01-01\",\"abuse\"\n"

	got, err := parseCSCBCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parseCSCBCSV error: %v", err)
	}
	if !got["AABBCCDD"] {
		t.Errorf("expected normalized thumbprint AABBCCDD to be present, got %v", got)
	}
}

func TestNormalizeThumbprint(t *testing.T) {
	if got := normalizeThumbprint("aa:bb cc-dd"); got != "AABBCC-DD" {
		t.Errorf("normalizeThumbprint() = %q", got)
	}
}
