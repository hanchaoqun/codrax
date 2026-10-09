package types

// InstallNativeTestRegistrationIdentity binds display-only source observations
// to the current private grant. It neither retains the old ChangeReport in the
// active verification lane nor installs declarations or execution receipts.
func (m *MutableState) InstallNativeTestRegistrationIdentity(authorizationID string, report *ChangeReport) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.nativeTestRegistrationAuthorization
	if g == nil || g.ID != authorizationID || g.contextDigest != m.nativeRegistrationContextDigestLocked() {
		return
	}
	g.identitySnapshot = ""
	g.identityChoices = nil
	view := nativeTestRegistrationSourceIdentities(g.Delivery.SourcePlanID, report)
	if view == nil {
		return
	}
	g.identitySnapshot, g.identityChoices = renderNativeTestIdentitySnapshotChoices(g.Delivery.SourcePlanID, view, true, g.ID)
}

// NativeTestRegistrationSourceIdentitiesAvailable is a planning capability,
// not permission or proof. The controller must separately resolve the retained
// source and unchanged contracts before offering this read-only planning lane.
func NativeTestRegistrationSourceIdentitiesAvailable(sourcePlanID string, report *ChangeReport) bool {
	view := nativeTestRegistrationSourceIdentities(sourcePlanID, report)
	return view != nil && len(view.TestResults) > 0
}

func nativeTestRegistrationSourceIdentities(sourcePlanID string, report *ChangeReport) *ChangeReport {
	if sourcePlanID == "" || report == nil || report.PlanID != sourcePlanID || report.Channel != ChangeReportChannelPostApplyVerify {
		return nil
	}
	// This registration route currently supports Python unittest only. A
	// unique producer invocation, not command prose, selects eligible rows.
	index := NewNativeTestInvocationIndex(report)
	view := ChangeReport{PlanID: report.PlanID, Channel: report.Channel, GeneratedAt: report.GeneratedAt}
	for i, row := range report.TestResults {
		if !nativeTestIdentitySnapshotEligible(row) || row.InvocationID == "" {
			continue
		}
		for j, command := range report.ExecutedCommands {
			if command.Runner == "python" && command.Framework == "unittest" && command.ProbeExecution == nil &&
				command.SourceCheckExecution == nil && index.Matches(j, i) {
				view.TestResults = append(view.TestResults, row)
				break
			}
		}
	}
	return &view
}

// NativeTestIdentityChoice selects only a producer-published identity pair.
// It deliberately has no test path, contract mapping, or execution receipt.
type NativeTestIdentityChoice struct {
	Ref            string `json:"assertion_ref"`
	AssertionSuite string `json:"assertion_suite"`
	AssertionID    string `json:"assertion_id"`
}

// NativeTestRegistrationIdentityChoices shares the exact bounded rows shown
// to this live registration dispatch. JSON/history cannot restore selectors.
func (m *MutableState) NativeTestRegistrationIdentityChoices(root string) []NativeTestIdentityChoice {
	g := m.NativeTestRegistrationAuthorization()
	if g == nil || g.RepositoryRoot != root {
		return nil
	}
	return append([]NativeTestIdentityChoice(nil), g.identityChoices...)
}

// NativeTestRegistrationIdentitySnapshot is available only for the grant's
// repository and live context. Clearing/replacing/revoking that grant also
// clears this view; persisted workflows cannot resurrect it themselves.
func (m *MutableState) NativeTestRegistrationIdentitySnapshot(root string) string {
	g := m.NativeTestRegistrationAuthorization()
	if g == nil || g.RepositoryRoot != root {
		return ""
	}
	return g.identitySnapshot
}
