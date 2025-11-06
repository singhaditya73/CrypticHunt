package handlers

import (
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"
)

// RateLimiter stores rate limiters per IP address
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerSecond float64, burst int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(requestsPerSecond),
		burst:    burst,
	}
}

// getLimiter returns the rate limiter for a given IP
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[ip]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[ip] = limiter
	}

	return limiter
}

// Cleanup removes old limiters periodically
func (rl *RateLimiter) Cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			rl.mu.Lock()
			// Clear all limiters to prevent memory leak
			// In production, you'd want to track last access time
			if len(rl.limiters) > 10000 {
				rl.limiters = make(map[string]*rate.Limiter)
			}
			rl.mu.Unlock()
		}
	}()
}

// RateLimitMiddleware creates an Echo middleware for rate limiting
func RateLimitMiddleware(requestsPerSecond float64, burst int) echo.MiddlewareFunc {
	limiter := NewRateLimiter(requestsPerSecond, burst)
	limiter.Cleanup(10 * time.Minute)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()
			
			if !limiter.getLimiter(ip).Allow() {
				return c.JSON(http.StatusTooManyRequests, map[string]string{
					"error": "Rate limit exceeded. Please slow down your requests.",
				})
			}

			return next(c)
		}
	}
}

// StrictRateLimitMiddleware is for more sensitive endpoints
func StrictRateLimitMiddleware() echo.MiddlewareFunc {
	return RateLimitMiddleware(2, 5) // 2 requests per second, burst of 5
}

// ModerateRateLimitMiddleware is for general API endpoints
func ModerateRateLimitMiddleware() echo.MiddlewareFunc {
	return RateLimitMiddleware(10, 20) // 10 requests per second, burst of 20
}
