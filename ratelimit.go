package limiter

// sample rate limit implementation inspired by https://github.com/manavo/go-rate-limiter
// algorythm inspired by https://konghq.com/blog/how-to-design-a-scalable-rate-limiting-algorithm

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gomodule/redigo/redis"
)

type RateLimiter struct {
	RedisPool *redis.Pool

	Limit         uint64
	BaseKey       string
	Interval      time.Duration
	FlushInterval time.Duration

	syncedCount     uint64
	currentCount    uint64
	currentKey      string
	lastWindowCount uint64
	lastKey         string
	retryTime       uint64

	ticker     *time.Ticker
	stopTicker chan bool
}

func New(redisPool *redis.Pool, baseKey string, limit uint64, interval time.Duration, flushInterval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		RedisPool: redisPool,

		Limit:         limit,
		BaseKey:       baseKey,
		Interval:      interval,
		FlushInterval: flushInterval,
	}

	return rl
}

// Updates the current key, based on the base key
func (rl *RateLimiter) updateCurrentKey() {
	now := float64(time.Now().Unix())
	seconds := rl.Interval.Seconds()
	currentTimeIntervalString := fmt.Sprintf("%d", int64(math.Floor(now/seconds)))
	rl.currentKey = fmt.Sprintf("%s:%s", rl.BaseKey, currentTimeIntervalString)
}

// update last window key
func (rl *RateLimiter) updateLastWindowKey() {
	now := float64(time.Now().Unix())
	seconds := rl.Interval.Seconds()
	lastTimeIntervalString := fmt.Sprintf("%d", int64(math.Floor(now/seconds)-1))
	lastKey := fmt.Sprintf("%s:%s", rl.BaseKey, lastTimeIntervalString)
	if rl.lastKey != lastKey {
		rl.lastKey = lastKey
	}
}

// Stop terminates the ticker, and flushed the final count we had
func (rl *RateLimiter) Stop() {
	close(rl.stopTicker)
	rl.Flush()
}

// Increment adds 1 to the local counter (doesn't get synced until Flush gets called)
func (rl *RateLimiter) Increment() {
	atomic.AddUint64(&rl.currentCount, 1)
}

func (rl *RateLimiter) Flush() {
	flushCount := atomic.SwapUint64(&rl.currentCount, 0)

	// send to redis, and get the updated value
	redisConn := rl.RedisPool.Get()

	// We have to close the connection ourselves when we're done
	defer redisConn.Close()

	var newSyncedCount uint64

	redisConn.Send("MULTI")
	redisConn.Send("INCRBY", rl.currentKey, flushCount)
	redisConn.Send("EXPIRE", rl.currentKey, rl.Interval.Seconds()*2)

	reply, redisErr := redis.Values(redisConn.Do("EXEC"))

	if redisErr != nil {
		// Could not increment, so restore the current count
		atomic.AddUint64(&rl.currentCount, flushCount)

		log.Printf("Error executing Redis commands: %v", redisErr)
		return
	}

	if _, scanErr := redis.Scan(reply, &newSyncedCount); scanErr != nil {
		log.Printf("Error reading new synced count: %v", scanErr)
		return
	}

	rl.syncedCount = newSyncedCount
}

// Update last window count
func (rl *RateLimiter) UpdateLastWindowCount() {
	var lastWindowCount uint64
	// send to redis, and get the updated value
	redisConn := rl.RedisPool.Get()
	reply, redisErr := redis.Values(redisConn.Do("GET", rl.lastKey))
	// Only update last window count when last key exists and retrieve succeed
	if redisErr == nil {
		if _, scanErr := redis.Scan(reply, &lastWindowCount); scanErr != nil {
			log.Printf("Error reading new synced count: %v", scanErr)
			return
		}
	}
	rl.lastWindowCount = lastWindowCount
}

// IsOverLimit checks if we are over the limit we have set
func (rl *RateLimiter) IsOverLimit() bool {
	if rl.lastWindowCount != 0 {
		lastWindowWeight := rl.GetLastWindowWeight()
		if float64(rl.lastWindowCount)*lastWindowWeight+float64(rl.syncedCount+rl.currentCount) > float64(rl.Limit) {
			rl.UpdateRetryTime(rl.syncedCount+rl.currentCount, rl.lastWindowCount, lastWindowWeight)
			return true
		}
	} else {
		if rl.syncedCount+rl.currentCount > rl.Limit {
			rl.UpdateRetryTime(rl.syncedCount+rl.currentCount, 0, 0.0)
			return true
		}
	}
	return false
}

// calculate the cooling off period.
func (rl *RateLimiter) UpdateRetryTime(currentCount uint64, lastWindowCount uint64, lastWindowWeight float64) {
	interval := rl.Interval.Seconds()
	var coolingOff uint64
	if currentCount > rl.Limit {
		coolingOff = uint64(interval)
	} else if float64(currentCount)+float64(lastWindowCount)*lastWindowWeight > float64(rl.Limit) {
		// use the percentage of required count from last window to calculate required time from last window.
		coolingOffRatio := float64(rl.Limit) - float64(currentCount)/float64(lastWindowCount)
		coolingOff = uint64(coolingOffRatio * interval)
	}
	atomic.SwapUint64(&rl.retryTime, coolingOff)
}

// now/seconds - math.Floor(now/seconds) will get the percentage of current window.
// e.g. if interval is 60 secs, current time is 01:00:06, then current percentage shall be 10%
// last window weight shall be 90%
func (rl *RateLimiter) GetLastWindowWeight() float64 {
	now := float64(time.Now().Unix())
	seconds := rl.Interval.Seconds()
	LastWindowWeight := 1.0 - (now/seconds - math.Floor(now/seconds))
	return LastWindowWeight
}

// Init starts the ticker, which takes care of periodically flushing/syncing the counter
func (rl *RateLimiter) Init() error {
	if rl.Interval < time.Minute {
		return fmt.Errorf("Minimum interval is 1 minute")
	}
	if rl.Interval.Seconds() < rl.FlushInterval.Seconds() || int64(rl.Interval.Seconds())%int64(rl.FlushInterval.Seconds()) != 0 {
		return fmt.Errorf("Flush interval must be x times of Interval")
	}

	rl.updateCurrentKey()

	rl.ticker = time.NewTicker(rl.FlushInterval)

	go func(rl *RateLimiter) {
		for {
			select {
			case <-rl.ticker.C:
				rl.updateCurrentKey()
				rl.updateLastWindowKey()
				rl.Flush()
				rl.UpdateLastWindowCount()
			case <-rl.stopTicker:
				log.Printf("Stopping rate limit worker")
				rl.ticker.Stop()
				return
			}
		}
	}(rl)

	return nil
}

// http handler to take care of too many api request.
func (rl *RateLimiter) MiddleWareHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rl.IsOverLimit() {
			w.Write([]byte(fmt.Sprintf("Api call limit of %d per %f seconds reached, please wait for %d seconds", rl.Limit, uint64(rl.Interval.Seconds()), rl.retryTime)))
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
