package contract

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInContractCheckerLiterals is
// internal/analysis/contract's marker in the glossarylint renderer
// roster (§40.52). Violation.Detail/Repair and SuspectedRoot.Reason
// become the composer's WhatFailed/WhyItFailed; the first red run
// listed two "finalizer …" reasons.
func TestNoInternalTermsInContractCheckerLiterals(t *testing.T) {
	glossarylint.RunPackageScan(t, ".", glossarylint.Policy{})
}
