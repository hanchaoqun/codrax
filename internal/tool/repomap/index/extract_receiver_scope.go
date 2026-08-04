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

type lexicalReceiverAuthorities map[lexicalReceiverScopeKey]map[string]lexicalReceiverAuthority

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

func addScopedReceiverAuthority(authorities lexicalReceiverAuthorities, scope *sitter.Node, name, typeName string) {
	key := lexicalReceiverScopeIdentity(scope)
	byName := authorities[key]
	if byName == nil {
		byName = make(map[string]lexicalReceiverAuthority)
		authorities[key] = byName
	}
	addLexicalReceiverAuthority(byName, name, typeName)
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
		if authority, declared := authorities[lexicalReceiverScopeIdentity(current)][binding]; declared {
			if authority.conflicted {
				return "", true
			}
			return authority.typeName, true
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
