package traceinput

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

// A disk-spool bound, independent of the model preview. Larger captures can
// still use a regular file. It matches the existing compressed-input ceiling.
const DefaultStreamMaxBytes = int64(64 << 30)

type StreamOptions struct {
	Options
	MaxBytes int64
}

type streamReceipt struct {
	Version          string `json:"version"`
	SourcePath       string `json:"source_path"`
	SourceBytes      int64  `json:"source_bytes"`
	SourceSHA256     string `json:"source_sha256"`
	SourceGeneration string `json:"source_generation"`
	InputByteLimit   int64  `json:"input_byte_limit"`
	EOFConfirmed     bool   `json:"eof_confirmed"`
}

// BeginStream takes exclusive ownership of input, including on failure. Close
// must unblock Read (as with a pipe or socket). No converter sees bytes until
// EOF has been observed. Cancellation closes the reader; it is not a timeout.
// The caller must defer Discard and publish only after Commit succeeds.
func BeginStream(ctx context.Context, input io.ReadCloser, opts StreamOptions) (*Preparation, error) {
	return beginStream(ctx, input, opts, hitraceconv.PrepareFile)
}

func PrepareStream(ctx context.Context, input io.ReadCloser, opts StreamOptions) (*attachment.TraceMaterial, error) {
	p, err := BeginStream(ctx, input, opts)
	if err != nil {
		return nil, err
	}
	defer p.Discard()
	return p.Commit(ctx)
}

func beginStream(ctx context.Context, input io.ReadCloser, opts StreamOptions, convert converter) (pending *Preparation, err error) {
	if input == nil {
		return nil, fmt.Errorf("trace stream reader is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var once sync.Once
	var closeErr error
	closeInput := func() { once.Do(func() { closeErr = input.Close() }) }
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			closeInput()
		case <-done:
		}
	}()
	defer func() {
		close(done)
		<-stopped
		closeInput()
		err = errors.Join(err, closeErr, ctx.Err())
		if err != nil && pending != nil {
			err = errors.Join(err, pending.Discard())
			pending = nil
		}
	}()
	if opts.MaxBytes == 0 {
		opts.MaxBytes = DefaultStreamMaxBytes
	}
	if opts.MaxBytes < 0 || opts.MaxBytes > DefaultStreamMaxBytes || opts.PreviewBytes < 0 || opts.InputPath != "" {
		return nil, fmt.Errorf("invalid trace stream limits or conflicting input path")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owned, err := newManagedDirectory(opts.RuntimeAnchor, opts.RuntimeAnchorFallback)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, owned.cleanup())
		}
	}()
	receipt, err := spoolTraceStream(ctx, input, owned, opts.MaxBytes)
	if err != nil {
		return nil, err
	}
	// Close before conversion: a late transport failure cannot publish an
	// otherwise parseable prefix, and the cancellation watcher is joined above.
	closeInput()
	if closeErr != nil {
		return nil, closeErr
	}
	receiptFile, err := owned.authority.CreateFile("input-stream.json")
	if err != nil {
		return nil, err
	}
	encodeErr := json.NewEncoder(receiptFile).Encode(receipt)
	id, idErr := filegeneration.FromFile(receiptFile)
	if err := errors.Join(encodeErr, idErr, receiptFile.Close()); err != nil {
		return nil, err
	}
	opts.Options.InputPath = receipt.SourcePath
	opts.Options.streamDirectory = owned
	opts.Options.streamReceiptPath = filepath.Join(owned.path, "input-stream.json")
	opts.Options.streamReceiptID = id
	opts.Options.streamSourceGeneration = receipt.SourceGeneration
	return begin(ctx, opts.Options, convert)
}

func spoolTraceStream(ctx context.Context, input io.Reader, owned *managedDirectory, limit int64) (receipt streamReceipt, err error) {
	file, err := owned.authority.CreateFile("input.trace")
	if err != nil {
		return receipt, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	hash := sha256.New()
	buf := make([]byte, 64<<10)
	var count int64
	idle := 0
	for {
		if err := ctx.Err(); err != nil {
			return receipt, err
		}
		size := int64(len(buf))
		if left := limit - count + 1; size > left {
			size = left
		}
		n, readErr := input.Read(buf[:size])
		if int64(n) > limit-count {
			return receipt, fmt.Errorf("trace stream exceeds %d-byte input limit; use a complete file path", limit)
		}
		if n > 0 {
			if _, err := file.Write(buf[:n]); err != nil {
				return receipt, err
			}
			_, _ = hash.Write(buf[:n])
			count += int64(n)
			idle = 0
		} else {
			idle++
			if idle >= 100 && readErr == nil {
				return receipt, io.ErrNoProgress
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return receipt, readErr
		}
	}
	if count == 0 {
		return receipt, fmt.Errorf("trace stream is empty")
	}
	if err := errors.Join(ctx.Err(), file.Sync(), owned.validate()); err != nil {
		return receipt, err
	}
	id, err := filegeneration.FromFile(file)
	if err != nil {
		return receipt, err
	}
	return streamReceipt{Version: "traceinput-stream-v1", SourcePath: filepath.Join(owned.path, "input.trace"), SourceBytes: count,
		SourceSHA256: fmt.Sprintf("%x", hash.Sum(nil)), SourceGeneration: id.CacheToken(), InputByteLimit: limit, EOFConfirmed: true}, nil
}
