// Source: cangjie-lang.cn docs / enum + pattern match
// Surface: enum with constructor params + match expression + wildcard

package demo.shapes2

public enum Shape {
    | Circle(Float64)
    | Rectangle(Float64, Float64)
    | Triangle(Float64, Float64, Float64)
}

public func area(s: Shape): Float64 {
    match (s) {
        case Circle(r) => 3.14159 * r * r
        case Rectangle(w, h) => w * h
        case Triangle(a, b, c) =>
            let s2 = (a + b + c) / 2.0
            (s2 * (s2 - a) * (s2 - b) * (s2 - c)).sqrt()
        case _ => 0.0
    }
}
