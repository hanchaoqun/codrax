# Reduced reproduction: google/gson LazilyParsedNumber value semantics

Upstream record: google/gson `gson/src/main/java/com/google/gson/internal/LazilyParsedNumber.java`
historically implemented `Number` without `equals`/`hashCode` (tracked as
google-gson issue 627 in the pre-GitHub tracker; the current upstream file
carries both methods comparing the wrapped string value). Two instances
wrapping the same literal compared unequal, breaking map keys and test
assertions.

This fixture reduces the class to its lazy-number core with the gap intact.
`make check` runs a source-level oracle (the eval host has no Java runtime).
