package main

import (
	"bufio"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"route-margin-engine/handlers"
)

var logger *slog.Logger

func init() {
	// Structured JSON Logging for GCP Cloud Run (Observability ready)
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Parse local .env configuration file if present
	envPath := ".env"
	file, err := os.Open(envPath)
	if err != nil {
		logger.Warn("Local .env file not found. Relying on system environment variables.")
		return
	}
	defer file.Close()

	logger.Info("Parsing local .env configuration keys...")
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)

		os.Setenv(key, value)
	}
}

func main() {
	mux := http.NewServeMux()

	// Helper to serve files from static/ directory with fallback to root directory
	serveStaticOrRoot := func(w http.ResponseWriter, r *http.Request, filename string) {
		staticPath := filepath.Join("static", filename)
		if _, err := os.Stat(staticPath); err == nil {
			http.ServeFile(w, r, staticPath)
			return
		}
		// Fallback to root directory if static/ directory is missing in build context
		if _, err := os.Stat(filename); err == nil {
			http.ServeFile(w, r, filename)
			return
		}
		logger.Error("Static file not found", "file", filename)
		http.NotFound(w, r)
	}

	// Static Dashboard Assets Routing
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			// Check if request is for any other static asset (e.g. favicon.ico, images)
			serveStaticOrRoot(w, r, strings.TrimPrefix(r.URL.Path, "/"))
			return
		}
		serveStaticOrRoot(w, r, "index.html")
	})

	mux.HandleFunc("/app.js", func(w http.ResponseWriter, r *http.Request) {
		serveStaticOrRoot(w, r, "app.js")
	})

	mux.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) {
		serveStaticOrRoot(w, r, "style.css")
	})

	// Core API Endpoints: Protected with Rate Limiting
	profitHandler := rateLimitMiddleware(http.HandlerFunc(handlers.HandleProfitCalculation))
	mux.Handle("/api/v1/route-profit", profitHandler)
	mux.Handle("/api/profit", profitHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	loggingServer := loggingMiddleware(mux)

	logger.Info("Freight Route Margin Engine running on Cloud Run target", "port", port)
	if err := http.ListenAndServe(":"+port, loggingServer); err != nil {
		logger.Error("Server process fatal exit", "error", err)
		os.Exit(1)
	}
}

// --- Middleware Architectures ---

type clientLimiter struct {
	tokens     float64
	lastRefill time.Time
}

var (
	limiters   = make(map[string]*clientLimiter)
	limitersMu sync.Mutex
)

// rateLimitMiddleware provides token-bucket rate limiting per IP (10 requests/sec, burst 10)
func rateLimitMiddleware(next http.Handler) http.Handler {
	const (
		ratePerSec  = 10.0
		bucketBurst = 10.0
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		limitersMu.Lock()
		client, exists := limiters[ip]
		now := time.Now()

		if !exists {
			client = &clientLimiter{tokens: bucketBurst, lastRefill: now}
			limiters[ip] = client
		} else {
			elapsed := now.Sub(client.lastRefill).Seconds()
			client.tokens += elapsed * ratePerSec
			if client.tokens > bucketBurst {
				client.tokens = bucketBurst
			}
			client.lastRefill = now
		}

		if client.tokens < 1.0 {
			limitersMu.Unlock()
			logger.Warn("Rate limit exceeded", "ip", ip, "path", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "Too many requests. Please slow down route calculations."}`))
			return
		}

		client.tokens -= 1.0
		limitersMu.Unlock()

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs incoming HTTP transaction telemetry
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("HTTP Request Handled",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start).String(),
			"remote_ip", r.RemoteAddr,
		)
	})
}