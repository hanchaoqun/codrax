package agent

import (
	"context"
	"errors"
	"sort"

	"github.com/hanchaoqun/codrax/internal/types"
)

func perfTriageContextError(ctx *types.AgentContext, err error) error {
	if ctx.Ctx != nil && ctx.Ctx.Err() != nil {
		return ctx.Ctx.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func validatePerfTriageParent(ctx *types.AgentContext) error {
	if err := perfTriageContextError(ctx, nil); err != nil {
		return err
	}
	if ctx.AttachedTraceMaterial != nil {
		return ctx.AttachedTraceMaterial.Validate(ctx.Ctx, ctx.AttachedHitrace)
	}
	return nil
}

func extractedPreviewUnionBytes(scopes []types.PerfObservationSourceScope) int {
	ranges := append([]types.PerfObservationSourceScope(nil), scopes...)
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].ByteStart < ranges[j].ByteStart })
	total, end := 0, 0
	for _, s := range ranges {
		start := s.ByteStart
		if start < end {
			start = end
		}
		if s.ByteEnd > start {
			total += s.ByteEnd - start
			end = s.ByteEnd
		}
	}
	return total
}
