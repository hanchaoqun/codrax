# Reduced reproduction: chronotope/chrono#1385 Duration minimum bound

Upstream record: https://github.com/chronotope/chrono/pull/1385

The upstream bug was a boundary mismatch in `Duration`: milliseconds could
construct `i64::MIN`, but arithmetic used `MIN` with the intentional lower
bound of `-i64::MAX` milliseconds. The final upstream direction kept
`-i64::MAX` as the lower bound, introduced a fallible
`Duration::try_milliseconds(milliseconds: i64) -> Option<Duration>`
constructor, and made `Duration::milliseconds()` reuse it and panic on
out-of-range input.

The fix should constrain constructors to the existing `MIN`/`MAX` range. Do
not redefine `MIN` or `MAX` through `Duration::milliseconds()`: that creates a
recursive constructor/bounds dependency instead of preserving the duration
constants as direct boundary facts.

This fixture keeps only the relevant Rust-shaped source and a Python oracle so
the eval does not require a local Rust toolchain.
