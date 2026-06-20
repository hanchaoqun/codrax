package sourceinventory

import (
	"path"
	"sort"
	"strings"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

type ScopeMatcher func(file, scope string) bool

type ExecutionViewOptions struct {
	Graph        *repotypes.Graph
	Scopes       []string
	ScopeMatcher ScopeMatcher
	Budget       *Budget
}

// ExecutionView is the bounded, sorted, scope-aware file universe for one
// source-inventory pass. Candidate builders decide which rows to produce; this
// kernel owns file materialization, language indexing, truncation state, and
// scope membership so each role/lens does not rebuild its own file slice.
type ExecutionView struct {
	graph             *repotypes.Graph
	scopes            []string
	files             []*repotypes.FileInfo
	filesByLanguage   map[string][]*repotypes.FileInfo
	fileSet           map[string]bool
	hasInventoryFiles bool
	budgetTruncated   bool
}

func NewExecutionView(options ExecutionViewOptions) *ExecutionView {
	view := &ExecutionView{
		graph:           options.Graph,
		scopes:          DedupeScopeAliases(options.Scopes),
		filesByLanguage: map[string][]*repotypes.FileInfo{},
		fileSet:         map[string]bool{},
	}
	if options.Graph == nil {
		return view
	}
	scanned := 0
	for _, fi := range options.Graph.Files {
		if fi == nil {
			continue
		}
		rel := NormalizeFileSurface(fi.RelPath)
		if rel == "" || !FileInScopes(rel, view.scopes, options.ScopeMatcher) || view.fileSet[rel] {
			continue
		}
		if options.Budget != nil && options.Budget.ScanExceeded(scanned) {
			view.budgetTruncated = true
			break
		}
		scanned++
		view.fileSet[rel] = true
		view.files = append(view.files, fi)
		if strings.TrimSpace(fi.Language) != "" || fi.IsSpecial {
			view.hasInventoryFiles = true
		}
	}
	sort.Slice(view.files, func(i, j int) bool {
		return strings.TrimSpace(view.files[i].RelPath) < strings.TrimSpace(view.files[j].RelPath)
	})
	return view
}

func (view *ExecutionView) Files(language string) []*repotypes.FileInfo {
	if view == nil {
		return nil
	}
	language = strings.TrimSpace(language)
	if language == "" {
		return append([]*repotypes.FileInfo(nil), view.files...)
	}
	if files, ok := view.filesByLanguage[language]; ok {
		return append([]*repotypes.FileInfo(nil), files...)
	}
	files := make([]*repotypes.FileInfo, 0)
	for _, fi := range view.files {
		if fi != nil && strings.TrimSpace(fi.Language) == language {
			files = append(files, fi)
		}
	}
	view.filesByLanguage[language] = files
	return append([]*repotypes.FileInfo(nil), files...)
}

func (view *ExecutionView) HasInventoryFiles() bool {
	return view != nil && view.hasInventoryFiles
}

func (view *ExecutionView) BudgetTruncated() bool {
	return view != nil && view.budgetTruncated
}

func (view *ExecutionView) Complete() bool {
	return view != nil && view.hasInventoryFiles && !view.budgetTruncated
}

func (view *ExecutionView) ContainsFile(file string) bool {
	rel := NormalizeFileSurface(file)
	return rel != "" && view != nil && view.fileSet[rel]
}

func (view *ExecutionView) Scopes() []string {
	if view == nil {
		return nil
	}
	return append([]string(nil), view.scopes...)
}

func NormalizeFileSurface(file string) string {
	return strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
}

func FileInScopes(file string, scopes []string, matcher ScopeMatcher) bool {
	file = NormalizeFileSurface(file)
	if file == "" {
		return false
	}
	for _, scope := range scopes {
		scope = NormalizeFileSurface(scope)
		if scope == "" || scope == "." {
			return true
		}
		if file == scope || strings.HasPrefix(file, scope+"/") || (matcher != nil && matcher(file, scope)) {
			return true
		}
	}
	return false
}

func DedupeScopeAliases(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = NormalizeFileSurface(scope)
		if scope == "" {
			scope = "."
		}
		scope = path.Clean(scope)
		if scope == "." || scope == "/" {
			scope = "."
		}
		scope = strings.Trim(scope, "/")
		if scope == "" {
			scope = "."
		}
		if seen[scope] {
			continue
		}
		seen[scope] = true
		out = append(out, scope)
	}
	return out
}
