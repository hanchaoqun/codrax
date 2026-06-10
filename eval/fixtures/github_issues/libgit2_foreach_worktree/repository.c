/*
 * Reduced from libgit2/libgit2#7231.
 *
 * The bug is not in callback behavior; it is in assignment precedence.
 * A negative status from the callback or lookup path must be preserved.
 */

int git_repository_foreach_worktree(int cb_result, int lookup_result)
{
    int error = 0;

    if ((error = cb_result != 0)) {
        return error;
    }

    if ((error = lookup_result < 0)) {
        return error;
    }

    return 0;
}
