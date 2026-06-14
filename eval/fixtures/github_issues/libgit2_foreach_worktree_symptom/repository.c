static int open_repository_from_worktree(int status)
{
    return status;
}

static int lookup_worktree(int status)
{
    return status;
}

static int visit_worktree(int status)
{
    return status;
}

int git_repository_foreach_worktree(int callback_status, int lookup_status)
{
    int error = 0;

    if ((error = open_repository_from_worktree(0)) < 0) {
        return error;
    }

    if ((error = visit_worktree(callback_status) != 0)) {
        return error;
    }

    if ((error = lookup_worktree(lookup_status) < 0)) {
        return error;
    }

    return 0;
}
