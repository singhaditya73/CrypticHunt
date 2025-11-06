# Pre-Production Checklist

Use this checklist before deploying the optimized system to production.

---

## 🔧 Code Compilation

- [ ] All Go dependencies installed: `go mod tidy`
- [ ] Templates regenerated: `templ generate`
- [ ] Build successful: `go build -o holmes.exe ./app`
- [ ] No compilation errors
- [ ] No lint warnings (run `golangci-lint run` if available)

---

## 🧪 Local Testing

### Basic Functionality
- [ ] Server starts without errors
- [ ] Health endpoint responds: `curl http://localhost:4200/api/health`
- [ ] SSE endpoint works: `curl -N http://localhost:4200/api/events`
- [ ] Login works
- [ ] Register works
- [ ] Hunt page loads
- [ ] Question detail page loads
- [ ] Leaderboard loads
- [ ] Admin panel accessible (if applicable)

### Real-Time Features
- [ ] Opening a question in one tab shows lock in another tab (< 1 second)
- [ ] Solving a question updates all tabs
- [ ] Leaderboard updates after solve
- [ ] Lock releases after 5 wrong attempts
- [ ] Lock releases after solve

### Database
- [ ] All indexes created (check logs on first startup)
- [ ] Question locking works atomically (no race conditions)
- [ ] No "database locked" errors under normal use
- [ ] Connection pool metrics visible in `/api/health`

### Browser Testing
- [ ] Test in Chrome/Edge
- [ ] Test in Firefox
- [ ] Test in Safari (if available)
- [ ] Check browser console for JavaScript errors
- [ ] Verify EventSource connection in Network tab
- [ ] Verify polling fallback if SSE disabled

---

## 📈 Performance Testing

### Load Test Preparation
- [ ] Created 10+ test users
- [ ] Added sample questions
- [ ] k6 installed: https://k6.io
- [ ] Reviewed `loadtest.js` configuration

### Run Load Tests
- [ ] Run basic test: `k6 run loadtest.js`
- [ ] Test passes all thresholds:
  - [ ] p95 < 500ms
  - [ ] p99 < 1s
  - [ ] Error rate < 5%
- [ ] Review k6 report summary
- [ ] Check `/api/health` after test (server still healthy)

### Stress Testing
- [ ] Gradually increase load (100, 500, 1000 users)
- [ ] Monitor CPU usage (should stay < 80%)
- [ ] Monitor memory (should not leak)
- [ ] Monitor SSE connections in `/api/health`
- [ ] Monitor database connections
- [ ] Verify no crashes or errors

---

## 🔒 Security

### Authentication & Authorization
- [ ] Admin routes protected
- [ ] API endpoints require auth (except `/api/health`)
- [ ] Session management works correctly
- [ ] CSRF protection enabled where needed

### Rate Limiting
- [ ] Rate limits configured appropriately
- [ ] Test with rapid requests (should get 429)
- [ ] Verify rate limit doesn't affect legitimate users
- [ ] Check rate limiter cleanup (no memory leak)

### Input Validation
- [ ] All forms validate input
- [ ] SQL injection not possible (using prepared statements)
- [ ] XSS protection in templates
- [ ] File upload validation (if applicable)

---

## 🗄️ Database

### Schema & Indexes
- [ ] All tables created successfully
- [ ] All indexes present:
  - [ ] `idx_question_locks_question_id`
  - [ ] `idx_question_timers_team_question`
  - [ ] `idx_question_attempts_team_question`
  - [ ] `idx_team_completed_questions`
  - [ ] `idx_team_hint_unlocked`
- [ ] Foreign keys enforced
- [ ] Primary keys set correctly

### Connection Pool
- [ ] `SetMaxOpenConns` configured (default: 50)
- [ ] `SetMaxIdleConns` configured (default: 25)
- [ ] `SetConnMaxLifetime` configured (default: 5 min)
- [ ] Pool stats visible in `/api/metrics`

### Backup
- [ ] Database backup strategy defined
- [ ] Backup script tested
- [ ] Restore procedure documented

---

## 🔴 Redis (Optional)

If using Redis for multi-instance:

### Installation
- [ ] Redis installed and running
- [ ] Redis accessible: `redis-cli ping`
- [ ] Password configured (if production)
- [ ] Persistence configured (if needed)

### Configuration
- [ ] `REDIS_ADDR` environment variable set
- [ ] `REDIS_PASSWORD` set (if applicable)
- [ ] `REDIS_DB` set
- [ ] Application connects successfully (check logs)
- [ ] Pub/sub working (test with `redis-cli`)

### Testing
- [ ] Start multiple server instances
- [ ] Lock question in instance 1
- [ ] Verify lock appears in instance 2
- [ ] Solve question in instance 2
- [ ] Verify update in instance 1

