// Source: cangjie-lang.cn / FFI foreign func + @CJOH ArkTS interop
// Surface: foreign func + decorator + unsafe block

package demo.ffi

foreign func native_add(a: Int64, b: Int64): Int64

@CJOH
public func ohSum(a: Int64, b: Int64): Int64 {
    unsafe {
        native_add(a, b)
    }
}

@CallingThread["main"]
public func runOnMainThread(callback: () -> Unit): Unit {
    callback()
}
