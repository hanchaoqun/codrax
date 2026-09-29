package tracewire

import (
	"fmt"
	"strings"
)

const ProcessIntervalPrefix = "# codrax_process_interval/v1"

// ProcessInterval is one source-table endpoint, not an ftrace thread-stack
// marker. The source record carries its own complete interval and local owner
// reference. No thread, scheduling CPU, startup instance or cause is inferred.
type ProcessInterval struct {
	Endpoint string
	Origin   MarkerNameOrigin
}

func (p ProcessInterval) TimestampNS() int64 {
	if p.Endpoint == "end" {
		return p.Origin.Record.EndNS
	}
	return p.Origin.Record.StartNS
}

func FormatProcessInterval(p ProcessInterval) (string, error) {
	if p.Origin.Record == nil || p.Endpoint != "begin" && p.Endpoint != "end" {
		return "", fmt.Errorf("invalid process interval endpoint")
	}
	payload, err := EncodeMarkerNameOrigin(p.Origin)
	if err != nil {
		return "", err
	}
	return ProcessIntervalPrefix + " endpoint=" + p.Endpoint + " source=" + payload, nil
}

func ParseProcessInterval(line string) (ProcessInterval, bool) {
	if !strings.HasPrefix(line, ProcessIntervalPrefix+" endpoint=") {
		return ProcessInterval{}, false
	}
	endpoint, payload, ok := strings.Cut(strings.TrimPrefix(line, ProcessIntervalPrefix+" endpoint="), " source=")
	if !ok {
		return ProcessInterval{}, false
	}
	origin, ok := DecodeMarkerNameOrigin(payload)
	if !ok {
		return ProcessInterval{}, false
	}
	p := ProcessInterval{Endpoint: endpoint, Origin: origin}
	canonical, err := FormatProcessInterval(p)
	return p, err == nil && canonical == line
}
