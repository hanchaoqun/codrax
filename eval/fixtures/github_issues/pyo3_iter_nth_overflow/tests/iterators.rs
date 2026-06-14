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
