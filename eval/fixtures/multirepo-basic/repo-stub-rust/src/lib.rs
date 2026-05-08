//! Stub Rust crate — the third sub-repo in the multirepo-basic
//! fixture. Holds a single unrelated function so the focus case
//! (mr_focus_single) can verify the answer does NOT spuriously
//! mention this crate when the question is scoped to one of the
//! other sub-repos.

/// Returns the universal answer.
pub fn unrelated_constant() -> i32 {
    42
}
