package types

// AnswerRenderedSurfaces is a private render-time audit snapshot, never a model
// schema or a source of answer authority. The body slices come from the SAME
// rendering pass, after deduplication; filtering and re-rendering a document
// could otherwise resurrect content that was not actually visible.
type AnswerRenderedSurfaces struct {
	Answer    string `json:"-"`
	Primary   string `json:"-"`
	Principal string `json:"-"`
}

func (m *MutableState) SetAnswerRenderedSurfaces(s AnswerRenderedSurfaces) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answerRenderedSurfaces = &s
}

func (m *MutableState) AnswerRenderedSurfaces() *AnswerRenderedSurfaces {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.answerRenderedSurfaces == nil {
		return nil
	}
	s := *m.answerRenderedSurfaces
	return &s
}
