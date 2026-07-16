package hitraceconv

// rawPerfTestQueryableAdmission returns the canonical admission face for a
// fixture whose structurally accepted SAMPLE records are all queryable. Tests
// exercising rejected candidates build their reason census explicitly.
func rawPerfTestQueryableAdmission(rows uint64) RawPerfSampleAdmission {
	admission := newRawPerfSampleAdmission()
	admission.Candidates = rows
	admission.QueryRows = rows
	return admission
}