---

## 📊 Monitoring

### Health Checks
- [ ] `/api/health` returns 200 OK
- [ ] `/api/health` shows all metrics:
  - [ ] Database status
  - [ ] DB connections
  - [ ] Server memory
  - [ ] Goroutines
  - [ ] SSE connections
- [ ] Set up monitoring script (cron/systemd timer)
- [ ] Set up alerts for unhealthy status

### Metrics (Admin)
- [ ] `/api/metrics` accessible to admin only
- [ ] Shows detailed database stats
- [ ] Shows runtime stats
- [ ] Shows SSE stats

### Logging
- [ ] Application logs to appropriate location
- [ ] Log level configured (info/debug/error)
- [ ] Log rotation set up
- [ ] Error logs monitored

---

## 🚀 Deployment

### Environment
- [ ] Production environment variables set
- [ ] Secrets secured (not in source code)
- [ ] Database path correct
- [ ] Static files accessible

### Server Configuration
- [ ] Systemd service file created (or equivalent)
- [ ] Service enabled: `systemctl enable holmes`
- [ ] Auto-restart configured
- [ ] Resource limits set (if needed)

### Reverse Proxy (if using)
- [ ] Nginx/Caddy installed and configured
- [ ] SSE endpoint configured correctly (no buffering)
- [ ] Proxy headers set (X-Real-IP, X-Forwarded-For)
- [ ] SSL/TLS certificate installed
- [ ] HTTP/2 enabled
- [ ] Proxy timeout for SSE set to 24h

### Firewall
- [ ] Port 4200 open (or proxy port)
- [ ] Database port NOT exposed publicly
- [ ] Redis port NOT exposed publicly (if using)
- [ ] Only necessary ports open

---

## 🔄 Rollback Plan

- [ ] Previous version backed up
- [ ] Database backed up
- [ ] Rollback procedure documented
- [ ] Tested rollback in staging

---

## 📝 Documentation

- [ ] README updated
- [ ] API endpoints documented
- [ ] Environment variables documented
- [ ] Deployment procedure documented
- [ ] Troubleshooting guide available
- [ ] Team trained on new features

---

## 🎯 Post-Deployment

### Immediate (First Hour)
- [ ] Monitor logs for errors
- [ ] Check `/api/health` every 5 minutes
- [ ] Test basic user flows
- [ ] Verify SSE connections working
- [ ] Monitor database connections

### Short Term (First 24 Hours)
- [ ] Monitor error rates
- [ ] Check response times
- [ ] Monitor memory usage (no leaks)
- [ ] Verify no connection pool exhaustion
- [ ] Check SSE reconnection behavior
- [ ] Monitor goroutine count

### Long Term (First Week)
- [ ] Run weekly load tests
- [ ] Review performance trends
- [ ] Optimize based on actual usage patterns
- [ ] Collect user feedback
- [ ] Document any issues and resolutions

---

## ✅ Final Checks

Before going live:

- [ ] All above sections completed
- [ ] Load test passed with flying colors
- [ ] Zero errors in logs during testing
- [ ] Team is ready for go-live
- [ ] Rollback plan ready and tested
- [ ] Monitoring and alerts configured
- [ ] Documentation complete
- [ ] Stakeholders informed

---

## 🎉 Go Live!

Once all checks pass:

```bash
# Start the service
sudo systemctl start holmes

# Check status
sudo systemctl status holmes

# Monitor logs
sudo journalctl -u holmes -f

# Check health
curl http://your-domain.com/api/health | jq
```

**Congratulations! Your optimized system is live!** 🚀

---

## 📞 Emergency Contacts

Document your emergency contacts:

- **Primary Developer**: [Name, Phone, Email]
- **Database Admin**: [Name, Phone, Email]
- **DevOps/Infrastructure**: [Name, Phone, Email]
- **On-Call Support**: [Contact Info]

---

## 🆘 Emergency Procedures

### If Server Crashes

1. Check logs: `journalctl -u holmes -n 100`
2. Check health: `curl http://localhost:4200/api/health`
3. Restart: `systemctl restart holmes`
4. If restart fails, rollback to previous version

### If Database Corrupted

1. Stop application
2. Restore from latest backup
3. Verify integrity: `sqlite3 hunt.db "PRAGMA integrity_check;"`
4. Restart application

### If High Memory Usage

1. Check SSE connections: `curl /api/health | jq '.sse_connections'`
2. If extremely high, restart to clear
3. Investigate memory leak
4. Apply fix and redeploy

### If Rate Limiting Too Strict

1. SSH to server
2. Edit `handlers/ratelimit.go`
3. Increase limits temporarily
4. Rebuild and restart
5. Plan permanent fix

---

This checklist ensures a smooth, safe, and successful production deployment! ✅
