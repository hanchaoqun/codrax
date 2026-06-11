use std::collections::HashMap;

/// MergeTable maps adjacent id pairs to their merged rank id.
pub struct MergeTable {
    ranks: HashMap<(u32, u32), u32>,
}

impl MergeTable {
    pub fn from_triples(triples: &[(u32, u32, u32)]) -> Self {
        let mut ranks = HashMap::with_capacity(triples.len());
        for (a, b, rank) in triples {
            ranks.insert((*a, *b), *rank);
        }
        MergeTable { ranks }
    }

    pub fn rank(&self, a: u32, b: u32) -> Option<u32> {
        self.ranks.get(&(a, b)).copied()
    }
}
