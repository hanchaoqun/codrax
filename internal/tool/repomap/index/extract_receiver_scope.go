package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// lexicalReceiverAuthorities records the only declaration identities allowed
// to promote a source receiver to a concrete type. Language extractors retain
// ownership of declaration and scope grammar; this carrier only centralizes
// nearest-scope shadowing and conflict semantics.
type lexicalReceiverScopeKey struct {
	nodeType  string
	startByte uint32
	endByte   uint32
}

type lexicalReceiverAuthority struct {
	typeName   string
	conflicted bool
}

type lexicalReceiverDeclaration struct {
	authority lexicalReceiverAuthority
	startByte uint32
	scopeWide bool
}

type lexicalReceiverAuthorities map[lexicalReceiverScopeKey]map[string][]lexicalReceiverDeclaration

func addLexicalReceiverAuthority(byName map[string]lexicalReceiverAuthority, name, typeName string) {
	name = strings.TrimSpace(name)
	typeName = strings.TrimSpace(typeName)
	if name == "" {
		return
	}
	authority, exists := byName[name]
	if authority.conflicted {
		return
	}
	if typeName == "" || (exists && authority.typeName != typeName) {
		byName[name] = lexicalReceiverAuthority{conflicted: true}
		return
	}
	byName[name] = lexicalReceiverAuthority{typeName: typeName}
}

func addScopedReceiverAuthority(authorities lexicalReceiverAuthorities, scope, declaration *sitter.Node, name, typeName string, scopeWide bool) {
	name = strings.TrimSpace(name)
	typeName = strings.TrimSpace(typeName)
	if name == "" {
		return
	}
	key := lexicalReceiverScopeIdentity(scope)
	byName := authorities[key]
	if byName == nil {
		byName = make(map[string][]lexicalReceiverDeclaration)
		authorities[key] = byName
	}
	startByte := uint32(0)
	if declaration != nil {
		startByte = declaration.StartByte()
	}
	byName[name] = append(byName[name], lexicalReceiverDeclaration{
		authority: lexicalReceiverAuthority{typeName: typeName, conflicted: typeName == ""},
		startByte: startByte,
		scopeWide: scopeWide,
	})
}

func lexicalReceiverTypeAt(node *sitter.Node, binding string, authorities lexicalReceiverAuthorities, isBoundary func(*sitter.Node) bool) (string, bool) {
	binding = strings.TrimSpace(binding)
	if node == nil || binding == "" {
		return "", false
	}
	for current := node; current != nil; current = current.Parent() {
		if !isBoundary(current) && current.Parent() != nil {
			continue
		}
		declarations := authorities[lexicalReceiverScopeIdentity(current)][binding]
		latestStart := uint32(0)
		var active []lexicalReceiverAuthority
		for _, declaration := range declarations {
			if !declaration.scopeWide && declaration.startByte > node.StartByte() {
				continue
			}
			start := declaration.startByte
			if declaration.scopeWide {
				start = 0
			}
			if len(active) == 0 || start > latestStart {
				latestStart = start
				active = active[:0]
			}
			if start == latestStart {
				active = append(active, declaration.authority)
			}
		}
		if len(active) != 0 {
			typeName := ""
			for _, authority := range active {
				if authority.conflicted || (typeName != "" && typeName != authority.typeName) {
					return "", true
				}
				typeName = authority.typeName
			}
			if typeName == "" {
				return "", true
			}
			return typeName, true
		}
	}
	return "", false
}

func lexicalReceiverBindingScope(node, root *sitter.Node, isBoundary func(*sitter.Node) bool) *sitter.Node {
	for current := node.Parent(); current != nil; current = current.Parent() {
		if isBoundary(current) {
			return current
		}
	}
	return root
}

func lexicalReceiverScopeIdentity(node *sitter.Node) lexicalReceiverScopeKey {
	if node == nil {
		return lexicalReceiverScopeKey{}
	}
	return lexicalReceiverScopeKey{nodeType: node.Type(), startByte: node.StartByte(), endByte: node.EndByte()}
}
