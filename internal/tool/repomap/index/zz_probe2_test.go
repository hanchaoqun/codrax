package index

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestZZProbe2(t *testing.T) {
	src := []byte(`package com.example

import org.springframework.web.bind.annotation.*

@RestController
@RequestMapping("/api/users")
class UserController {
    @GetMapping("/{id}")
    fun get(): String = "g"
}

data class UserDto(val id: Int)
`)
	root, ok := parseTreeSitterIfPossible(types.LangKotlin, src)
	if !ok {
		t.Fatal("no kotlin binding")
	}
	var sb strings.Builder
	dumpKt(root, src, 0, &sb)
	t.Log("\n" + sb.String())
}
