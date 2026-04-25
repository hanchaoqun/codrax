// Source: cangjie-lang.cn / class declaration with init + public methods
// Surface: package + public class + init constructor + let immutable field + public func

package demo.greeter

public class Greeter {
    let message: String

    init(msg: String) {
        this.message = msg
    }

    public func greet(): Unit {
        println(this.message)
    }
}

main(): Int64 {
    let g = Greeter("hello")
    g.greet()
    return 0
}
