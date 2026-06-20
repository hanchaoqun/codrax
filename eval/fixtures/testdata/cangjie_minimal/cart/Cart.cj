// Cart domain types — exercises a public struct, a public class, and an
// `extend` block. Package is demo.cart (distinct from the file path).

package demo.cart

public struct Item {
    let label: String

    public init(label: String) {
        this.label = label
    }
}

public class Cart {
    var items: Array<Item>

    public init() {
        this.items = Array<Item>()
    }

    public func add(label: String): Unit {
        // append elided for the fixture
    }

    public func size(): Int64 {
        return items.size
    }
}

extend Cart {
    public func isEmpty(): Bool {
        return this.size() == 0
    }
}
