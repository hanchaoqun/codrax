package tracequery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexLimitMemoScopesReuseToRequestSourceAndOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "limit.trace")
	row := "          <idle>-0     (-----) [000] d..3  100.000000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=10 next_prio=100\n"
	content := strings.Repeat(row, 3)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	request := WithIndexLimitMemo(t.Context())
	query := UseIndexLimitMemo(request)
	opts := BuildOptions{AllowWindowedParse: true, LineStart: 1, LineEnd: 10, MaxEvents: 1}
	var key parseCacheKey
	index, err := buildIndexWithObserver(query, path, opts, func(phase traceIndexBuildPhase, selected parseCacheKey) {
		if phase == traceIndexPhaseSelectionFrozen {
			key = selected
		}
	})
	first, ok := err.(*IndexEventLimitError)
	if index != nil || !ok || first.MaxEvents != opts.MaxEvents {
		t.Fatalf("expected an event-limit failure, got index=%p err=%v", index, err)
	}
	memo := query.Value(activeIndexLimitMemoKey{}).(*indexLimitMemo)
	memoKey := indexLimitMemoKey{source: key, opts: normalizeBuildOptions(opts)}
	if stored, ok := memo.get(memoKey); !ok || stored != *first {
		t.Fatalf("expected a copy of the original failure, got %+v (ok=%t)", stored, ok)
	}

	// A distinct line makes the memo hit observable independently of timing.
	marked := *first
	marked.Line += 100
	memo.put(memoKey, marked)
	scoped, larger, narrower := opts, opts, opts
	scoped.ScopePID = 10
	larger.MaxEvents = 2
	narrower.LineEnd = 9
	for _, tc := range []struct {
		name string
		ctx  context.Context
		opts BuildOptions
		line int
	}{
		{"same request and options", UseIndexLimitMemo(request), opts, marked.Line},
		{"query did not opt in", request, opts, first.Line},
		{"different request", UseIndexLimitMemo(WithIndexLimitMemo(t.Context())), opts, first.Line},
		{"different scope metadata", query, scoped, first.Line},
		{"different event limit", query, larger, first.Line + 1},
		{"different window", query, narrower, first.Line},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildIndexWithOptions(tc.ctx, path, tc.opts)
			var limit *IndexEventLimitError
			if !errors.As(err, &limit) || limit.Line != tc.line || limit.ScopePID != tc.opts.ScopePID || limit.MaxEvents != tc.opts.MaxEvents {
				t.Fatalf("wanted limit at line %d with options %+v, got %v", tc.line, tc.opts, err)
			}
		})
	}
	canceled, cancel := context.WithCancel(query)
	cancel()
	if _, err := BuildIndexWithOptions(canceled, path, opts); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled query must return cancellation, got %v", err)
	}
	if err := os.WriteFile(path, []byte("# updated trace\n"+content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = BuildIndexWithOptions(query, path, opts)
	var updated *IndexEventLimitError
	if !errors.As(err, &updated) || updated.Line != first.Line+1 {
		t.Fatalf("changed source must rebuild, got %v", err)
	}
}
