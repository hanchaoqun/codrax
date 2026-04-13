// One-shot diagnostic: parse a Go source file with tree-sitter-go and
// dump the AST shape of each import_declaration so we can verify
// whether grouped imports (import ( ... )) nest under import_spec_list
// or directly under import_declaration. Delete after use.
//
// Run:
//
//	go run ./eval/repomap_v3/probe_ast <file.go>
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	tsgo "github.com/smacker/go-tree-sitter/golang"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: probe_ast <file.go>")
		os.Exit(2)
	}
	path := os.Args[1]
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	parser := sitter.NewParser()
	parser.SetLanguage(tsgo.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	root := tree.RootNode()
	for i := 0; i < int(root.NamedChildCount()); i++ {
		ch := root.NamedChild(i)
		if ch.Type() != "import_declaration" {
			continue
		}
		fmt.Printf("== import_declaration @ line %d\n", ch.StartPoint().Row+1)
		dump(ch, src, 0)
	}
}

func dump(n *sitter.Node, src []byte, depth int) {
	prefix := strings.Repeat("  ", depth)
	text := string(src[n.StartByte():n.EndByte()])
	if len(text) > 60 {
		text = text[:57] + "..."
	}
	text = strings.ReplaceAll(text, "\n", "\\n")
	fmt.Printf("%s%s  %q\n", prefix, n.Type(), text)
	for i := 0; i < int(n.NamedChildCount()); i++ {
		dump(n.NamedChild(i), src, depth+1)
	}
}
