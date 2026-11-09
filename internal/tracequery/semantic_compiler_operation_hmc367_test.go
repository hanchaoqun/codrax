package tracequery

import "testing"

func TestHMC367CompilerMarkerRequiresOperationNotDomain(t *testing.T) {
	for _, c := range []struct{ name, want string }{
		{"bool art::jit::JitMemoryRegion::Initialize(size_t, size_t, bool, bool, std::string *)", ""},
		{"void vm::jit::JitCompiler::LoadCacheEntry(int)", ""},
		{"H:RuntimeCompiler::Initialize(ArkMethod*)", ""},
		{"H:Ark runtime compiler cache lookup", ""},
		{"JIT MemoryRegion Setup", ""},
		{"JIT compiling void pkg.Entry.run() (kind=Baseline)", "jit_compile"},
		{"H:JITCompileTask", "jit_compile"},
		{"void vm::jit::JitCompiler::CompileMethod(int)", "jit_compile"},
		{"H:Ark runtime compile", "runtime_compile"},
		{"H:ark::RuntimeCompiler::CompileMethod(int)", "runtime_compile"},
		{"VerifyClass com.example.Foo", "class_verification"},
		{"H:Texture upload(15283) 512x194", "texture_upload"},
	} {
		if got := traceSpanSemanticClass(c.name); got != c.want {
			t.Errorf("native marker %q classified %q; want %q", c.name, got, c.want)
		}
	}
}
