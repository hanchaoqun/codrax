use crate::Duration;

#[test]
fn checked_sub_zero_keeps_constructor_range_consistent() {
    let min = Duration::milliseconds(-i64::MAX);
    assert!(min.checked_sub(&Duration::milliseconds(0)).is_some());
}
