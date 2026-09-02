package repl

// Delivery errors stay out of Run's analysis-error lane so the REPL can still
// render the original answer. This warning is terminal chrome, not answer text.
func (r *REPL) warnRootCauseOutputFailure() {
	reporter, ok := r.runner.(interface{ RootCauseOutputError() error })
	if !ok {
		return
	}
	if err := reporter.RootCauseOutputError(); err != nil {
		if isZh(r.language) {
			r.warn("根因 JSON 文件写入失败；分析答案不受影响：%v\n", err)
		} else {
			r.warn("Root-cause JSON write failed; the analysis answer is unchanged: %v\n", err)
		}
	}
}
