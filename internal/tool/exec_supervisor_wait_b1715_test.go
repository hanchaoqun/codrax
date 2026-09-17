package tool

import (
	"errors"
	"testing"
	"time"
)

func TestB1715SupervisorWaitTimeoutIsIncomplete(t *testing.T) {
	for _, budget := range []time.Duration{0, time.Millisecond} {
		if err := waitForExistingWaitWithin(make(chan error), budget); !errors.Is(err, errSupervisedWaitIncomplete) {
			t.Errorf("expired wait budget minted success: budget=%v err=%v", budget, err)
		}
	}
	for _, want := range []error{nil, errors.New("actual command failure")} {
		ready := make(chan error, 1)
		ready <- want
		if err := waitForExistingWaitWithin(ready, 0); !errors.Is(err, want) {
			t.Errorf("completed wait changed: got=%v want=%v", err, want)
		}
	}
	if killWaitTimeout != 10*time.Second {
		t.Fatalf("cleanup budget changed: %v", killWaitTimeout)
	}
}
