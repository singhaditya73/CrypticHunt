# Load Testing & Performance Optimization

## Overview

This document describes the performance optimizations implemented and how to run load tests.

## Optimizations Implemented

### 1. **Server-Sent Events (SSE) for Real-Time Updates**
- Replaced constant polling with SSE push notifications
- Reduces request load by ~80-90%
- Events broadcast: `question_locked`, `question_unlocked`, `question_solved`, `leaderboard_update`

### 2. **Atomic Question Locking**
- Uses SQL `INSERT...SELECT...WHERE NOT EXISTS` pattern
- Prevents race conditions where multiple users lock the same question
- Returns error if question already locked

### 3. **Database Optimizations**
- **Connection Pooling:**
  - Max open connections: 50
  - Max idle connections: 25
  - Connection max lifetime: 5 minutes
- **Indexes added:**
  - `idx_question_locks_question_id`
  - `idx_question_timers_team_question`
  - `idx_question_attempts_team_question`
  - `idx_team_completed_questions`
  - `idx_team_hint_unlocked`

### 4. **Caching & Conditional Requests**
- ETag support for `/api/locked-questions`
- Returns `304 Not Modified` when data unchanged
- `Cache-Control: public, max-age=5` header

### 5. **Rate Limiting**
- Moderate rate limit: 10 req/s, burst of 20
- Strict rate limit: 2 req/s, burst of 5
- Per-IP tracking with automatic cleanup

### 6. **Page Visibility API**
- Pauses SSE/polling when tab is hidden
- Automatically reconnects when tab becomes visible
- Reduces unnecessary background traffic

### 7. **Redis Pub/Sub (Optional)**
- Coordinates events across multiple server instances
- Falls back to standalone mode if Redis unavailable
- Channel: `hunt_events`

## Architecture

```
┌─────────────┐
│   Browser   │
│             │
│  ┌───────┐  │         ┌──────────────┐
│  │  SSE  │──┼────────>│ Go Server    │
│  └───────┘  │         │              │
│             │         │ ┌──────────┐ │      ┌───────┐
│  ┌───────┐  │ Fallback│ │Rate Limit│ │      │ Redis │
│  │Polling│──┼────────>│ │Middleware│ │<────>│Pub/Sub│
│  └───────┘  │         │ └──────────┘ │      └───────┘
└─────────────┘         │              │
                        │ ┌──────────┐ │
                        │ │Broadcast │ │
                        │ │ Service  │ │
                        │ └──────────┘ │
                        │              │
                        │ ┌──────────┐ │
                        │ │ SQLite   │ │
                        │ │ +Indexes │ │
                        │ └──────────┘ │
                        └──────────────┘
```

## Running Load Tests

### Prerequisites

1. Install k6: https://k6.io/docs/getting-started/installation/
2. Create test users in your database

### Create Test Users

```sql
-- Create 10 test users
INSERT INTO teams (email, password, name, points) VALUES
  ('test1@example.com', '$2a$10$hash1', 'testuser1', 0),
  ('test2@example.com', '$2a$10$hash2', 'testuser2', 0),
  ('test3@example.com', '$2a$10$hash3', 'testuser3', 0);
-- Add more as needed
```

Or use the register endpoint programmatically.

### Run the Load Test

```bash
# Basic load test
k6 run loadtest.js

# With custom base URL
k6 run --env BASE_URL=http://localhost:8080 loadtest.js

# Save results to file
k6 run --out json=results.json loadtest.js

# Run with specific stage
k6 run --stage 30s:100,1m:500,2m:1000 loadtest.js
```

### Interpreting Results

Key metrics to watch:

- **http_req_duration**: Should be <500ms for p95, <1s for p99
- **http_req_failed**: Should be <5%
- **SSE connections**: Monitor in `/api/metrics`
- **Database connections**: Check `/api/health`

## Health Monitoring

### Health Check Endpoint

```bash
curl http://localhost:8080/api/health
```

Response:
```json
{
  "status": "healthy",
  "timestamp": "2025-11-07T10:30:00Z",
  "database": {
    "status": "healthy",
    "open_connections": 5,
    "idle_connections": 3,
    "max_open_conns": 50
  },
  "server": {
    "goroutines": 25,
    "memory_alloc_mb": 45.2,
    "memory_sys_mb": 72.5,
    "num_gc": 12
  },
  "sse_connections": 342
}
```

### Metrics Endpoint (Admin Only)

```bash
curl http://localhost:8080/api/metrics
```

## Redis Configuration (Optional)

To enable Redis for multi-instance coordination:

### Environment Variables

```bash
export REDIS_ADDR="localhost:6379"
export REDIS_PASSWORD=""
export REDIS_DB=0
```

### Update main.go

```go
import "github.com/namishh/holmes/services"

// Initialize broadcaster with Redis
redisAddr := os.Getenv("REDIS_ADDR")
redisPassword := os.Getenv("REDIS_PASSWORD")
redisDB, _ := strconv.Atoi(os.Getenv("REDIS_DB"))

broadcaster := services.NewBroadcaster(redisAddr, redisPassword, redisDB)

// Pass to handler
authHandler := handlers.NewAuthHandler(userService, broadcaster)
```

## Performance Benchmarks

### Before Optimization
- Polling every 5 seconds
- 1000 users = 200 req/s to `/api/locked-questions`
- No caching
- No atomic locking

**Results:**
- High CPU usage
- Database connection exhaustion at ~500 users
- Response times >2s under load

### After Optimization
- SSE push notifications
- ETag caching for fallback polling
- Atomic SQL operations
- Rate limiting

**Expected Results:**
- <10 req/s to polling endpoint (most use SSE)
- Supports 1000+ concurrent SSE connections
- Response times <200ms for p95
- Minimal CPU/memory impact

## Troubleshooting

### High Memory Usage
- Check SSE connection count: `curl /api/health`
- Verify cleanup on disconnect
- Adjust `MAX_RECONNECT_ATTEMPTS` in frontend

### Database Connection Pool Exhausted
- Increase `SetMaxOpenConns` in `database.go`
- Check for long-running queries
- Monitor with `/api/metrics`

### SSE Not Working
- Check browser console for errors
- Verify `/api/events` returns `Content-Type: text/event-stream`
- Test with: `curl -N http://localhost:8080/api/events`

### Rate Limiting Too Strict
- Adjust limits in `handlers/ratelimit.go`
- Check IP address detection (proxy headers)

## Production Deployment

### Nginx Configuration

```nginx
location /api/events {
    proxy_pass http://localhost:8080;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 24h;
}

location /api/ {
    proxy_pass http://localhost:8080;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
```

### Recommended Settings
- Enable Redis for multi-instance setups
- Use a reverse proxy (Nginx, Caddy)
- Monitor with Prometheus/Grafana
- Set up log aggregation
- Configure automatic restarts

## Future Enhancements

1. **WebSocket Alternative**: Full duplex communication
2. **Caching Layer**: Redis for question data
3. **Database Replication**: Read replicas for queries
4. **CDN**: Static assets delivery
5. **Horizontal Scaling**: Load balancer + multiple instances
