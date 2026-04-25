// Source: cangjie-lang.cn docs / generic function with where clause
// Surface: generic_params + where_clause + interface bound + method call

package demo.generics

public interface Comparable<T> {
    func compareTo(other: T): Int64
}

public func max<T>(a: T, b: T): T where T <: Comparable<T> {
    if (a.compareTo(b) >= 0) {
        a
    } else {
        b
    }
}

public class Version <: Comparable<Version> {
    let major: Int64
    let minor: Int64

    init(major: Int64, minor: Int64) {
        this.major = major
        this.minor = minor
    }

    public func compareTo(other: Version): Int64 {
        if (this.major != other.major) {
            return this.major - other.major
        }
        return this.minor - other.minor
    }
}
