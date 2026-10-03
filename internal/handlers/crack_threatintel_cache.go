package handlers

import (
	"sync"
	"time"
)

// threatIntelCacheTTL bounds how long a hash's MalwareBazaar/OTX/NSRL lookup result is
// reused. Bulk discovery scans many machines that often share the same binaries (e.g. the
// same WINWORD.EXE build) - without this, every machine re-queries the same external APIs
// for the same hash, multiplying external calls and risking rate limits.
const threatIntelCacheTTL = 6 * time.Hour

type threatIntelResult struct {
	// MalwareBazaar: hash lookup
	mbFound     bool
	mbSignature string
	mbTags      []string
	// MalwareBazaar: imphash lookup (aynı import yapısına sahip varyantlar)
	// OTX analysis'ten elde edilen imphash ile MB'de arama yapılır.
	mbImphashFound   bool
	mbImphashFamilies []string
	// OTX /general: topluluk tehdit istihbaratı
	otxPulseCount int
	otxFamilies   []string
	otxReputation int // negatif = zararlı/şüpheli
	// OTX /analysis: içerik tabanlı statik analiz
	otxAnalysis otxAnalysisResult
	// VirusTotal: 70+ AV motoru konsensüsü
	vtMalicious int
	vtLabel     string
	// NSRL: bilinen iyi dosya (Office NotSigned dalında kullanılır)
	nsrl      string
	fetchedAt time.Time
}

var threatIntelCache = struct {
	sync.Mutex
	entries map[string]threatIntelResult
}{entries: make(map[string]threatIntelResult)}

// lookupThreatIntel returns a cached result for sha256 if still fresh, otherwise:
// Phase 1 (parallel): MB hash, OTX /general, OTX /analysis, VirusTotal, NSRL (optional)
// Phase 2 (sequential): MB imphash (uses imphash from OTX analysis — depends on phase 1)
func lookupThreatIntel(sha256, mbKey, otxKey, vtKey string, includeNSRL bool) threatIntelResult {
	threatIntelCache.Lock()
	if cached, ok := threatIntelCache.entries[sha256]; ok && time.Since(cached.fetchedAt) < threatIntelCacheTTL {
		if !includeNSRL || cached.nsrl != "" {
			threatIntelCache.Unlock()
			return cached
		}
	}
	threatIntelCache.Unlock()

	var result threatIntelResult
	var wg sync.WaitGroup

	// Faz 1: MB hash, OTX /general, OTX /analysis, VT — paralel
	wg.Add(3)
	go func() {
		defer wg.Done()
		result.mbFound, result.mbSignature, result.mbTags = checkMalwareBazaar(sha256, mbKey)
	}()
	go func() {
		defer wg.Done()
		result.otxPulseCount, result.otxFamilies, result.otxReputation = checkOTX(sha256, otxKey)
	}()
	go func() {
		defer wg.Done()
		result.vtMalicious, result.vtLabel = checkVirusTotal(sha256, vtKey)
	}()

	// OTX /analysis: Adobe Malware Classifier, PE anomalileri, VMProtect/UPX/Themida, ClamAV, YARA
	if otxKey != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result.otxAnalysis = checkOTXAnalysis(sha256, otxKey)
		}()
	}

	if includeNSRL {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result.nsrl = checkNSRL(sha256)
		}()
	}

	wg.Wait()

	// Faz 2: MB imphash sorgusu — OTX analysis'ten elde edilen imphash'e göre.
	// Hash eşleşmesi olmasa bile aynı import yapısına sahip malware varyantlarını bulur.
	// (Örn: crack'lenen bir exe'nin özel hash'i MB'de yokken aynı araç ailesinden örnekler olabilir.)
	if mbKey != "" && result.otxAnalysis.Imphash != "" {
		result.mbImphashFound, result.mbImphashFamilies = checkMalwareBazaarImphash(result.otxAnalysis.Imphash, mbKey)
	}

	result.fetchedAt = time.Now()

	threatIntelCache.Lock()
	threatIntelCache.entries[sha256] = result
	threatIntelCache.Unlock()
	return result
}
