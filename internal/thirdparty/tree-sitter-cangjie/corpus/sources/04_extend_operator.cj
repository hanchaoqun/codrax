// Source: cangjie-lang.cn docs / extend keyword + operator overload
// Surface: extend String + operator func + slicing

package demo.stringext

extend String {
    operator func >>(n: Int64): String {
        if (n <= 0) {
            return this.clone()
        }
        let size = this.size
        let offset = size - n % size
        this[offset..size] + this[0..offset]
    }
}

main(): Int64 {
    let rotated = "abcdef" >> 2
    println(rotated)
    return 0
}
