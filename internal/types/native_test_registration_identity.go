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
	if report == nil || report.PlanID != g.Delivery.SourcePlanID || report.Channel != ChangeReportChannelPostApplyVerify {
		return
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
	g.identitySnapshot = renderNativeTestIdentitySnapshot(g.Delivery.SourcePlanID, &view, true)
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
