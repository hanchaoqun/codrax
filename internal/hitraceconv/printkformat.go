package hitraceconv

import (
	"bytes"
	"strconv"
	"strings"
)

const maxPrintkFormatLineBytes = 64 * 1024

type printkFormatCatalog struct {
	Formats   map[uint64]string
	Poisoned  map[uint64]bool
	Malformed int
}

func parsePrintkFormats(data []byte) printkFormatCatalog {
	out := printkFormatCatalog{Formats: make(map[uint64]string), Poisoned: make(map[uint64]bool)}
	for len(data) > 0 {
		lineBytes := data
		if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			lineBytes = data[:newline]
			data = data[newline+1:]
		} else {
			data = nil
		}
		if len(lineBytes) == 0 {
			continue
		}
		address, addressOK := attributePrintkAddress(lineBytes)
		if len(lineBytes) > maxPrintkFormatLineBytes {
			if addressOK {
				poisonPrintkFormat(&out, address)
			} else {
				out.Malformed++
			}
			continue
		}
		line := string(lineBytes)
		payload, lineOK := parsePrintkFormatLine(line)
		if !addressOK {
			out.Malformed++
			continue
		}
		if !lineOK {
			poisonPrintkFormat(&out, address)
			continue
		}
		admitPrintkFormat(&out, address, payload)
	}
	return out
}

func parsePrintkFormatLine(line string) (string, bool) {
	address, ok := parsePrintkAddressPrefix(line)
	if !ok {
		return "", false
	}
	prefix := "0x" + strconv.FormatUint(address, 16) + ` : "`
	if !strings.HasPrefix(line, prefix) || len(line) < len(prefix)+1 || line[len(line)-1] != '"' {
		return "", false
	}
	payload := line[len(prefix) : len(line)-1]
	if !traceDBSinglePhysicalLine(payload, true) {
		return "", false
	}
	for index := 0; index < len(payload); index++ {
		switch payload[index] {
		case '\\':
			if index+1 >= len(payload) {
				return "", false
			}
			index++ // Preserve, but validate, the kernel's escaped spelling.
		case '"':
			return "", false
		}
	}
	return payload, true
}

// attributePrintkAddress is deliberately broader than canonical admission.
// Uppercase prefixes/digits and leading whitespace are malformed kernel
// spelling, but still identify the same physical pointer and therefore must
// poison an earlier clean mapping instead of escaping quarantine.
func attributePrintkAddress(line []byte) (uint64, bool) {
	index := 0
	for index < len(line) && (line[index] == ' ' || line[index] == '\t') {
		index++
	}
	if index+2 > len(line) || line[index] != '0' || (line[index+1] != 'x' && line[index+1] != 'X') {
		return 0, false
	}
	start := index + 2
	end := start
	for end < len(line) && isPrintkHexByte(line[end]) {
		end++
	}
	if end == start {
		return 0, false
	}
	separator := end
	for separator < len(line) && (line[separator] == ' ' || line[separator] == '\t') {
		separator++
	}
	if separator >= len(line) || line[separator] != ':' {
		return 0, false
	}
	digits := line[start:end]
	for len(digits) > 1 && digits[0] == '0' {
		digits = digits[1:]
	}
	if len(digits) > 16 {
		return 0, false
	}
	address, err := strconv.ParseUint(string(digits), 16, 64)
	return address, err == nil && address != 0
}

func isPrintkHexByte(value byte) bool {
	return (value >= '0' && value <= '9') || (value >= 'a' && value <= 'f') || (value >= 'A' && value <= 'F')
}

func parsePrintkAddressPrefix(line string) (uint64, bool) {
	if !strings.HasPrefix(line, "0x") {
		return 0, false
	}
	separator := strings.Index(line, " :")
	if separator <= 2 {
		return 0, false
	}
	hexText := line[2:separator]
	for _, r := range hexText {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return 0, false
		}
	}
	address, err := strconv.ParseUint(hexText, 16, 64)
	return address, err == nil && address != 0
}

func admitPrintkFormat(catalog *printkFormatCatalog, address uint64, payload string) {
	if catalog == nil || address == 0 {
		return
	}
	if catalog.Formats == nil {
		catalog.Formats = make(map[uint64]string)
	}
	if catalog.Poisoned == nil {
		catalog.Poisoned = make(map[uint64]bool)
	}
	if catalog.Poisoned[address] {
		return
	}
	existing, found := catalog.Formats[address]
	if !found {
		catalog.Formats[address] = payload
		return
	}
	if existing == payload {
		return
	}
	poisonPrintkFormat(catalog, address)
}

func poisonPrintkFormat(catalog *printkFormatCatalog, address uint64) {
	if catalog == nil || address == 0 {
		return
	}
	if catalog.Formats == nil {
		catalog.Formats = make(map[uint64]string)
	}
	if catalog.Poisoned == nil {
		catalog.Poisoned = make(map[uint64]bool)
	}
	delete(catalog.Formats, address)
	catalog.Poisoned[address] = true
}

func mergePrintkFormatCatalog(destination *printkFormatCatalog, source printkFormatCatalog) {
	if destination == nil {
		return
	}
	if destination.Formats == nil {
		destination.Formats = make(map[uint64]string)
	}
	if destination.Poisoned == nil {
		destination.Poisoned = make(map[uint64]bool)
	}
	for address := range source.Poisoned {
		poisonPrintkFormat(destination, address)
	}
	for address, payload := range source.Formats {
		admitPrintkFormat(destination, address, payload)
	}
	destination.Malformed += source.Malformed
}
