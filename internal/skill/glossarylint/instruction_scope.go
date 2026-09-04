package glossarylint

import (
	"go/ast"
	"go/token"
)

// instruction_scope.go — lexical name resolution for the instruction
// walker. Locals are resolved by block scope and position, not by name
// alone, so a `for _, hint := range operatorHints` loop variable never
// aliases the `hint` local that feeds a prompt three blocks away in the
// same function (the shape that made the first name-keyed draft union the
// REPL's operator nudges into the classifier's user blob).

// varDecl is one declared name: a parameter, receiver, named result,
// closure parameter, or a local with every value ever assigned to it.
type varDecl struct {
	name   string
	end    token.Pos // the declaration is visible at positions >= end
	values []binding
	typ    string // declared or literal type name ("" when unknown)
	param  *paramInfo
	result bool
	data   bool // receiver / closure parameter: runtime data by declaration
}

// scopeNode is one lexical block with the names it declares.
type scopeNode struct {
	pos, end token.Pos
	parent   *scopeNode
	children []*scopeNode
	decls    map[string][]*varDecl
}

func (s *scopeNode) child(n ast.Node) *scopeNode {
	c := &scopeNode{pos: n.Pos(), end: n.End(), parent: s, decls: map[string][]*varDecl{}}
	s.children = append(s.children, c)
	return c
}

func (s *scopeNode) declare(d *varDecl) *varDecl {
	s.decls[d.name] = append(s.decls[d.name], d)
	return d
}

// fnScope is the scope tree of one function with its parameter and
// named-result declarations by position.
type fnScope struct {
	root    *scopeNode
	params  []*varDecl // by index; nil for an unnamed parameter
	results []*varDecl
}

// lookup resolves name at pos to the innermost declaration visible
// there, or nil for a package-level or unknown name.
func (sc *fnScope) lookup(name string, pos token.Pos) *varDecl {
	s := sc.root
	for {
		descended := false
		for _, c := range s.children {
			if pos >= c.pos && pos < c.end {
				s = c
				descended = true
				break
			}
		}
		if !descended {
			break
		}
	}
	for ; s != nil; s = s.parent {
		var found *varDecl
		for _, d := range s.decls[name] {
			if d.end <= pos && (found == nil || d.end > found.end) {
				found = d
			}
		}
		if found != nil {
			return found
		}
	}
	return nil
}

func (idx *packageIndex) scope(fn *ast.FuncDecl) *fnScope {
	if sc, ok := idx.scopes[fn]; ok {
		return sc
	}
	sc := &fnScope{root: &scopeNode{pos: fn.Pos(), end: fn.End(), decls: map[string][]*varDecl{}}}
	idx.scopes[fn] = sc
	visible := fn.Type.End()
	if fn.Recv != nil {
		for _, f := range fn.Recv.List {
			for _, name := range f.Names {
				sc.root.declare(&varDecl{name: name.Name, end: visible, typ: recvTypeName(fn), data: true})
			}
		}
	}
	if fn.Type.Params != nil {
		i := 0
		for _, f := range fn.Type.Params.List {
			_, variadic := f.Type.(*ast.Ellipsis)
			if len(f.Names) == 0 {
				sc.params = append(sc.params, nil)
				i++
				continue
			}
			for _, name := range f.Names {
				info := paramInfo{index: i, variadic: variadic}
				d := sc.root.declare(&varDecl{name: name.Name, end: visible, typ: typeNameOf(f.Type), param: &info})
				sc.params = append(sc.params, d)
				i++
			}
		}
	}
	if fn.Type.Results != nil {
		for _, f := range fn.Type.Results.List {
			for _, name := range f.Names {
				sc.results = append(sc.results, sc.root.declare(&varDecl{name: name.Name, end: visible, typ: typeNameOf(f.Type), result: true}))
			}
		}
	}
	if fn.Body != nil {
		// The body shares the function's outermost scope in Go.
		for _, stmt := range fn.Body.List {
			sc.visit(stmt, sc.root)
		}
	}
	return sc
}

