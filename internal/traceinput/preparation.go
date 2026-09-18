package traceinput

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

// Preparation holds a fully prepared but unpublished attachment. Unlike
// Prepare, Begin retains directory authority until Commit or Discard. This
// permits a product boundary to cancel after conversion without leaking its
// unpublished outputs or deleting by a pathname. It confers no query authority
// until Commit returns the material. Callers must defer Discard after Begin.
type Preparation struct {
	mu       sync.Mutex
	ctx      context.Context
	material *attachment.TraceMaterial
	owned    *managedDirectory
	finished bool
	err      error
}

func Begin(ctx context.Context, opts Options) (*Preparation, error) {
	return begin(ctx, opts, hitraceconv.ConvertFile)
}

func begin(ctx context.Context, opts Options, convert converter) (*Preparation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	p := &Preparation{ctx: ctx}
	material, err := prepareWithOwnership(ctx, opts, convert, func(owned *managedDirectory) {
		p.owned = owned
	})
	if err != nil {
		return nil, err // preparation itself already rolled back its authority
	}
	p.material = material
	return p, nil
}

// Commit validates the original and every derived generation again, then
// releases cleanup authority. The caller must serialize cancellation and
// publication around this call if it needs an atomic UI operation boundary.
// Cancellation after this commit point cannot revoke a published attachment.
// Repeated Commit never reissues authority, including after a failed commit.
func (p *Preparation) Commit(ctx context.Context) (*attachment.TraceMaterial, error) {
	if p == nil {
		return nil, fmt.Errorf("trace preparation is nil")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return nil, errors.Join(fmt.Errorf("trace preparation already finished"), p.err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if p.ctx == nil || p.material == nil {
		p.err = fmt.Errorf("trace preparation was not initialized by Begin")
	} else {
		p.err = errors.Join(p.ctx.Err(), p.material.Validate(ctx, p.material.Preview()))
		p.err = errors.Join(p.err, p.ctx.Err(), ctx.Err())
	}
	if p.err == nil && p.owned != nil {
		p.err = p.owned.close()
	}
	p.finished = true
	if p.err != nil {
		p.err = errors.Join(p.err, p.cleanup())
		p.material = nil
		return nil, p.err
	}
	material := p.material
	p.material = nil
	return material, nil
}

// Discard is safe in a defer, after Commit, or more than once. Failed cleanup
// is terminal and stays visible: no retry can reacquire deletion authority by
// path, even if a swapped directory later appears at the original location.
func (p *Preparation) Discard() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.finished {
		p.finished = true
		p.material = nil
		p.err = p.cleanup()
	}
	return p.err
}

func (p *Preparation) cleanup() error {
	if p.owned == nil {
		return nil // never delete the caller's original text file
	}
	return p.owned.cleanup()
}
