package transport

import (
	"net/http"
	"testing"
	"time"
)

func TestRetryPolicyDelayNeverExceedsMaxDelayAfterJitter(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    250 * time.Millisecond,
		Jitter: func(delay time.Duration) time.Duration {
			return delay
		},
	}
	if got := policy.Delay(2); got != policy.MaxDelay {
		t.Fatalf("Delay(2) = %s, want %s", got, policy.MaxDelay)
	}
}

func TestRetryPolicyRetryAfterSupportsSecondsAndHTTPDate(t *testing.T) {
	now := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	policy := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Second}

	if got, ok := policy.RetryAfterDelay(http.Header{"Retry-After": []string{"1"}}, now); !ok || got != time.Second {
		t.Fatalf("seconds Retry-After = (%s, %t), want (%s, true)", got, ok, time.Second)
	}
	date := now.Add(time.Second).Format(http.TimeFormat)
	if got, ok := policy.RetryAfterDelay(http.Header{"Retry-After": []string{date}}, now); !ok || got != time.Second {
		t.Fatalf("date Retry-After = (%s, %t), want (%s, true)", got, ok, time.Second)
	}
	if got, ok := policy.RetryAfterDelay(http.Header{"Retry-After": []string{"60"}}, now); !ok || got != policy.MaxDelay {
		t.Fatalf("clamped Retry-After = (%s, %t), want (%s, true)", got, ok, policy.MaxDelay)
	}
}

func TestRetryPolicyRetryAfterRejectsInvalidOrPastValue(t *testing.T) {
	policy := DefaultRetryPolicy()
	now := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)

	for _, value := range []string{"", "nope", now.Add(-time.Second).Format(http.TimeFormat), "-1"} {
		t.Run(value, func(t *testing.T) {
			if got, ok := policy.RetryAfterDelay(http.Header{"Retry-After": []string{value}}, now); ok || got != 0 {
				t.Fatalf("RetryAfterDelay(%q) = (%s, %t), want (0, false)", value, got, ok)
			}
		})
	}
}
