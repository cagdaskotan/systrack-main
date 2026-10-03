package scanner

// generatedOUIData is populated via `go generate`. At first it is nil,
// so the scanner falls back to the embedded vendor map from gopacket.
// Run `go generate ./internal/network/scanner` after placing an IEEE
// `oui.txt` file under the `data/` directory to populate this map.
var generatedOUIData map[[3]byte]string