// visit records the declarations and assignments of n in scope s,
// opening child scopes for the block-like statements.
func (sc *fnScope) visit(n ast.Node, s *scopeNode) {
	if n == nil {
		return
	}
	switch v := n.(type) {
	case *ast.BlockStmt:
		c := s.child(v)
		for _, stmt := range v.List {
			sc.visit(stmt, c)
		}
	case *ast.IfStmt:
		c := s.child(v)
		sc.visit(v.Init, c)
		sc.visit(v.Cond, c)
		sc.visit(v.Body, c)
		sc.visit(v.Else, c)
	case *ast.ForStmt:
		c := s.child(v)
		sc.visit(v.Init, c)
		sc.visit(v.Cond, c)
		sc.visit(v.Post, c)
		sc.visit(v.Body, c)
	case *ast.RangeStmt:
		sc.visit(v.X, s)
		c := s.child(v)
		for _, e := range []ast.Expr{v.Key, v.Value} {
			id, ok := e.(*ast.Ident)
			if !ok || id.Name == "_" {
				continue
			}
			if v.Tok == token.DEFINE {
				c.declare(&varDecl{name: id.Name, end: v.Body.Pos(), values: []binding{{expr: v.X, idx: -1}}})
			} else if d := sc.lookup(id.Name, id.Pos()); d != nil {
				d.values = append(d.values, binding{expr: v.X, idx: -1})
			}
		}
		sc.visit(v.Body, c)
	case *ast.SwitchStmt:
		c := s.child(v)
		sc.visit(v.Init, c)
		sc.visit(v.Tag, c)
		sc.visit(v.Body, c)
	case *ast.TypeSwitchStmt:
		c := s.child(v)
		sc.visit(v.Init, c)
		if as, ok := v.Assign.(*ast.AssignStmt); ok && as.Tok == token.DEFINE && len(as.Lhs) == 1 && len(as.Rhs) == 1 {
			if id, ok := as.Lhs[0].(*ast.Ident); ok && id.Name != "_" {
				sc.visit(as.Rhs[0], c)
				c.declare(&varDecl{name: id.Name, end: as.End(), values: []binding{{expr: as.Rhs[0], idx: -1}}})
			}
		} else {
			sc.visit(v.Assign, c)
		}
		sc.visit(v.Body, c)
	case *ast.CaseClause:
		c := s.child(v)
		for _, e := range v.List {
			sc.visit(e, c)
		}
		for _, stmt := range v.Body {
			sc.visit(stmt, c)
		}
	case *ast.SelectStmt:
		sc.visit(v.Body, s)
	case *ast.CommClause:
		c := s.child(v)
		sc.visit(v.Comm, c)
		for _, stmt := range v.Body {
			sc.visit(stmt, c)
		}
	case *ast.LabeledStmt:
		sc.visit(v.Stmt, s)
	case *ast.DeclStmt:
		gd, ok := v.Decl.(*ast.GenDecl)
		if !ok {
			return
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, val := range vs.Values {
				sc.visit(val, s)
			}
			for i, name := range vs.Names {
				d := &varDecl{name: name.Name, end: vs.End()}
				if vs.Type != nil {
					d.typ = typeNameOf(vs.Type)
				}
				if i < len(vs.Values) {
					d.values = []binding{{expr: vs.Values[i], idx: -1}}
					if d.typ == "" {
						d.typ = literalTypeName(vs.Values[i])
					}
				}
				s.declare(d)
			}
		}
	case *ast.AssignStmt:
		for _, rhs := range v.Rhs {
			sc.visit(rhs, s)
		}
		for i, lhs := range v.Lhs {
			id, ok := lhs.(*ast.Ident)
			if !ok {
				sc.visit(lhs, s)
				continue
			}
			if id.Name == "_" {
				continue
			}
			var b binding
			switch {
			case len(v.Lhs) == len(v.Rhs):
				b = binding{expr: v.Rhs[i], idx: -1}
			case len(v.Rhs) == 1:
				b = binding{expr: v.Rhs[0], idx: i} // x, err := f(): one result position each
			default:
				b = binding{idx: -1}
			}
			if v.Tok == token.DEFINE {
				// Redeclaration in the same scope re-uses the variable.
				if d := sameScopeDecl(s, id.Name); d != nil {
					d.values = append(d.values, b)
					continue
				}
				d := &varDecl{name: id.Name, end: v.End(), values: []binding{b}}
				if b.expr != nil && b.idx < 0 {
					d.typ = literalTypeName(b.expr)
				}
				s.declare(d)
				continue
			}
			if d := sc.lookup(id.Name, id.Pos()); d != nil {
				d.values = append(d.values, b)
			}
		}
	case *ast.FuncLit:
		c := s.child(v)
		visible := v.Type.End()
		if v.Type.Params != nil {
			for _, f := range v.Type.Params.List {
				for _, name := range f.Names {
					c.declare(&varDecl{name: name.Name, end: visible, typ: typeNameOf(f.Type), data: true})
				}
			}
		}
		if v.Type.Results != nil {
			for _, f := range v.Type.Results.List {
				for _, name := range f.Names {
					c.declare(&varDecl{name: name.Name, end: visible, result: true})
				}
			}
		}
		if v.Body != nil {
			for _, stmt := range v.Body.List {
				sc.visit(stmt, c)
			}
		}
	default:
		// Any other node: descend one level so nested closures and
		// statements inside expressions are still recorded.
		ast.Inspect(n, func(child ast.Node) bool {
			if child == n {
				return true
			}
			if child != nil {
				sc.visit(child, s)
			}
			return false
		})
	}
}

func sameScopeDecl(s *scopeNode, name string) *varDecl {
	ds := s.decls[name]
	if len(ds) == 0 {
		return nil
	}
	return ds[len(ds)-1]
}
