package types

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

type sentinelTraceInputPreparer struct {
	calls     int
	materials []*attachment.TraceMaterial
}

func (p *sentinelTraceInputPreparer) Prepare(context.Context, string) (*attachment.TraceMaterial, error) {
	p.calls++
	return nil, nil
}

func (p *sentinelTraceInputPreparer) PreparedMaterials() []*attachment.TraceMaterial {
	return append([]*attachment.TraceMaterial(nil), p.materials...)
}

func TestTraceInputPreparerSharedThroughProjectionsWithoutJSONAuthority(t *testing.T) {
	p := &sentinelTraceInputPreparer{}
	bus := &BusContext{TraceInputPreparer: p}
	cloned := bus.ShallowClone()
	agent := SubAgentContext(cloned, &SubAgentRequest{SubAgent: "worker"})
	tool := ToolBusContext(agent.ShallowClone(), "worker")
	if cloned.TraceInputPreparer != p || agent.TraceInputPreparer != p || tool.TraceInputPreparer != p {
		t.Fatal("Run-owned trace preparer must be the same handle across every projection")
	}
	if _, err := tool.TraceInputPreparer.Prepare(context.Background(), "/trace.sys"); err != nil || p.calls != 1 {
		t.Fatalf("projection did not reach shared preparer: calls=%d err=%v", p.calls, err)
	}
	for _, value := range []any{bus, agent} {
		data, err := json.Marshal(value)
		if err != nil || strings.Contains(string(data), "TraceInputPreparer") || strings.Contains(string(data), "trace_input_preparer") {
			t.Fatalf("runtime preparer leaked into serialized model authority: %s, err=%v", data, err)
		}
	}
}
