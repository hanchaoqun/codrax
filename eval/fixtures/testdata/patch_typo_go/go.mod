// Self-contained module so go build ./... from the codrax repo root
// does NOT descend here (separate modules aren't recursed into). The
// build-tag marker on main.go is a belt — this go.mod is the braces.
module codrax.example/patch_typo_fixture

go 1.22
