package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ajiwo/ratelimit"
	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Create rate limiter with memory backend
	limiter, err := ratelimit.New(
		ratelimit.WithBackend(memory.New()),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				SetKey("client").
				AddQuota("default", 10, time.Minute). // Allow 10 requests per minute
				Build(),
		),
		ratelimit.WithBaseKey("api"),
	)
	if err != nil {
		log.Fatalf("Failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	// Create Echo instance
	e := echo.New()

	// Add some middleware for logging and recovery
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Define routes
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello from protected endpoint!\n")
	}, RateLimitMiddleware(limiter))

	e.GET("/slow", func(c echo.Context) error {
		time.Sleep(100 * time.Millisecond) // Simulate some processing time
		return c.String(http.StatusOK, "This is a slow endpoint!\n")
	}, RateLimitMiddleware(limiter))

	e.GET("/unprotected", func(c echo.Context) error {
		return c.String(http.StatusOK, "This endpoint is not rate limited.\n")
	})

	// Start server
	log.Println("Echo server with rate limiting listening on :8082")
	if err := e.Start(":8082"); err != nil && err != http.ErrServerClosed {
		// Close the limiter before exiting to ensure resources are cleaned up
		if closeErr := limiter.Close(); closeErr != nil {
			log.Printf("Error closing rate limiter: %v", closeErr)
		}
		log.Printf("Server failed to start: %v", err)
		return // Exit the main function, which will execute the defer statement
	}
}

// RateLimitMiddleware creates an Echo middleware that applies rate limiting
func RateLimitMiddleware(limiter *ratelimit.RateLimiter) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			clientID := c.RealIP()

			// Check if request is allowed by rate limiter
			allowed, err := limiter.Allow(
				c.Request().Context(),
				ratelimit.AccessOptions{Key: clientID},
			)
			if err != nil {
				log.Printf("Rate limiter error for %s: %v", clientID, err)
				return echo.NewHTTPError(http.StatusInternalServerError, "Internal Server Error")
			}

			log.Printf("Client %s allowed: %t", clientID, allowed)

			// If rate limit exceeded, return 429 with rate limit headers
			if !allowed {
				// Get rate limit statistics to include in the response
				var stats strategies.Results
				statsOK, err := limiter.Peek(
					c.Request().Context(),
					ratelimit.AccessOptions{
						Key:    clientID,
						Result: &stats,
					},
				)
				if err == nil && statsOK && len(stats) > 0 {
					result := stats["default"]
					c.Response().Header().Set("X-RateLimit-Limit", "10")
					c.Response().Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
					c.Response().Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", result.Reset.Unix()))
					c.Response().Header().Set("Retry-After", fmt.Sprintf("%.0f", time.Until(result.Reset).Seconds()))
				}

				return echo.NewHTTPError(http.StatusTooManyRequests, "Rate limit exceeded")
			}

			// Request is allowed, continue to next handler
			return next(c)
		}
	}
}
