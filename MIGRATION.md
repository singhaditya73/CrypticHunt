# Migration Guide: Old System → Optimized System

## Overview

This guide helps you upgrade from the polling-based system to the optimized SSE-based system.

---

## ⚠️ Breaking Changes

### 1. AuthHandler Constructor Changed

**Before:**
```go
ah := handlers.NewAuthHandler(us)
```

**After:**
```go
broadcaster := services.NewBroadcaster("", "", 0) // or with Redis
ah := handlers.NewAuthHandler(us, broadcaster)
```

### 2. New Dependencies

Add to your project:
```bash
go get github.com/redis/go-redis/v9
go get golang.org/x/time/rate
```

### 3. Database Schema Updates

The application will automatically create new indexes on first run. No manual migration needed.

---

## 📋 Step-by-Step Migration

### Step 1: Backup

```bash
# Backup your database
cp hunt.db hunt.db.backup

# Backup your current binary
cp holmes.exe holmes.exe.old
```

### Step 2: Update Dependencies

```bash
go get github.com/redis/go-redis/v9
go get golang.org/x/time/rate
go mod tidy
```

### Step 3: Update main.go

Add broadcaster initialization before creating AuthHandler:

```go
// After creating UserService
broadcaster := services.NewBroadcaster("", "", 0)
log.Println("Broadcaster initialized for real-time updates")

// Update AuthHandler creation
ah := handlers.NewAuthHandler(us, broadcaster)
```

### Step 4: Regenerate Templates

```bash
templ generate
```

### Step 5: Build

```bash
go build -o holmes.exe ./app
```

### Step 6: Test

```bash
# Start the server
./holmes.exe

# In another terminal, check health
curl http://localhost:4200/api/health

# Check SSE endpoint
curl -N http://localhost:4200/api/events
```

### Step 7: Verify

1. Open the hunt page in a browser
2. Open browser DevTools → Network tab
3. Look for `/api/events` with type `eventsource`
4. Open a question in one browser tab
5. Verify the lock appears in another tab instantly

---

## 🔄 Rollback Plan

If something goes wrong:

### Quick Rollback

```bash
# Stop the new server
pkill holmes

# Restore old binary
cp holmes.exe.old holmes.exe

# Restore database if needed
cp hunt.db.backup hunt.db

# Restart
./holmes.exe
```

### Database Rollback

The new indexes don't break anything. If you want to remove them:

```sql
DROP INDEX IF EXISTS idx_question_locks_question_id;
DROP INDEX IF EXISTS idx_question_timers_team_question;
DROP INDEX IF EXISTS idx_question_attempts_team_question;
DROP INDEX IF EXISTS idx_team_completed_questions;
DROP INDEX IF EXISTS idx_team_hint_unlocked;
```

---

## 🆕 New Features Available

### 1. SSE Real-Time Updates

**Endpoint**: `GET /api/events`

Users automatically connect on the hunt page. No configuration needed.

### 2. ETag Caching

**Endpoint**: `GET /api/locked-questions`

Add `If-None-Match` header with previous ETag to get 304 responses.

### 3. Health Monitoring

**Public**: `GET /api/health`  
**Admin**: `GET /api/metrics`

```bash
# Check server health
curl http://localhost:4200/api/health | jq

# View detailed metrics (requires admin auth)
curl http://localhost:4200/api/metrics | jq
```

### 4. Rate Limiting

Automatic per-IP rate limiting:
- API endpoints: 10 req/s, burst 20
- Sensitive operations: 2 req/s, burst 5

Adjust in `handlers/ratelimit.go` if needed.

### 5. Atomic Locking

Question locking is now race-condition-free. No configuration needed.

---

## 🧪 Testing the Migration

### Test Checklist

- [ ] Server starts without errors
- [ ] Login works
- [ ] Hunt page loads
- [ ] Question cards display
- [ ] Opening a question shows lock in other tabs
- [ ] Solving a question updates leaderboard
- [ ] `/api/health` returns 200 OK
- [ ] SSE connection established (check Network tab)
- [ ] Polling fallback works if SSE disabled
- [ ] Admin panel functions correctly

### Load Testing

```bash
# Create test users first
# Then run k6
k6 run loadtest.js
```

Expected results:
- ✅ Request duration p95 < 500ms
- ✅ Error rate < 5%
- ✅ 1000+ virtual users supported

---

## 🐛 Common Issues

### Issue 1: "undefined: services.Broadcaster"

**Cause**: Old import cache

