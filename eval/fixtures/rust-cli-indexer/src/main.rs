mod matcher;
mod walker;

use matcher::{LiteralMatcher, Matcher, RegexLikeMatcher};

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let pattern = args.get(1).cloned().unwrap_or_default();
    let fixed = args.iter().any(|a| a == "--fixed");
    std::process::exit(run(&pattern, fixed));
}

/// Builds the matcher per flags, walks the tree, indexes every file.
fn run(pattern: &str, fixed: bool) -> i32 {
    let m: Box<dyn Matcher> = if fixed {
        Box::new(LiteralMatcher::new(pattern))
    } else {
        Box::new(RegexLikeMatcher::new(pattern))
    };
    let files = walker::collect_files(".");
    let mut hits = 0usize;
    for f in &files {
        hits += index_file(f, m.as_ref());
    }
    if hits > 0 { 0 } else { 1 }
}

fn index_file(path: &str, m: &dyn Matcher) -> usize {
    let Ok(content) = std::fs::read_to_string(path) else { return 0 };
    content.lines().filter(|l| m.is_match(l)).count()
}
