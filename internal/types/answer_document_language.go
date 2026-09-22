package types

import "strings"

// AnswerDocumentLanguageSource records which existing authority selected the
// bilingual renderer locale. It is display metadata, not model-owned content.
type AnswerDocumentLanguageSource string

const (
	AnswerDocumentLanguageDefault          AnswerDocumentLanguageSource = "default"
	AnswerDocumentLanguageDisabled         AnswerDocumentLanguageSource = "disabled"
	AnswerDocumentLanguageConfigured       AnswerDocumentLanguageSource = "configured"
	AnswerDocumentLanguageAnalysisContract AnswerDocumentLanguageSource = "analysis_contract"
	AnswerDocumentLanguageAnalysisRequest  AnswerDocumentLanguageSource = "analysis_request"
)

// ResolveAnswerDocumentLanguage keeps finalizer teaching and system-authored
// additions on the same locale. A concrete project/CLI setting wins. off/none
// disables language teaching, but system copy still needs the English renderer
// fallback. Empty, auto/follow and unsupported codes use structured analysis,
// then English; this does not add translation support for other languages or
// inspect, translate, or mutate a request, answer, or analysis contract.
func ResolveAnswerDocumentLanguage(configured, contract, request string) (string, AnswerDocumentLanguageSource) {
	if lang := normalizeAnswerDocumentLanguage(configured); lang != "" {
		return lang, AnswerDocumentLanguageConfigured
	}
	switch strings.ToLower(strings.TrimSpace(configured)) {
	case "off", "none":
		return "en", AnswerDocumentLanguageDisabled
	}
	if lang := normalizeAnswerDocumentLanguage(contract); lang != "" {
		return lang, AnswerDocumentLanguageAnalysisContract
	}
	if lang := normalizeAnswerDocumentLanguage(request); lang != "" {
		return lang, AnswerDocumentLanguageAnalysisRequest
	}
	return "en", AnswerDocumentLanguageDefault
}

func normalizeAnswerDocumentLanguage(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh", "zh-cn", "cn", "chinese", "简体中文":
		return "zh"
	case "en", "en-us", "english":
		return "en"
	}
	return ""
}
