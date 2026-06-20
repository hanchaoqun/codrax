// Minimal Cangjie app entry — exercises a public class + main entry.
// Part of the cangjie_minimal repomap fixture (extend / foreign func /
// public class enumeration). Package namespace is independent of the
// filesystem layout (Cangjie package_clause, red line L-Cangjie-2).

package demo.app

import demo.cart.*
import demo.bridge.*

public class App {
    let name: String

    public init(name: String) {
        this.name = name
    }

    public func run(): Int64 {
        let cart = Cart()
        cart.add("apple")
        return ohSum(1, 2)
    }
}

main(): Int64 {
    let app = App("demo")
    return app.run()
}
