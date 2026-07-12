package hitraceconv

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type eventField struct {
	Type   string
	Name   string
	Offset int
	Size   int
	Signed bool
}

type eventFormat struct {
	ID       int
	Name     string
	Fields   []eventField
	PrintFmt string
}

type eventFormatCatalog struct {
	Formats  map[int]eventFormat
	Poisoned map[int]bool
}

var fieldLineRE = regexp.MustCompile(`^field:([^;]+);\s*offset:(\d+);\s*size:(\d+);\s*signed:(\d+)\s*;?\s*$`)

func parseEventFormats(data []byte) (eventFormatCatalog, error) {
	out := eventFormatCatalog{Formats: make(map[int]eventFormat), Poisoned: make(map[int]bool)}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	cur := eventFormat{ID: -1}
	ids := make(map[int]bool)
	seenID := false
	seenPrintFmt := false
	fieldNames := make(map[string]bool)
	malformed := false
	flush := func() {
		if malformed || (len(ids) > 0 && cur.Name == "") {
			for id := range ids {
				poisonEventFormatID(&out, id)
			}
		} else if cur.ID >= 0 && cur.Name != "" {
			admitEventFormat(&out, cur)
		}
		cur = eventFormat{ID: -1}
		ids = make(map[int]bool)
		seenID = false
		seenPrintFmt = false
		fieldNames = make(map[string]bool)
		malformed = false
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "name:"):
			flush()
			cur.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			malformed = cur.Name == ""
		case strings.HasPrefix(line, "ID:"):
			id, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "ID:")))
			if seenID {
				malformed = true
			}
			seenID = true
			if err == nil && id >= 0 {
				ids[id] = true
				if cur.ID < 0 {
					cur.ID = id
				}
			} else {
				malformed = true
			}
		case strings.HasPrefix(line, "field:"):
			f, ok := parseFieldLine(line)
			if !ok {
				malformed = true
				break
			}
			fieldName := cleanFieldName(f.Name)
			if fieldName == "" || fieldNames[fieldName] {
				malformed = true
				break
			}
			fieldNames[fieldName] = true
			cur.Fields = append(cur.Fields, f)
		case strings.HasPrefix(line, "print fmt:"):
			if seenPrintFmt {
				malformed = true
			} else {
				cur.PrintFmt = strings.TrimSpace(strings.TrimPrefix(line, "print fmt:"))
			}
			seenPrintFmt = true
		}
	}
	if err := sc.Err(); err != nil {
		return eventFormatCatalog{}, fmt.Errorf("parse events_format segment: %w", err)
	}
	flush()
	return out, nil
}

func admitEventFormat(catalog *eventFormatCatalog, candidate eventFormat) {
	if catalog == nil || candidate.ID < 0 || candidate.Name == "" {
		return
	}
	if catalog.Formats == nil {
		catalog.Formats = make(map[int]eventFormat)
	}
	if catalog.Poisoned == nil {
		catalog.Poisoned = make(map[int]bool)
	}
	if catalog.Poisoned[candidate.ID] {
		return
	}
	existing, found := catalog.Formats[candidate.ID]
	if !found {
		catalog.Formats[candidate.ID] = candidate
		return
	}
	if eventFormatsEqual(existing, candidate) {
		return
	}
	poisonEventFormatID(catalog, candidate.ID)
}

func poisonEventFormatID(catalog *eventFormatCatalog, id int) {
	if catalog == nil || id < 0 {
		return
	}
	if catalog.Formats == nil {
		catalog.Formats = make(map[int]eventFormat)
	}
	if catalog.Poisoned == nil {
		catalog.Poisoned = make(map[int]bool)
	}
	delete(catalog.Formats, id)
	catalog.Poisoned[id] = true
}

func mergeEventFormatCatalog(destination *eventFormatCatalog, source eventFormatCatalog) {
	if destination == nil {
		return
	}
	if destination.Formats == nil {
		destination.Formats = make(map[int]eventFormat)
	}
	if destination.Poisoned == nil {
		destination.Poisoned = make(map[int]bool)
	}
	for id := range source.Poisoned {
		poisonEventFormatID(destination, id)
	}
	for id, format := range source.Formats {
		if destination.Poisoned[id] {
			continue
		}
		admitEventFormat(destination, format)
	}
}

func eventFormatsEqual(left, right eventFormat) bool {
	if left.ID != right.ID || left.Name != right.Name || left.PrintFmt != right.PrintFmt || len(left.Fields) != len(right.Fields) {
		return false
	}
	for index := range left.Fields {
		if left.Fields[index] != right.Fields[index] {
			return false
		}
	}
	return true
}

func parseFieldLine(line string) (eventField, bool) {
	m := fieldLineRE.FindStringSubmatch(line)
	if len(m) != 5 {
		return eventField{}, false
	}
	typeAndName := strings.TrimSpace(m[1])
	pos := strings.LastIndex(typeAndName, " ")
	if pos <= 0 || pos >= len(typeAndName)-1 {
		return eventField{}, false
	}
	offset, err1 := strconv.Atoi(m[2])
	size, err2 := strconv.Atoi(m[3])
	signed, err3 := strconv.Atoi(m[4])
	if err1 != nil || err2 != nil || err3 != nil || offset < 0 || size <= 0 || (signed != 0 && signed != 1) {
		return eventField{}, false
	}
	return eventField{
		Type:   strings.TrimSpace(typeAndName[:pos]),
		Name:   strings.TrimSpace(typeAndName[pos+1:]),
		Offset: offset,
		Size:   size,
		Signed: signed != 0,
	}, true
}

func parseCmdlines(data []byte) map[int]string {
	out := make(map[int]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, "\t", ""))
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}
		out[pid] = strings.TrimSpace(parts[1])
	}
	return out
}

func parseTGIDs(data []byte) map[int]int {
	out := make(map[int]int)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		pid, err1 := strconv.Atoi(parts[0])
		tgid, err2 := strconv.Atoi(parts[1])
		if err1 == nil && err2 == nil {
			out[pid] = tgid
		}
	}
	return out
}
