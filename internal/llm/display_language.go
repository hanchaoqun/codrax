package llm

import (
	"strings"
	"sync/atomic"
)

var displayLanguage atomic.Value // string

// SetDisplayLanguage configures short provider-side UX messages such as OAuth
// authorization prompts and model auto-selection notices. Empty/zh/anything
// non-English follows the repository convention and renders Chinese.
func SetDisplayLanguage(lang string) {
	displayLanguage.Store(strings.TrimSpace(lang))
}

func preferChineseDisplay() bool {
	lang, _ := displayLanguage.Load().(string)
	lang = strings.ToLower(strings.TrimSpace(lang))
	return lang == "" || !strings.HasPrefix(lang, "en")
}
