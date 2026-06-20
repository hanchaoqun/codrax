// FFI bridge — exercises a `foreign func` declaration plus a public class
// and a public wrapper func. Package is demo.bridge.

package demo.bridge

foreign func native_add(a: Int64, b: Int64): Int64

@CJOH
public func ohSum(a: Int64, b: Int64): Int64 {
    unsafe {
        native_add(a, b)
    }
}

public class Bridge {
    public func sum(a: Int64, b: Int64): Int64 {
        return ohSum(a, b)
    }
}
