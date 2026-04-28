package trusterr

import (
	"fmt"
	"testing"
)

func TestCodeOfWrappedError(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("outer: %w", Wrap(CodeInvalidArgument, "bad input", fmt.Errorf("missing field")))
	if got := CodeOf(err); got != CodeInvalidArgument {
		t.Fatalf("CodeOf() = %s, want %s", got, CodeInvalidArgument)
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode() = %d, want 2", got)
	}
}
