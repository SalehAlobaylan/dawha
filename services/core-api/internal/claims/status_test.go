package claims

import "testing"

func TestPlatformFindingCannotBecomeDocumentedWithoutReview(t *testing.T) {
	if CanTransition(PlatformGenerated, Documented, false) {
		t.Fatal("expected unreviewed platform output not to become documented")
	}
	if !CanTransition(PlatformGenerated, Documented, true) {
		t.Fatal("expected explicitly reviewed platform output to be publishable")
	}
}

func TestRejectedClaimCanOnlyReturnToUnknownWithoutReview(t *testing.T) {
	if CanTransition(Rejected, Documented, false) {
		t.Fatal("expected rejected claim not to be silently restored")
	}
	if !CanTransition(Rejected, Unknown, false) {
		t.Fatal("expected rejected claim to be revisitable as unknown")
	}
}
