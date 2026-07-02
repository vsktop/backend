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

// allowNScript is an atomic Lua script that performs the sliding-window rate limit check and
// increment in a single Redis round-trip, preventing races between concurrent callers.
//
// Returns: {allowed (0|1), remaining, retryAfterSeconds}
var allowNScript = rueidis.NewLuaScript(`
local key      = KEYS[1]
local now      = tonumber(ARGV[1])
local cutoff   = tonumber(ARGV[2])
local rate     = tonumber(ARGV[3])
local n        = tonumber(ARGV[4])
local window   = tonumber(ARGV[5])

redis.call('ZREMRANGEBYSCORE', key, '0', cutoff)
local count = tonumber(redis.call('ZCARD', key))

if count + n > rate then
    local remaining = math.max(0, rate - count)
    local ttl = redis.call('TTL', key)
    if ttl < 0 then ttl = 0 end
    return {0, remaining, ttl}
end

for i = 0, n - 1 do
    local score = now + i
    redis.call('ZADD', key, score, tostring(now) .. ':' .. tostring(i))
end
redis.call('EXPIRE', key, window)
return {1, rate - count - n, 0}
`)

func (r *RateLimiter) AllowN(ctx context.Context, key string, n int64, l Limit) (RateLimitResult, error) {
	now := time.Now().UnixNano()
	cutoff := now - l.Period.Nanoseconds()
	windowSec := int64(l.Period.Seconds()) + 1

	vals, err := allowNScript.Exec(ctx, r.client, []string{key}, []string{
		i64toa(now), i64toa(cutoff), i64toa(l.Rate), i64toa(n), i64toa(windowSec),
	}).AsIntSlice()
	if err != nil {
		return RateLimitResult{}, err
	}

	return RateLimitResult{
		Allowed:    int(vals[0]),
		Remaining:  vals[1],
		RetryAfter: time.Duration(vals[2]) * time.Second,
	}, nil
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
		ttl, err := r.client.Do(ctx, r.client.B().Ttl().Key(key).Build()).AsInt64()
		if err == nil && ttl > 0 {
			retryAfter = time.Duration(ttl) * time.Second
		}
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

