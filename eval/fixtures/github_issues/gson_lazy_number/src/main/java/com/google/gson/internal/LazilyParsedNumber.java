package com.google.gson.internal;

import java.math.BigDecimal;

/**
 * Reduced from google/gson. Holds the string form of a JSON number and
 * parses lazily. Historical gap: the class implemented Number but not
 * equals/hashCode, so two instances wrapping the same literal compared
 * unequal and broke map/set usage and test assertions.
 */
public final class LazilyParsedNumber extends Number {
    private final String value;

    public LazilyParsedNumber(String value) {
        this.value = value;
    }

    @Override
    public int intValue() {
        try {
            return Integer.parseInt(value);
        } catch (NumberFormatException e) {
            try {
                return (int) Long.parseLong(value);
            } catch (NumberFormatException nfe) {
                return new BigDecimal(value).intValue();
            }
        }
    }

    @Override
    public long longValue() {
        try {
            return Long.parseLong(value);
        } catch (NumberFormatException e) {
            return new BigDecimal(value).longValue();
        }
    }

    @Override
    public float floatValue() {
        return Float.parseFloat(value);
    }

    @Override
    public double doubleValue() {
        return Double.parseDouble(value);
    }

    @Override
    public String toString() {
        return value;
    }
}
