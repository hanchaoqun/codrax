package normalizer

// enStopwords holds the closed class of English words the normalizer
// refuses to promote to concept terms. Terms here are filtered *after*
// structural patterns (CamelCase, snake_case, paths, flags) have
// claimed their matches — so "Explorer" still survives because the
// CamelCase pattern catches it first. The set is intentionally small;
// broader stopword lists over-filter domain words like "register",
// "config", "verify".
var enStopwords = map[string]bool{
	// articles, prepositions, conjunctions
	"the": true, "a": true, "an": true, "and": true, "or": true,
	"but": true, "of": true, "in": true, "on": true, "at": true,
	"to": true, "for": true, "with": true, "from": true, "by": true,
	"as": true, "into": true, "onto": true, "about": true,
	"over": true, "under": true, "between": true, "through": true,

	// question words
	"how": true, "what": true, "why": true, "when": true, "where": true,
	"which": true, "who": true, "whom": true, "whose": true,

	// auxiliaries / common verbs
	"is": true, "are": true, "was": true, "were": true, "be": true,
	"been": true, "being": true, "do": true, "does": true, "did": true,
	"have": true, "has": true, "had": true, "can": true, "could": true,
	"should": true, "would": true, "will": true, "may": true, "might": true,
	"must": true, "shall": true,

	// pronouns
	"it": true, "its": true, "this": true, "that": true, "these": true,
	"those": true, "they": true, "them": true, "their": true,
	"you": true, "your": true, "yours": true, "we": true, "our": true,
	"ours": true, "us": true, "he": true, "him": true, "his": true,
	"she": true, "her": true, "hers": true,

	// meta-instructions that are not useful as search terms
	"please": true, "tell": true, "show": true, "explain": true,
	"describe": true, "help": true, "want": true, "need": true,
	"find": true, "there": true, "here": true, "also": true, "just": true,
	"some": true, "any": true, "each": true, "every": true, "all": true,
	"then": true, "than": true, "such": true, "very": true, "much": true,
	"more": true, "most": true, "less": true, "least": true,
	"same": true, "other": true, "another": true,
}

func isEnglishStopword(lower string) bool { return enStopwords[lower] }
