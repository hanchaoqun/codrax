package orchestrator

import "strings"

func runtimeAggregateSurfaceTokenSetSupportsLabel(surface, label string) bool {
	surfaceTokens := runtimeArtifactSupportTokenSet(surface)
	if len(surfaceTokens) == 0 {
		return false
	}
	labelTokens := runtimeArtifactSupportTokens(label)
	if len(labelTokens) == 0 {
		return false
	}
	matched := 0
	for _, tok := range labelTokens {
		if _, ok := surfaceTokens[tok]; !ok {
			return false
		}
		matched++
	}
	return matched > 0
}

func runtimeArtifactSupportTokenSet(raw string) map[string]struct{} {
	tokens := runtimeArtifactSupportTokens(raw)
	if len(tokens) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(tokens))
	for _, tok := range tokens {
		out[tok] = struct{}{}
	}
	return out
}

func runtimeArtifactSupportTokens(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(s string) {
		key := asciiCodeSupportKey(s)
		if len(key) < 4 {
			return
		}
		if stripped := runtimeNumericUnitSupportToken(key); stripped != "" {
			addRuntimeArtifactSupportToken(stripped, seen, &out)
			return
		}
		addRuntimeArtifactSupportToken(key, seen, &out)
	}
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		add(b.String())
		b.Reset()
	}
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_',
			r == '-':
			b.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return out
}

func addRuntimeArtifactSupportToken(key string, seen map[string]struct{}, out *[]string) {
	if key == "" || len(key) < 4 {
		return
	}
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*out = append(*out, key)
}

func runtimeNumericUnitSupportToken(key string) string {
	key = strings.TrimSpace(key)
	if len(key) < 5 {
		return ""
	}
	for _, suffix := range []string{"ms", "us", "ns", "s"} {
		if !strings.HasSuffix(key, suffix) || len(key) <= len(suffix) {
			continue
		}
		stem := strings.TrimSuffix(key, suffix)
		if len(stem) < 4 || !asciiDigitsOnly(stem) {
			continue
		}
		return stem
	}
	return ""
}

func asciiDigitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
