package limiter

import (
	"testing"
)

func TestIsOverLimit(t *testing.T) {
	testShallOverLimit := []struct {
		Limit           uint64
		syncedCount     uint64
		currentCount    uint64
		lastWindowCount uint64
		weight          float64
	}{
		{5, 3, 3, 0, 0.0},
		{5, 2, 2, 5, 0.5},
		{20, 10, 3, 10, 0.8},
	}

	for _, limitTestSet := range testShallOverLimit {
		rl := &RateLimiter{Limit: limitTestSet.Limit, syncedCount: limitTestSet.syncedCount, currentCount: limitTestSet.currentCount, lastWindowCount: limitTestSet.lastWindowCount}
		weight := limitTestSet.weight
		assumeTrue := rl.IsOverLimit(weight)
		if assumeTrue != true {
			t.Errorf("limit rate is %d, syncedCount account is %d, currentCount is %d, last window count is %d with a weight of %f shall over the limit", rl.Limit, rl.syncedCount, rl.currentCount, rl.lastWindowCount, weight)
		}
	}

	testShallNotOverLimit := []struct {
		Limit           uint64
		syncedCount     uint64
		currentCount    uint64
		lastWindowCount uint64
		weight          float64
	}{
		{8, 3, 3, 0, 0.0},
		{7, 2, 2, 5, 0.5},
		{22, 10, 3, 10, 0.8},
	}

	for _, limitTestSet := range testShallNotOverLimit {
		rl := &RateLimiter{Limit: limitTestSet.Limit, syncedCount: limitTestSet.syncedCount, currentCount: limitTestSet.currentCount, lastWindowCount: limitTestSet.lastWindowCount}
		weight := limitTestSet.weight
		assumeTrue := rl.IsOverLimit(weight)
		if assumeTrue == true {
			t.Errorf("limit rate is %d, syncedCount account is %d, currentCount is %d, last window count is %d with a weight of %f shall not over the limit", rl.Limit, rl.syncedCount, rl.currentCount, rl.lastWindowCount, weight)
		}
	}
}
