package hitraceconv

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestExternalToolCommandPreCanceledDoesNotStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ready := t.TempDir() + string(os.PathSeparator) + "unexpected.ready"
	command, err := newExternalToolCommand(ctx, os.Args[0], "-test.run=^TestExternalToolCommandSupervisorProcessTree$")
	if err != nil {
		t.Fatal(err)
	}
	command.setEnvironment(append(os.Environ(),
		externalToolSupervisorTestMode+"=root_cancel",
		externalToolSupervisorTestReady+"="+ready,
	))
	runErr, started := runExternalToolCommandUntilExit(command, nil)
	if started || !errors.Is(runErr, context.Canceled) {
		t.Fatalf("pre-canceled command crossed Start: started=%v err=%v", started, runErr)
	}
	if _, err := os.Stat(ready); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-canceled command published child receipt: %v", err)
	}
}

func TestExternalToolCommandCanOnlyBeConsumedOnce(t *testing.T) {
	command, err := newExternalToolCommand(context.Background(), os.Args[0], "-test.run=^$")
	if err != nil {
		t.Fatal(err)
	}
	if runErr, started := runExternalToolCommandUntilExit(command, nil); runErr != nil || !started {
		t.Fatalf("first command consumption failed: started=%v err=%v", started, runErr)
	}
	if runErr, started := runExternalToolCommandUntilExit(command, nil); started || !errors.Is(runErr, errExternalToolSupervisorAuthority) {
		t.Fatalf("second command consumption escaped singleton authority: started=%v err=%v", started, runErr)
	}
}
