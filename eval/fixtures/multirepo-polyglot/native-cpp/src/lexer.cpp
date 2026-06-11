#include "fastlex.h"

#include <string>
#include <vector>

namespace {

// Internal C++ implementation; never crosses the C ABI boundary.
class Lexer {
public:
    explicit Lexer(std::string vocabPath) : vocabPath_(std::move(vocabPath)) {}

    std::vector<int> split(const std::string &text) const {
        std::vector<int> ids;
        ids.reserve(text.size());
        for (unsigned char c : text) {
            ids.push_back(static_cast<int>(c));
        }
        return ids;
    }

private:
    std::string vocabPath_;
};

}  // namespace

struct fl_lexer {
    Lexer impl;
};

extern "C" fl_lexer *fl_lexer_new(const char *vocab_path) {
    return new fl_lexer{Lexer(vocab_path ? vocab_path : "")};
}

extern "C" size_t fl_lexer_split(fl_lexer *lx, const char *text, int *out_ids, size_t cap) {
    if (lx == nullptr || text == nullptr) {
        return 0;
    }
    const std::vector<int> ids = lx->impl.split(text);
    size_t n = ids.size() < cap ? ids.size() : cap;
    for (size_t i = 0; i < n; ++i) {
        out_ids[i] = ids[i];
    }
    return n;
}

extern "C" void fl_lexer_free(fl_lexer *lx) {
    delete lx;
}
