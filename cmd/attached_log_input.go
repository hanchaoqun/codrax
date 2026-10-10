package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

type cliPreparedLog struct {
	body    string
	catalog *loginput.Catalog
}

// File/stream input limits are independent of the bounded triage preview.
const maxCompleteLogInputBytes int64 = 1 << 30

func loadPreparedAttachedLog(ctx context.Context) (cliPreparedLog, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return cliPreparedLog{}, err
	}
	if len(flagAttachLog) > 0 && flagAttachLogText != "" {
		return cliPreparedLog{}, fmt.Errorf("--log and --log-text are mutually exclusive")
	}
	if err := enforceStdinExclusivity(); err != nil {
		return cliPreparedLog{}, err
	}
	if len(flagAttachLog) == 0 && flagAttachLogText == "" {
		return cliPreparedLog{}, nil
	}
	worktree.SetSignalHandlerSuppressed(true)
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer func() { stop(); worktree.SetSignalHandlerSuppressed(false) }()
	inputs := make([]loginput.Input, 0, len(flagAttachLog)+1)
	for _, path := range flagAttachLog {
		if path == "-" {
			data, err := readCompleteLogStream(ctx, os.Stdin, maxCompleteLogInputBytes)
			if err != nil {
				return cliPreparedLog{}, err
			}
			inputs = append(inputs, loginput.Input{Name: "stdin", Data: data})
		} else {
			inputs = append(inputs, loginput.Input{Path: path})
		}
	}
	if flagAttachLogText != "" {
		inputs = append(inputs, loginput.Input{Name: "inline", Data: []byte(flagAttachLogText)})
	}
	catalog, err := loginput.Prepare(ctx, inputs, loginput.Options{MaxInputBytes: maxCompleteLogInputBytes})
	if err != nil {
		return cliPreparedLog{}, err
	}
	return cliPreparedLog{body: catalog.Preview(maxAttachedLogBytes), catalog: catalog}, nil
}

// The sole stdin owner closes a blocked stream only on cancellation. EOF is
// mandatory: reaching a preview cap must never seal a partial source.
func readCompleteLogStream(ctx context.Context, reader io.ReadCloser, maxBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	done, exited := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(exited)
		select {
		case <-ctx.Done():
			_ = reader.Close()
		case <-done:
		}
	}()
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	close(done)
	<-exited
	if ctx.Err() != nil {
		return nil, errors.Join(err, ctx.Err())
	}
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("attached log stream exceeds %d-byte input limit; use a smaller capture", maxBytes)
	}
	return data, nil
}
