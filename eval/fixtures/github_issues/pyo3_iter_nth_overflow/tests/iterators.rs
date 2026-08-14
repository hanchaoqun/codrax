use crate::types::list::PyListIterator;
use crate::types::tuple::PyTupleIterator;

#[test]
fn list_nth_past_end_exhausts_iterator() {
    let mut iter = PyListIterator::new(vec![1, 2, 3]);
    assert_eq!(iter.nth(10), None);
    assert_eq!(iter.next(), None);
}

#[test]
fn tuple_nth_back_past_end_exhausts_iterator() {
    let mut iter = PyTupleIterator::new(vec![1, 2, 3]);
    assert_eq!(iter.nth_back(10), None);
    assert_eq!(iter.next_back(), None);
}

#[test]
fn list_nth_back_empty_stays_empty() {
    let mut iter = PyListIterator::<i32>::new(vec![]);
    assert_eq!(iter.nth_back(0), None);
    assert_eq!(iter.next(), None);
}

#[test]
fn tuple_nth_back_empty_stays_empty() {
    let mut iter = PyTupleIterator::<i32>::new(vec![]);
    assert_eq!(iter.nth_back(0), None);
    assert_eq!(iter.next(), None);
}

#[test]
fn list_max_skip_exhausts_both_directions() {
    let mut forward = PyListIterator::new(vec![1, 2, 3]);
    assert_eq!(forward.nth(usize::MAX), None);
    assert_eq!(forward.next(), None);

    let mut reverse = PyListIterator::new(vec![1, 2, 3]);
    assert_eq!(reverse.nth_back(usize::MAX), None);
    assert_eq!(reverse.next_back(), None);
}

#[test]
fn tuple_max_skip_exhausts_both_directions() {
    let mut forward = PyTupleIterator::new(vec![1, 2, 3]);
    assert_eq!(forward.nth(usize::MAX), None);
    assert_eq!(forward.next(), None);

    let mut reverse = PyTupleIterator::new(vec![1, 2, 3]);
    assert_eq!(reverse.nth_back(usize::MAX), None);
    assert_eq!(reverse.next_back(), None);
}

#[test]
fn list_exact_remaining_reverse_skip_exhausts_shared_range() {
    let mut iter = PyListIterator::new(vec![1, 2, 3]);
    assert_eq!(iter.nth_back(3), None);
    assert_eq!(iter.next(), None);
    assert_eq!(iter.next_back(), None);
}

#[test]
fn tuple_exact_remaining_reverse_skip_exhausts_shared_range() {
    let mut iter = PyTupleIterator::new(vec![1, 2, 3]);
    assert_eq!(iter.nth_back(3), None);
    assert_eq!(iter.next(), None);
    assert_eq!(iter.next_back(), None);
}
