package attachment

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// ReadTextFileLimited validates a fixed safety prefix and the complete
// publishable payload from one held regular-file generation. payloadLimit
// controls only returned bytes; it never shrinks the admission probe.
func ReadTextFileLimited(kind Kind, path string, payloadLimit int) (data []byte, truncated bool, err error) {
	if payloadLimit < 0 {
		return nil, false, fmt.Errorf("read attached %s %q: payload limit is negative", kind, path)
	}
	file, err := openTextSourceFile(path)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			data = nil
			truncated = false
			err = errors.Join(err, fmt.Errorf("close attached %s %q: %w", kind, path, closeErr))
		}
	}()

	opened, err := filegeneration.FromFile(file)
	if err != nil {
		return nil, false, fmt.Errorf("inspect attached %s %q: %w", kind, path, err)
	}
	if !opened.Mode().IsRegular() || opened.Size() < 0 {
		return nil, false, fmt.Errorf("attached %s source is not a regular file: %q", kind, path)
	}
	var validationErr error
	if kind == KindTrace {
		validationErr = ValidateTextReaderAtFull(context.Background(), kind, path, file, opened.Size(), TracePhysicalLineMaxBytes)
	} else {
		validationErr = ValidateTextReaderAt(kind, path, file, opened.Size())
	}
	provisionalErr := validationErr
	if provisionalErr == nil {
		readBytes := opened.Size()
		if limit := int64(payloadLimit); readBytes > limit {
			readBytes = limit
		}
		if readBytes < 0 {
			provisionalErr = fmt.Errorf("read attached %s %q: payload size overflow", kind, path)
		} else {
			raw, readErr := io.ReadAll(io.NewSectionReader(file, 0, readBytes))
			switch {
			case readErr != nil:
				provisionalErr = fmt.Errorf("read attached %s %q: %w", kind, path, readErr)
			case int64(len(raw)) != readBytes:
				provisionalErr = fmt.Errorf("read attached %s %q: short frozen read got=%d want=%d", kind, path, len(raw), readBytes)
			default:
				truncated = opened.Size() > int64(payloadLimit)
				data, provisionalErr = ValidatePublishableText(kind, path, raw, truncated)
			}
		}
	}

	final, identityErr := filegeneration.FromFile(file)
	if identityErr == nil && !opened.SameVersion(final) {
		identityErr = fmt.Errorf("attached %s source changed while being validated/read: %q", kind, path)
	}
	bound, bindingErr := filegeneration.FromPath(path)
	if bindingErr == nil && !opened.SameVersion(bound) {
		bindingErr = fmt.Errorf("attached %s source path was replaced while being validated/read: %q", kind, path)
	}
	if authoritativeErr := authoritativeTextReadError(identityErr, bindingErr, provisionalErr); authoritativeErr != nil {
		return nil, false, authoritativeErr
	}
	return data, truncated, nil
}

func authoritativeTextReadError(identityErr, bindingErr, provisionalErr error) error {
	if identityErr != nil || bindingErr != nil {
		// Mixed-generation bytes have no format authority. Do not join a
		// provisional TextIssue here: errors.As would otherwise render a binary
		// conversion prescription derived from bytes whose identity already
		// failed. The generation/path verdict deliberately dominates.
		return errors.Join(identityErr, bindingErr)
	}
	return provisionalErr
}
