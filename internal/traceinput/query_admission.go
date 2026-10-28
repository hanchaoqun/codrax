package traceinput

import (
	"context"
	"errors"
	"io"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/pcapinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// ValidateQueryMaterial admits the reader selected by physical preparation.
// A packet header is navigation, not completed packet statistics; the native
// query owns record validation, partial/empty status and packet facts. Text
// and bundles retain their existing complete-universe admission unchanged.
func ValidateQueryMaterial(ctx context.Context, material *attachment.TraceMaterial, preview string) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := material.Validate(ctx, preview); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, material.Validate(ctx, preview)) }()
	if material.QueryDomain() != attachment.TraceQueryDomainPacketCapture {
		return tracequery.ValidateTraceInputPath(ctx, material.QueryPath())
	}
	held, identity, err := filegeneration.OpenRegularReadOnly(material.QueryPath())
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, validateHeld(material.QueryPath(), held, identity), ctx.Err(), held.Close())
	}()
	var header [24]byte
	n, _ := io.ReadFull(held, header[:])
	_, err = pcapinput.ParseHeader(header[:n])
	return err
}
