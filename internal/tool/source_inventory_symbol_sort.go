package tool

import (
	"sort"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func (index *sourceInventoryGraphSymbolIndex) sort() {
	if index == nil {
		return
	}
	sourceInventorySortSymbols(index.all)
	for key := range index.byFile {
		sourceInventorySortSymbols(index.byFile[key])
	}
	for key := range index.byDir {
		sourceInventorySortSymbols(index.byDir[key])
	}
}

func sourceInventorySortSymbols(symbols []*repotypes.Symbol) {
	sort.SliceStable(symbols, func(i, j int) bool {
		if symbols[i] == nil || symbols[j] == nil {
			return symbols[j] != nil
		}
		if symbols[i].File != symbols[j].File {
			return symbols[i].File < symbols[j].File
		}
		if symbols[i].Line != symbols[j].Line {
			return symbols[i].Line < symbols[j].Line
		}
		if symbols[i].Name != symbols[j].Name {
			return symbols[i].Name < symbols[j].Name
		}
		return symbols[i].Kind < symbols[j].Kind
	})
}
