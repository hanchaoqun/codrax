// Source: cangjie-lang.cn docs / modifier combinations
// Surface: open / sealed / abstract / static / override + access modifiers

package demo.modifiers

public sealed class Animal {
    let name: String

    public init(name: String) {
        this.name = name
    }

    public open func sound(): String {
        return "generic sound"
    }

    public static func defaultName(): String {
        return "Unknown"
    }
}

public class Dog <: Animal {
    public init(name: String) {
        super(name)
    }

    public override func sound(): String {
        return "Woof"
    }
}

public abstract class Service {
    public abstract func start(): Unit
    public abstract func stop(): Unit
}

internal class InternalCache {
    private var entries: Int64 = 0
    protected func reset(): Unit {
        this.entries = 0
    }
}
