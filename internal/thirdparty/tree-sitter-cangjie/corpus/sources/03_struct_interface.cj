// Source: cangjie-lang.cn docs / struct + interface + value semantics
// Surface: struct + interface + implementing trait-like interface

package demo.shapes

public interface Drawable {
    func draw(): Unit
}

public struct Point {
    let x: Float64
    let y: Float64

    init(x: Float64, y: Float64) {
        this.x = x
        this.y = y
    }
}

public struct Circle <: Drawable {
    let center: Point
    let radius: Float64

    init(center: Point, radius: Float64) {
        this.center = center
        this.radius = radius
    }

    public func draw(): Unit {
        println("Circle at (${center.x}, ${center.y}) r=${radius}")
    }
}
