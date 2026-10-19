package traceinput

import (
	"context"
	"fmt"
)

// Called while mu is held, before reuse/preparation and before publication.
// Alias and source generation are physical properties, never member keys.
func (c *Coordinator) validateSourceSelectionLocked(ctx context.Context, spelling, canonical, member string) error {
	if prior := c.aliases[spelling]; prior != "" && prior != canonical {
		return fmt.Errorf("trace source alias changed %q; start a new run", spelling)
	}
	if member != "" {
		if entry := c.ready[preparationKey{canonical, ""}]; entry != nil && entry.queryCanonical == canonical && entry.sourceCanonical != canonical {
			return fmt.Errorf("archive member selection cannot target derived query material")
		}
	}
	if original := c.origins[canonical]; original != nil {
		// Pin a newly observed spelling even when the original is already
		// stale; retargeting that same alias must not rescue this Run.
		c.aliases[spelling] = canonical
		if err := original.ValidateOriginalSource(ctx); err != nil {
			return err
		}
	}
	return ctx.Err()
}
