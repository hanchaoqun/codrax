#include <stdio.h>

int git_repository_foreach_worktree(int cb_result, int lookup_result);

static int expect_eq(const char* name, int got, int want)
{
    if (got != want) {
        fprintf(stderr, "%s: got %d, want %d\n", name, got, want);
        return 1;
    }
    return 0;
}

int main(void)
{
    int failed = 0;

    failed |= expect_eq("callback negative status", git_repository_foreach_worktree(-42, 0), -42);
    failed |= expect_eq("lookup negative status", git_repository_foreach_worktree(0, -7), -7);
    failed |= expect_eq("success path", git_repository_foreach_worktree(0, 0), 0);

    return failed;
}
