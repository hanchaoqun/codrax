# PyO3 iterator nth overflow fixture

This fixture reconstructs the symptom behind PyO3 PR #6086 in a small
Rust/Python-binding-shaped source tree.

Observed behavior:

- calling `nth()` or `nth_back()` with a skip count larger than the remaining
  list/tuple items returns `None`;
- after that `None`, the iterator can still yield an item instead of being
  exhausted;
- extremely large skip counts can overflow index arithmetic.

The regression contract is encoded in `tests/check_iterators.py` so this
fixture can run without a full Rust toolchain.
