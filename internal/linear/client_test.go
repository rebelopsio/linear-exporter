package linear

import (
	"net/http"
	"testing"
)

func TestUpdateRateLimits(t *testing.T) {
	c := &Client{}
	headers := http.Header{}
	headers.Set("X-RateLimit-Remaining", "10")
	headers.Set("X-RateLimit-Reset", "1700000000")

	c.updateRateLimits(headers)

	if got := c.RateLimitRemaining(); got != 10 {
		t.Fatalf("expected remaining 10, got %d", got)
	}
	if got := c.rateLimitReset.Unix(); got != 1700000000 {
		t.Fatalf("expected reset unix 1700000000, got %d", got)
	}
}
