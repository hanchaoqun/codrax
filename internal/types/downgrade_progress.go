package types

// RecordDowngradeProgressDelta records a pre-complete downgrade attempt and
// converges only when the same typed blocker repeats.
func (c *EvidenceClosure) RecordDowngradeProgressDelta(lane DowngradeLane, blockerKey uint32, hardThreshold int) ProgressDecision {
	return c.RecordDowngradeProgressDeltaWithLaneChurn(lane, blockerKey, hardThreshold, false)
}

// RecordDowngradeProgressDeltaWithLaneChurn mirrors
// RecordDowngradeProgressDelta, with an explicit opt-in for lanes whose
// blocker key may legitimately churn while the underlying debt class is still a
// framework-owned form/presentation repair. This is intentionally not the
// default: evidence/location/risk gates still require the exact typed blocker
// to repeat before convergence can force-complete.
func (c *EvidenceClosure) RecordDowngradeProgressDeltaWithLaneChurn(lane DowngradeLane, blockerKey uint32, hardThreshold int, allowLaneChurn bool) ProgressDecision {
	return c.RecordDowngradeProgressDeltaWithLaneChurnThresholds(lane, blockerKey, hardThreshold, hardThreshold, allowLaneChurn)
}

// RecordDowngradeProgressDeltaWithLaneChurnThresholds is the lane-aware form
// used when an opted-in form lane should converge quickly on the exact same
// blocker but remain more conservative when the blocker key keeps changing.
// Evidence/location/risk lanes should keep using
// RecordDowngradeProgressDeltaWithLaneChurn with allowLaneChurn=false.
func (c *EvidenceClosure) RecordDowngradeProgressDeltaWithLaneChurnThresholds(lane DowngradeLane, blockerKey uint32, hardThreshold, laneChurnThreshold int, allowLaneChurn bool) ProgressDecision {
	consecutive := c.AppendDowngradeFingerprint(DowngradeFingerprint{Lane: lane, BlockerKey: blockerKey})
	decision := ShouldReplan(ProgressDelta{
		Kind:          ProgressDeltaDowngradeBlocker,
		DowngradeLane: lane,
		BlockerKey:    blockerKey,
		Consecutive:   consecutive,
		HardThreshold: hardThreshold,
	})
	if allowLaneChurn && decision.ShouldReplan {
		laneConsecutive := c.ConsecutiveDowngradeLaneTail(lane)
		if laneConsecutive >= laneChurnThreshold {
			decision = ShouldReplan(ProgressDelta{
				Kind:          ProgressDeltaDowngradeBlocker,
				DowngradeLane: lane,
				BlockerKey:    blockerKey,
				Consecutive:   laneConsecutive,
				HardThreshold: laneChurnThreshold,
			})
		}
	}
	if c != nil {
		c.mu.Lock()
		c.latestProgressDecision = decision
		c.mu.Unlock()
	}
	return decision
}

// ConsecutiveDowngradeLaneTail returns the same-lane tail length regardless of
// blocker key. It is a narrow support primitive for explicitly opted-in form
// debt lanes; ordinary convergence still uses identical lane+blocker tails.
func (c *EvidenceClosure) ConsecutiveDowngradeLaneTail(lane DowngradeLane) int {
	if c == nil || lane == "" {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	count := 0
	for i := len(c.downgradeFingerprints) - 1; i >= 0; i-- {
		if c.downgradeFingerprints[i].Lane != lane {
			break
		}
		count++
	}
	return count
}
