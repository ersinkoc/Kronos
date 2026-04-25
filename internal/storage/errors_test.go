package storage

import "testing"

func TestInvalidKeyError(t *testing.T) {
	t.Parallel()

	err := InvalidKeyError{Key: "../bad"}
	if got := err.Error(); got != `invalid object key "../bad"` {
		t.Fatalf("Error() = %q", got)
	}
}
