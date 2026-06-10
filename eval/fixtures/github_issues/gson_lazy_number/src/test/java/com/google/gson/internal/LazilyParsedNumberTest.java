package com.google.gson.internal;

/** Mirrors the upstream regression: value semantics for equal literals. */
public final class LazilyParsedNumberTest {
    public static void main(String[] args) {
        LazilyParsedNumber a = new LazilyParsedNumber("1");
        LazilyParsedNumber b = new LazilyParsedNumber("1");
        LazilyParsedNumber c = new LazilyParsedNumber("2");
        if (!a.equals(b)) {
            throw new AssertionError("equal literals must be equal");
        }
        if (a.hashCode() != b.hashCode()) {
            throw new AssertionError("equal literals must share a hash code");
        }
        if (a.equals(c)) {
            throw new AssertionError("different literals must not be equal");
        }
        System.out.println("LazilyParsedNumber value semantics ok");
    }
}
