package redis

import (
	"context"
	"time"

	"github.com/redis/rueidis"
)

type Limit struct {
	Rate   int64
	Period time.Duration
}

func PerSecond(n int64) Limit { return Limit{Rate: n, Period: time.Second} }
func PerMinute(n int64) Limit { return Limit{Rate: n, Period: time.Minute} }
func PerHour(n int64) Limit   { return Limit{Rate: n, Period: time.Hour} }

type RateLimitResult struct {
	Allowed    int
	Remaining  int64
	RetryAfter time.Duration
}

type RateLimiter struct {
	client rueidis.Client
}

func NewRateLimiter(client rueidis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

func (r *RateLimiter) Allow(ctx context.Context, key string, l Limit) (RateLimitResult, error) {
	return r.AllowN(ctx, key, 1, l)
}

func (r *RateLimiter) AllowN(ctx context.Context, key string, n int64, l Limit) (RateLimitResult, error) {
	now := time.Now().UnixNano()
	cutoff := now - l.Period.Nanoseconds()
	windowSec := int64(l.Period.Seconds()) + 1

	resps := r.client.DoMulti(ctx,
		r.client.B().Zremrangebyscore().Key(key).Min("0").Max(i64toa(cutoff)).Build(),
		r.client.B().Zcard().Key(key).Build(),
	)
	if err := resps[0].Error(); err != nil {
		return RateLimitResult{}, err
	}
	count, err := resps[1].AsInt64()
	if err != nil {
		return RateLimitResult{}, err
	}

	remaining := l.Rate - count
	if remaining < n {
		if remaining < 0 {
			remaining = 0
		}
		ttl, _ := r.client.Do(ctx, r.client.B().Ttl().Key(key).Build()).AsInt64()
		return RateLimitResult{
			Allowed:    0,
			Remaining:  remaining,
			RetryAfter: time.Duration(ttl) * time.Second,
		}, nil
	}

	args := make([]string, 0, n*2)
	for i := int64(0); i < n; i++ {
		ts := i64toa(now + i)
		args = append(args, ts, ts)
	}
	r.client.DoMulti(ctx,
		r.client.B().Arbitrary("ZADD").Keys(key).Args(args...).Build(),
		r.client.B().Expire().Key(key).Seconds(windowSec).Build(),
	)

	return RateLimitResult{Allowed: 1, Remaining: remaining - n}, nil
}

func (r *RateLimiter) Check(ctx context.Context, key string, l Limit) (RateLimitResult, error) {
	now := time.Now().UnixNano()
	cutoff := now - l.Period.Nanoseconds()

	resps := r.client.DoMulti(ctx,
		r.client.B().Zremrangebyscore().Key(key).Min("0").Max(i64toa(cutoff)).Build(),
		r.client.B().Zcard().Key(key).Build(),
	)
	if err := resps[0].Error(); err != nil {
		return RateLimitResult{}, err
	}
	count, err := resps[1].AsInt64()
	if err != nil {
		return RateLimitResult{}, err
	}

	remaining := l.Rate - count
	if remaining < 0 {
		remaining = 0
	}
	allowed := 1
	var retryAfter time.Duration
	if count >= l.Rate {
		allowed = 0
		ttl, _ := r.client.Do(ctx, r.client.B().Ttl().Key(key).Build()).AsInt64()
		retryAfter = time.Duration(ttl) * time.Second
	}
	return RateLimitResult{Allowed: allowed, Remaining: remaining, RetryAfter: retryAfter}, nil
}

func i64toa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
