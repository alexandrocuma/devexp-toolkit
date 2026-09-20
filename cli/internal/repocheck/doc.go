// Package repocheck holds no runtime code. It exists so that the repository's
// cross-asset coupling — the registry against the files it names, the hooks on
// disk against the registry, the catalogs and counts in prose against what is
// actually there — is checked by `go test ./...` instead of by a human reading
// a checklist.
//
// Every rule asserted here replaces a line of documentation that asked someone
// to remember. docs/guides/workflows.md names the step people miss: "the
// opencode and kimi mappings in step 3 are the ones people miss... Without the
// opencode mapping, opencode silently lacks the hook." That is the failure this
// package makes loud, and it is one shape of a recurring defect here: a
// coupling that nothing verified, found later by accident rather than by any
// check on the change that broke it.
//
// Failure messages are written to read like the checklist entry they replace:
// each one names the file to open and the change to make, because a developer
// who adds an agent and trips a count assertion needs a sentence, not a diff of
// two integers.
package repocheck
