package types

// NativeDisplayDataKey identifies only an already qualified complete display
// body. It never replaces query, whole-publication, population or causal
// identity. The caller must retain the source heading and every table note.
// JSON, mutation, missing current source, unknown windows and other producers
// keep the ordinary full-table path. No path is reopened during rendering.
func (t RuntimeMeasurementTable) NativeDisplayDataKey() string {
	if t.nativeDisplayDataIdentity == "" || t.MemberSet != nil || t.DisplayPopulation != nil || t.EventData != nil {
		return ""
	}
	basis, valid := t.NativeSourceTitleBasis()
	if !valid || basis.OriginalSource == "" || basis.QuerySourceVersion == "" || basis.QueryWindow == "" {
		return ""
	}
	return runtimeSamplingDigest(struct {
		Data   string
		Source RuntimeMeasurementSourceTitleBasis
	}{t.nativeDisplayDataIdentity, basis})
}
