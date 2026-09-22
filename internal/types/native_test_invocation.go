package types

// NativeTestInvocationIndex snapshots only report-local execution identities.
// It does not grant test, path, source, verdict, or behavior-contract authority.
// Names and command text are never interpreted. A wholly legacy report retains
// its previous joins; any identified edge requires the new exact protocol.
type NativeTestInvocationIndex struct {
	commandIDs    []string
	resultIDs     []string
	commandOwners map[string]int
	identified    bool
}

func NewNativeTestInvocationIndex(report *ChangeReport) *NativeTestInvocationIndex {
	index := &NativeTestInvocationIndex{}
	if report == nil {
		return index
	}
	index.commandIDs = make([]string, len(report.ExecutedCommands))
	index.resultIDs = make([]string, len(report.TestResults))
	index.commandOwners = make(map[string]int)
	for i, command := range report.ExecutedCommands {
		id := command.InvocationID
		index.commandIDs[i] = id
		if id == "" {
			continue
		}
		index.identified = true
		if _, duplicate := index.commandOwners[id]; duplicate {
			index.commandOwners[id] = -1
		} else {
			index.commandOwners[id] = i
		}
	}
	for i, result := range report.TestResults {
		index.resultIDs[i] = result.InvocationID
		index.identified = index.identified || result.InvocationID != ""
	}
	return index
}

// Matches requires one unique command owner for an identified result. Many
// suites/results may belong to that command; duplicate command IDs are not
// resolved by arrival order. Bounds are checked even for legacy reports.
func (index *NativeTestInvocationIndex) Matches(commandIndex, resultIndex int) bool {
	if index == nil || commandIndex < 0 || commandIndex >= len(index.commandIDs) ||
		resultIndex < 0 || resultIndex >= len(index.resultIDs) {
		return false
	}
	if !index.identified {
		return true
	}
	id := index.commandIDs[commandIndex]
	return id != "" && index.resultIDs[resultIndex] == id && index.commandOwners[id] == commandIndex
}