**Solution**:
```bash
go clean -cache
go mod tidy
go build -o holmes.exe ./app
```

### Issue 2: SSE Connection Fails

**Symptoms**: Browser shows "Failed to connect" for EventSource

**Solutions**:
1. Check firewall rules
2. Verify port 4200 is accessible
3. Check nginx/proxy configuration if behind one
4. Ensure `Content-Type: text/event-stream` header is set

### Issue 3: High Memory Usage

**Cause**: SSE connections not cleaning up

**Solution**: The broadcaster automatically cleans up on disconnect. If memory still grows:

```go
// In services/broadcaster.go, adjust channel buffer sizes
Channel: make(chan Event, 50), // Reduce from 100
```

### Issue 4: Rate Limiting Too Strict

**Symptoms**: Users getting 429 errors

**Solution**: Edit `handlers/ratelimit.go`:

```go
func ModerateRateLimitMiddleware() echo.MiddlewareFunc {
    return RateLimitMiddleware(20, 40) // Increase from 10, 20
}
```

### Issue 5: Database Locks

**Symptoms**: "database is locked" errors

**Solution**: Increase connection pool:

```go
// In database/database.go
db.SetMaxOpenConns(100) // Increase from 50
```

---

## 📊 Monitoring After Migration

### Key Metrics to Watch

1. **SSE Connections**
   ```bash
   curl http://localhost:4200/api/health | jq '.sse_connections'
   ```
   Expected: Close to number of active users

2. **Database Connections**
   ```bash
   curl http://localhost:4200/api/health | jq '.database.open_connections'
   ```
   Expected: < 30 under normal load

3. **Memory Usage**
   ```bash
   curl http://localhost:4200/api/health | jq '.server.memory_alloc_mb'
   ```
   Expected: Stable, not constantly growing

4. **Response Times**
   - Monitor application logs
   - Use k6 for periodic benchmarks
   - Set up Prometheus/Grafana for production

### Setting Up Alerts

**Simple Script** (cron every 5 minutes):

```bash
#!/bin/bash
HEALTH=$(curl -s http://localhost:4200/api/health)
STATUS=$(echo $HEALTH | jq -r '.status')

if [ "$STATUS" != "healthy" ]; then
    echo "Server unhealthy at $(date)" | mail -s "Alert: Holmes Server" admin@example.com
fi
```

---

## 🚀 Optimization Tips

### 1. Enable Redis for Production

```bash
export REDIS_ADDR="localhost:6379"
export REDIS_PASSWORD="yourPassword"
./holmes.exe
```

See `REDIS_SETUP.md` for full guide.

### 2. Tune for Your Load

**Low traffic (<100 users):**
```go
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(10)
```

**Medium traffic (100-500 users):**
```go
db.SetMaxOpenConns(50)  // Default
db.SetMaxIdleConns(25)
```

**High traffic (500-2000 users):**
```go
db.SetMaxOpenConns(100)
db.SetMaxIdleConns(50)
```

### 3. Use a Reverse Proxy

**Nginx configuration for SSE:**

```nginx
location /api/events {
    proxy_pass http://localhost:4200;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 24h;
}
```

### 4. Monitor with Prometheus

Add prometheus metrics endpoint (future enhancement):

```go
// Add to handlers
import "github.com/prometheus/client_golang/prometheus/promhttp"

e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
```

---

## ✅ Success Criteria

Your migration is successful when:

- ✅ Zero downtime during deployment
- ✅ All existing features work as before
- ✅ Real-time updates appear instantly (<1 second)
- ✅ Server handles expected load without errors
- ✅ Response times are improved (check k6 results)
- ✅ No memory leaks after 24 hours
- ✅ Health endpoint reports "healthy"

---

## 📞 Support

If you encounter issues:

1. Check the logs: `journalctl -u holmes -f` (systemd) or `tail -f holmes.log`
2. Run health check: `curl http://localhost:4200/api/health`
3. Test SSE: `curl -N http://localhost:4200/api/events`
4. Review error logs in the console
5. Compare with `OPTIMIZATION_SUMMARY.md`

---

## 🎉 Post-Migration

After successful migration:

1. **Remove old binaries**: `rm holmes.exe.old`
2. **Document your setup**: Note any custom configurations
3. **Set up monitoring**: Health checks, log aggregation
4. **Run load tests**: Benchmark your specific workload
5. **Optimize further**: Based on your actual traffic patterns

Congratulations! Your system is now optimized for scalability! 🚀
