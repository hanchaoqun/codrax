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

var fieldLineRE = regexp.MustCompile(`^field:(.+);\s*offset:(\d+);\s*size:(\d+);\s*signed:(\d+)`)

func parseEventFormats(data []byte) (map[int]eventFormat, error) {
	out := make(map[int]eventFormat)
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	cur := eventFormat{ID: -1}
	flush := func() {
		if cur.ID >= 0 && cur.Name != "" {
			out[cur.ID] = cur
		}
		cur = eventFormat{ID: -1}
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "name: "):
			flush()
			cur.Name = strings.TrimSpace(strings.TrimPrefix(line, "name: "))
		case strings.HasPrefix(line, "ID: "):
			id, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "ID: ")))
			if err == nil {
				cur.ID = id
			}
		case strings.HasPrefix(line, "field:"):
			if f, ok := parseFieldLine(line); ok {
				cur.Fields = append(cur.Fields, f)
			}
		case strings.HasPrefix(line, "print fmt: "):
			cur.PrintFmt = strings.TrimSpace(strings.TrimPrefix(line, "print fmt: "))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("parse events_format segment: %w", err)
	}
	flush()
	return out, nil
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
	if err1 != nil || err2 != nil || err3 != nil || offset < 0 || size <= 0 {
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
