# Source Operation Site Handoff

## Problem

Customer log `../customlogs/cgroup_err.log` exposed a final-answer loss for a
current-source question asking where process/thread IDs are written into cgroup
files and what all write points are.

Exploration found the important sites:

- write helpers such as `SetPidToCgroup` and `SetPidAndFlagToCgroup`
- concrete `write(...)` calls
- dispatcher/caller sites such as `SetCgroup`, `SetTasksToCgroup`,
  `DoThreadControl`, and `LowBackgroundSuppress`
- constant path targets such as `/dev/cpuctl/cgroup.procs` and
  `/dev/cpuset/tasks`

Those facts reached the evidence/reference pool, but the final answer treated
the question mostly as a mechanism narrative. Some discovered write sites were
not rendered as principal rows, and one visible row borrowed a nearby constant
line as the member citation instead of citing the actual function/call site.

## Root Cause

Codrax already has an authoritative handoff lane for closed answer sets:
`emit_investigation_complete.aggregate_facts` with `kind="member_set"`.
That lane is required for typed enumeration/relation questions, and finalizer
contracts know how to preserve those members verbatim.

The cgroup request was classified as root-cause/mechanism, which is normally
correct because many mechanism questions need prose rather than a table. For
ordinary mechanism questions, member sets are deliberately demoted to supporting
coverage so helper names, branches, or inspected files do not become a fake
principal enumeration.

The missing shape is narrower: a current-source mechanism/root-cause question
whose analyzer IR explicitly carries both a complete-set boundary and a
source-operation-site surface. This is still a mechanism question, but its
visible answer has a principal site set.

## Red Lines

- Do not reroute ordinary mechanism, architecture, trace, or diagnostic
  questions into enumeration.
- Do not synthesize principal members from grep snippets, evidence prose, or
  analyzer entities. The investigating model must still author the
  `member_set`.
- Do not use noisy ranker scores or repo-map candidate counts as hard gates.
- Do not inspect `RawRequest` with localized keyword tables to infer this
  intent. Raw user text may only be used upstream for provenance validation of
  analyzer-emitted source quotes / mentioned entities, not as a downstream hard
  router.
- Do not couple to cgroup, Linux, C, or any specific repository.
- Do not let constants or adjacent target paths replace citations for the
  actual principal function/call/write site.

## Design

Add one precise request trait:

`RequiresSourceOperationSiteMemberSetHandoff(RequestModel)`.

The helper is intentionally conservative:

- It only fires when typed IR has a set boundary: enumerate intent,
  category/relational/per-member-table predicates, a declared enumeration
  boundary, a completeness obligation, or typed buckets.
- It also requires a typed operation-site surface: call/register/configure/
  condition predicate axis, call-chain/registration/config-mapping requirement
  kind, or a source-inventory profile whose principal roles are function,
  method, route, or file.
- Scalar/count/history-only requests stay out.
- Architecture narrative questions stay out unless the analyzer emits the
  typed operation-site shape above.

When active:

1. Explorer close readiness requires an accepted
   `aggregate_facts.member_set`.
2. Explorer prompts tell the model to put principal source operation sites in
   `members`, and to map each member to a function/call/file:line via
   `support_refs`.
3. Aggregate fact role projection keeps that member set as principal even
   though the broader question family is mechanism/root-cause.
4. Finalizer prompt tells the model to render the operation-site set as a
   visible list/table, use support refs for member citations, and keep target
   constants/paths as row details rather than borrowing them as the member
   citation.

This keeps the mechanism explanation and the operation-site table in the same
answer without changing normal source analysis or runtime observation flows.

## Tasks

- [x] Add the request trait and focused tests.
- [x] Remove downstream `RawRequest` keyword routing from the request trait.
- [x] Wire explorer structured member-set handoff to the trait.
- [x] Add source-operation-site aggregate handoff teaching to explorer prompt.
- [x] Keep source-operation-site `member_set` facts principal in aggregate
      projection.
- [x] Add finalizer rendering guidance for member citations versus target path
      details.
- [x] Add focused regression tests for the cgroup-shaped request and negative
      ordinary-mechanism cases.
- [ ] Run targeted Go tests and push the batch.
