package repl

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

type rootCauseDeliveryTestRunner struct {
	stubRunner
	err error
}

func (r rootCauseDeliveryTestRunner) RootCauseOutputError() error { return r.err }

func TestRootCauseOutputFailureWarnsWithoutDiscardingAnswer(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		t.Run(language, func(t *testing.T) {
			var out bytes.Buffer
			r := newTestREPL(nil, strings.NewReader(""), &out)
			r.language = language
			r.runner = rootCauseDeliveryTestRunner{err: errors.New("disk unavailable")}
			bus := &types.BusContext{Mutable: types.NewMutableState("trace")}
			bus.Mutable.SetResult("model-owned answer")
			got, err := r.runInFlightWrap(func() (*types.BusContext, error) { return bus, nil })
			if err != nil || got != bus || got.Mutable.Result() != "model-owned answer" {
				t.Fatal("file warning must not fail or rewrite answer")
			}
			if !strings.Contains(out.String(), "disk unavailable") {
				t.Fatalf("missing warning: %s", out.String())
			}
		})
	}
}
