/// Line-matching strategy. Exactly two implementations exist:
/// LiteralMatcher (--fixed) and RegexLikeMatcher (default).
pub trait Matcher {
    fn is_match(&self, line: &str) -> bool;
}

pub struct LiteralMatcher {
    needle: String,
}

impl LiteralMatcher {
    pub fn new(needle: &str) -> Self {
        LiteralMatcher { needle: needle.to_string() }
    }
}

impl Matcher for LiteralMatcher {
    fn is_match(&self, line: &str) -> bool {
        line.contains(&self.needle)
    }
}

pub struct RegexLikeMatcher {
    tokens: Vec<String>,
}

impl RegexLikeMatcher {
    pub fn new(pattern: &str) -> Self {
        RegexLikeMatcher { tokens: pattern.split(".*").map(str::to_string).collect() }
    }
}

impl Matcher for RegexLikeMatcher {
    fn is_match(&self, line: &str) -> bool {
        let mut rest = line;
        for t in &self.tokens {
            match rest.find(t.as_str()) {
                Some(i) => rest = &rest[i + t.len()..],
                None => return false,
            }
        }
        true
    }
}
