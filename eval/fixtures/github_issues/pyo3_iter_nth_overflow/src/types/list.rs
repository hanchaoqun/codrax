#[derive(Clone, Debug)]
pub struct PyListIterator<T> {
    items: Vec<T>,
    index: usize,
    length: usize,
}

impl<T: Clone> PyListIterator<T> {
    pub fn new(items: Vec<T>) -> Self {
        let length = items.len();
        Self {
            items,
            index: 0,
            length,
        }
    }
}

impl<T: Clone> Iterator for PyListIterator<T> {
    type Item = T;

    fn next(&mut self) -> Option<Self::Item> {
        self.nth(0)
    }

    fn nth(&mut self, n: usize) -> Option<Self::Item> {
        let current_length = self.length.min(self.items.len());
        let target_index = self.index + n;
        if target_index < current_length {
            self.index = target_index + 1;
            Some(self.items[target_index].clone())
        } else {
            None
        }
    }
}

impl<T: Clone> DoubleEndedIterator for PyListIterator<T> {
    fn next_back(&mut self) -> Option<Self::Item> {
        self.nth_back(0)
    }

    fn nth_back(&mut self, n: usize) -> Option<Self::Item> {
        let current_length = self.length.min(self.items.len());
        if self.index + n < current_length {
            let target_index = current_length - n - 1;
            self.length = target_index;
            Some(self.items[target_index].clone())
        } else {
            None
        }
    }
}
