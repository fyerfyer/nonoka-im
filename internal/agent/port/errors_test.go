package port

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestClassifyExplicitAndUnknownErrors(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantClass ErrorClass
		retryable bool
	}{
		{name: "transient marker", err: &Error{Class: ClassTransientInfra, Err: errors.New("db down")}, wantClass: ClassTransientInfra, retryable: true},
		{name: "model", err: fmt.Errorf("%w: 529", ErrModel), wantClass: ClassModel, retryable: true},
		{name: "tool", err: fmt.Errorf("%w: timeout", ErrTool), wantClass: ClassTool, retryable: true},
		{name: "auth", err: fmt.Errorf("%w: denied", ErrAuthDenied), wantClass: ClassAuthDenied, retryable: false},
		{name: "invalid config", err: ErrInvalidConfig, wantClass: ClassInvalidConfig, retryable: false},
		{name: "timeout", err: context.DeadlineExceeded, wantClass: ClassTimeout, retryable: true},
		{name: "canceled", err: context.Canceled, wantClass: ClassCanceled, retryable: false},
		{name: "unknown defaults terminal", err: errors.New("boom"), wantClass: ClassInternal, retryable: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			class, retryable := Classify(tc.err)
			if class != tc.wantClass || retryable != tc.retryable {
				t.Fatalf("Classify(%v) = (%q, %v), want (%q, %v)", tc.err, class, retryable, tc.wantClass, tc.retryable)
			}
		})
	}
}
