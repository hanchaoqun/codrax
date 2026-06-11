/* C ABI for the fastlex native lexer (cJSON/libgit2-style flat C API).
 * Only the three fl_* functions are exported; the C++ Lexer class is an
 * internal implementation detail. */
#ifndef FASTLEX_H
#define FASTLEX_H

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct fl_lexer fl_lexer;

fl_lexer *fl_lexer_new(const char *vocab_path);
size_t fl_lexer_split(fl_lexer *lx, const char *text, int *out_ids, size_t cap);
void fl_lexer_free(fl_lexer *lx);

#ifdef __cplusplus
}
#endif

#endif /* FASTLEX_H */
