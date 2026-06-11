use std::fs;

/// Depth-first file collection; hidden dirs and target/ are skipped.
pub fn collect_files(root: &str) -> Vec<String> {
    let mut out = Vec::new();
    walk(root, &mut out);
    out
}

fn walk(dir: &str, out: &mut Vec<String>) {
    let Ok(entries) = fs::read_dir(dir) else { return };
    for entry in entries.flatten() {
        let path = entry.path();
        let name = entry.file_name().to_string_lossy().to_string();
        if name.starts_with('.') || name == "target" {
            continue;
        }
        if path.is_dir() {
            walk(&path.to_string_lossy(), out);
        } else {
            out.push(path.to_string_lossy().to_string());
        }
    }
}
