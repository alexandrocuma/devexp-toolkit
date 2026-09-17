package manifest

import "testing"

// TEMPORARY, for PR #166 (#155) only — deleted in the next commit.
// Makes the `test` job inside ci.yml fail on purpose, so the pull request shows
// what a tag would do: the `ci` call job fails, and anything with `needs: ci`
// (on a tag, `goreleaser`) is skipped and publishes nothing.
func TestZZGateProbeDeliberateFailure(t *testing.T) {
	t.Fatalf("deliberate failure: proving the release gate on PR #166")
}
