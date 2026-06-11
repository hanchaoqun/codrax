//! fastlex core: byte-pair tokenization primitives.
//! Python bindings export these via pyo3 under the module name `_fastlex`.

mod merge;

pub use merge::MergeTable;

/// Tokenize raw bytes into vocabulary ids. Exposed to Python as
/// `_fastlex.tokenize_bytes` (see #[pyfunction] below).
pub fn tokenize_bytes(input: &[u8], table: &MergeTable) -> Vec<u32> {
    let mut ids: Vec<u32> = input.iter().map(|b| *b as u32).collect();
    loop {
        let Some((pos, rank)) = best_merge(&ids, table) else { break };
        ids[pos] = rank;
        ids.remove(pos + 1);
    }
    ids
}

fn best_merge(ids: &[u32], table: &MergeTable) -> Option<(usize, u32)> {
    let mut best: Option<(usize, u32)> = None;
    for i in 0..ids.len().saturating_sub(1) {
        if let Some(rank) = table.rank(ids[i], ids[i + 1]) {
            if best.map_or(true, |(_, r)| rank < r) {
                best = Some((i, rank));
            }
        }
    }
    best
}

// ---- pyo3 surface (module `_fastlex`) ----

#[cfg(feature = "python")]
mod py {
    use pyo3::prelude::*;

    /// Python-facing wrapper: `_fastlex.tokenize_bytes(data, merges)`.
    #[pyfunction]
    fn tokenize_bytes(data: Vec<u8>, merges: Vec<(u32, u32, u32)>) -> Vec<u32> {
        let table = super::MergeTable::from_triples(&merges);
        super::tokenize_bytes(&data, &table)
    }

    #[pymodule]
    fn _fastlex(m: &Bound<'_, PyModule>) -> PyResult<()> {
        m.add_function(wrap_pyfunction!(tokenize_bytes, m)?)?;
        Ok(())
    }
}
