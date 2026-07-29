package repl

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// PIB-7 (pi borrow — pi's prompt templates, ledger
// docs/design/pi_borrow_analysis_20260729.md §3.1 item 4): operators
// drop markdown files into <runtimeAnchor>/prompts/ and the file name
// becomes a REPL slash command (`audit.md` → `/audit`). The template
// body expands into a PLAIN ANALYSIS REQUEST — never into another
// slash command — so a repo-local template cannot invoke /approve,
// /merge, or any other side-effectful verb (repo-controlled content is
// a prompt-injection surface; see ledger §3.7 item 11). Static
// registered commands always win: templates are only consulted for
// names the canonical registry does not know.
//
// Argument substitution is the bash-style subset pi uses: $1..$9
// (quote-aware split), $@ / $ARGUMENTS (raw remainder). Unknown
// references expand to "".

type promptTemplate struct {
	Name        string
	Description string
	Body        string
	Path        string
}

var promptTemplateNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// loadPromptTemplates scans dir (non-recursive, mirrors pi) for *.md
// templates. Invalid names and collisions with built-in commands are
// skipped with a warning — a template must never shadow a registered
// verb.
func loadPromptTemplates(dir string) map[string]promptTemplate {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			logging.Warning("[repl/prompts] read templates dir %s: %v", dir, err)
		}
		return nil
	}
	canonical := map[string]bool{}
	for _, cmd := range types.CanonicalREPLCommands() {
		canonical[strings.TrimPrefix(cmd, "/")] = true
	}
	templates := map[string]promptTemplate{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		if !promptTemplateNameRE.MatchString(name) {
			logging.Warning("[repl/prompts] skip %s: template names must match %s", entry.Name(), promptTemplateNameRE.String())
			continue
		}
		if canonical[name] {
			logging.Warning("[repl/prompts] skip %s: /%s is a built-in command and cannot be shadowed", entry.Name(), name)
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			logging.Warning("[repl/prompts] read %s: %v", path, err)
			continue
		}
		description, body := splitPromptTemplateFrontmatter(string(raw))
		if strings.TrimSpace(body) == "" {
			logging.Warning("[repl/prompts] skip %s: empty template body", entry.Name())
			continue
		}
		templates[name] = promptTemplate{Name: name, Description: description, Body: body, Path: path}
	}
	return templates
}

// splitPromptTemplateFrontmatter peels an optional minimal YAML
// frontmatter block (only `description:` is consumed) off the body.
func splitPromptTemplateFrontmatter(raw string) (description, body string) {
	trimmed := strings.TrimPrefix(raw, "\uFEFF")
	if !strings.HasPrefix(trimmed, "---\n") {
		return "", raw
	}
	rest := trimmed[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", raw
	}
	front := rest[:end]
	body = strings.TrimPrefix(rest[end+len("\n---"):], "\n")
	for _, line := range strings.Split(front, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "description:"); ok {
			description = strings.TrimSpace(value)
		}
	}
	return description, body
}

// expandPromptTemplate performs the bash-style substitution subset:
// $1..$9 (quote-aware positional), $@ and $ARGUMENTS (raw remainder).
// Values are substituted literally — a "$1" inside an argument value
// is NOT re-expanded.
func expandPromptTemplate(body, argsLine string) string {
	argsLine = strings.TrimSpace(argsLine)
	positional := splitPromptTemplateArgs(argsLine)
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '$' || i+1 >= len(body) {
			b.WriteByte(c)
			continue
		}
		next := body[i+1]
		switch {
		case next == '@':
			b.WriteString(argsLine)
			i++
		case next >= '1' && next <= '9':
			idx := int(next - '1')
			if idx < len(positional) {
				b.WriteString(positional[idx])
			}
			i++
		case strings.HasPrefix(body[i+1:], "ARGUMENTS"):
			b.WriteString(argsLine)
			i += len("ARGUMENTS")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// splitPromptTemplateArgs splits the argument line into positional
// values with single/double-quote awareness ("two words" is one value).
func splitPromptTemplateArgs(line string) []string {
	var args []string
	var current strings.Builder
	inQuote := byte(0)
	flush := func() {
		if current.Len() > 0 {
			args = append(args, current.String())
			current.Reset()
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case inQuote != 0:
			if c == inQuote {
				inQuote = 0
			} else {
				current.WriteByte(c)
			}
		case c == '\'' || c == '"':
			inQuote = c
		case c == ' ' || c == '\t':
			flush()
		default:
			current.WriteByte(c)
		}
	}
	flush()
	return args
}

// expandTemplateCommand resolves `/name args` against the loaded
// templates. Only consulted AFTER the canonical registry misses (the
// caller guarantees ordering); returns the expanded request text.
func (r *REPL) expandTemplateCommand(line string) (expanded string, tpl promptTemplate, ok bool) {
	if len(r.promptTemplates) == 0 || !strings.HasPrefix(line, "/") {
		return "", promptTemplate{}, false
	}
	name := strings.TrimPrefix(line, "/")
	argsLine := ""
	if idx := strings.IndexAny(name, " \t"); idx >= 0 {
		argsLine = name[idx+1:]
		name = name[:idx]
	}
	tpl, found := r.promptTemplates[name]
	if !found {
		return "", promptTemplate{}, false
	}
	expanded = strings.TrimSpace(expandPromptTemplate(tpl.Body, argsLine))
	// Hard rule: the expansion is a plain analysis request. A template
	// body that tries to start with a slash command is neutralised so
	// repo-local content can never reach the slash dispatcher.
	expanded = strings.TrimPrefix(expanded, "/")
	return expanded, tpl, expanded != ""
}

// promptTemplateExpandedMsg is the visible disclosure line: the
// expansion source and size are shown so a repo-local template can
// never smuggle content invisibly (ledger §3.7 item 11).
func promptTemplateExpandedMsg(lang, name, path string, chars int) string {
	if isZh(lang) {
		return fmt.Sprintf("模板 /%s 展开为 %d 字符请求（来源 %s）", name, chars, path)
	}
	return fmt.Sprintf("Template /%s expanded to a %d-char request (from %s)", name, chars, path)
}

// promptTemplateNames returns the loaded names sorted, for diagnostics.
func promptTemplateNames(templates map[string]promptTemplate) []string {
	names := make([]string, 0, len(templates))
	for name := range templates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
