# Redis Setup Guide

## Why Redis?

Redis enables **multi-instance coordination** for the hunt application. When you have multiple Go server instances (for load balancing), Redis pub/sub ensures that events broadcast from one server reach clients connected to all other servers.

**Without Redis**: Only clients connected to the same server instance receive SSE updates  
**With Redis**: All clients across all server instances receive SSE updates in real-time

---

## Installation

### Windows

**Option 1: Using WSL2 (Recommended)**
```bash
wsl --install
wsl
sudo apt update
sudo apt install redis-server
```

**Option 2: Windows Native Build**
Download from: https://github.com/tporadowski/redis/releases
Or use Chocolatey:
```powershell
choco install redis-64
```

### macOS
```bash
brew install redis
brew services start redis
```

### Linux (Ubuntu/Debian)
```bash
sudo apt update
sudo apt install redis-server
sudo systemctl enable redis-server
sudo systemctl start redis-server
```

### Docker
```bash
docker run -d \
  --name redis \
  -p 6379:6379 \
  redis:latest
```

---

## Configuration

### Basic Redis Config

Edit `/etc/redis/redis.conf` (Linux/macOS) or `C:\Program Files\Redis\redis.windows.conf` (Windows):

```conf
# Bind to localhost only for development
bind 127.0.0.1

# Set a password (optional but recommended)
requirepass yourStrongPasswordHere

# Max memory (optional)
maxmemory 256mb
maxmemory-policy allkeys-lru

# Persistence (optional - not needed for pub/sub)
save ""
```

Restart Redis after changes:
```bash
sudo systemctl restart redis-server  # Linux
brew services restart redis           # macOS
```

---

## Testing Redis Connection

### Using redis-cli

```bash
# Connect to Redis
redis-cli

# Test connection
127.0.0.1:6379> PING
PONG

# If password is set
redis-cli -a yourPassword
```

### Using Go

Create a test file `test_redis.go`:

```go
package main

import (
    "context"
    "fmt"
    "github.com/redis/go-redis/v9"
)

func main() {
    ctx := context.Background()
    
    client := redis.NewClient(&redis.Options{
        Addr:     "localhost:6379",
        Password: "", // Set your password if configured
        DB:       0,
    })
    
    pong, err := client.Ping(ctx).Result()
    if err != nil {
        fmt.Println("Error:", err)
        return
    }
    
    fmt.Println("Redis connection successful:", pong)
}
```

Run: `go run test_redis.go`

---

## Configuring the Hunt Application

### Development (Local)

Set environment variables before starting the server:

**Windows PowerShell:**
```powershell
$env:REDIS_ADDR="localhost:6379"
$env:REDIS_PASSWORD=""
$env:REDIS_DB="0"
.\holmes.exe
```

**Linux/macOS:**
```bash
export REDIS_ADDR="localhost:6379"
export REDIS_PASSWORD=""
export REDIS_DB=0
./holmes
```

### Production

#### Using .env file

Create/edit `.env`:
```env
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=yourStrongPassword
REDIS_DB=0
```

#### Using systemd (Linux)

Create `/etc/systemd/system/holmes.service`:

```ini
[Unit]
Description=Holmes Hunt Application
After=network.target redis-server.service

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/holmes
Environment="REDIS_ADDR=localhost:6379"
Environment="REDIS_PASSWORD=yourPassword"
Environment="REDIS_DB=0"
ExecStart=/opt/holmes/holmes
Restart=always

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl daemon-reload
sudo systemctl enable holmes
sudo systemctl start holmes
```

---

## Monitoring Redis

### Check if Redis is Running

```bash
# Linux/macOS
sudo systemctl status redis-server

# Or check process
ps aux | grep redis
```

### Monitor Redis Activity

```bash
# Connect to redis-cli
redis-cli

# Monitor all commands in real-time
127.0.0.1:6379> MONITOR

# Check connected clients
127.0.0.1:6379> CLIENT LIST

# Check memory usage
127.0.0.1:6379> INFO memory

# Check pub/sub channels
127.0.0.1:6379> PUBSUB CHANNELS
```

### Test Pub/Sub

**Terminal 1 (Subscribe):**
```bash
redis-cli
SUBSCRIBE hunt_events
```

**Terminal 2 (Publish):**
```bash
redis-cli
PUBLISH hunt_events "test message"
```

You should see the message appear in Terminal 1.

---

## Application Logs

When Redis is configured correctly, you'll see these logs on startup:

```
Successfully connected to Redis for pub/sub
Broadcaster initialized for real-time updates
Starting server on :4200
```

If Redis is unavailable:
```
Warning: Redis connection failed: dial tcp 127.0.0.1:6379: connect: connection refused. Running in standalone mode.
Broadcaster initialized for real-time updates
Starting server on :4200
```

The application will **still work** in standalone mode, but multi-instance coordination will be disabled.

---

## Scaling with Multiple Instances

### Architecture

```
                    ┌─────────────┐
                    │   Nginx     │
                    │ Load Balancer│
                    └──────┬──────┘
                           │
            ┌──────────────┼──────────────┐
            │              │              │
       ┌────▼────┐    ┌────▼────┐    ┌───▼─────┐
       │ Holmes  │    │ Holmes  │    │ Holmes  │
       │Instance1│    │Instance2│    │Instance3│
       │ :4200   │    │ :4201   │    │ :4202   │
       └────┬────┘    └────┬────┘    └────┬────┘
            │              │              │
            └──────────────┼──────────────┘
                           │
                    ┌──────▼──────┐
                    │    Redis    │
                    │  Pub/Sub    │
                    └─────────────┘
```

### Running Multiple Instances

**Instance 1:**
```bash
export PORT=4200
export REDIS_ADDR="localhost:6379"
./holmes &
```

**Instance 2:**
```bash
export PORT=4201
export REDIS_ADDR="localhost:6379"
./holmes &
```

**Instance 3:**
```bash
export PORT=4202
export REDIS_ADDR="localhost:6379"
./holmes &
```

### Nginx Load Balancer Config

```nginx
upstream holmes_backend {
    least_conn;
    server localhost:4200;
    server localhost:4201;
    server localhost:4202;
}

server {
    listen 80;
    server_name yourdomain.com;

    location /api/events {
        proxy_pass http://holmes_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "";
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 24h;
    }

    location / {
        proxy_pass http://holmes_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

---

## Troubleshooting

### Connection Refused

**Error**: `dial tcp 127.0.0.1:6379: connect: connection refused`

**Solutions**:
1. Check if Redis is running: `redis-cli ping`
2. Verify port 6379 is open: `netstat -an | grep 6379`
3. Check firewall rules
4. Verify `REDIS_ADDR` environment variable

### Authentication Failed

**Error**: `NOAUTH Authentication required`

**Solution**: Set `REDIS_PASSWORD` environment variable to match Redis config

### Wrong Database

**Error**: Events not propagating between instances

**Solution**: Ensure all instances use the same `REDIS_DB` number

### Redis Memory Full

**Error**: `OOM command not allowed when used memory > 'maxmemory'`

**Solutions**:
1. Increase `maxmemory` in redis.conf
2. Use `maxmemory-policy allkeys-lru`
3. Clear old data: `redis-cli FLUSHDB`

---

## Security Best Practices

### Production Checklist

- [ ] Set a strong `requirepass` in redis.conf
- [ ] Bind Redis to internal network only (`bind 10.0.0.1` instead of `0.0.0.0`)
- [ ] Use TLS for Redis connections if on public network
- [ ] Disable dangerous commands in redis.conf:
  ```conf
  rename-command FLUSHDB ""
  rename-command FLUSHALL ""
  rename-command CONFIG ""
  ```
- [ ] Run Redis as non-root user
- [ ] Enable Redis persistence if needed for recovery
- [ ] Monitor Redis with tools like Redis Insight or Prometheus

### Securing Redis Connection

If Redis is on a different server, use TLS:

```go
client := redis.NewClient(&redis.Options{
    Addr:     "redis.example.com:6380",
    Password: "yourPassword",
    DB:       0,
    TLSConfig: &tls.Config{
        MinVersion: tls.VersionTLS12,
    },
})
```

---

## Alternative: Redis Cloud

For production without managing Redis yourself:

1. **Redis Cloud** (free tier available): https://redis.com/try-free/
2. **AWS ElastiCache**: https://aws.amazon.com/elasticache/
3. **Azure Cache for Redis**: https://azure.microsoft.com/en-us/services/cache/
4. **Google Cloud Memorystore**: https://cloud.google.com/memorystore

Set `REDIS_ADDR` to the provided endpoint and `REDIS_PASSWORD` to the access key.

---

## Summary

✅ **Development**: Run Redis locally, no password  
✅ **Production**: Use Redis Cloud or self-hosted with password  
✅ **Multi-Instance**: All instances connect to same Redis  
✅ **Fallback**: App works without Redis (standalone mode)  
✅ **Monitoring**: Use redis-cli and application `/api/health` endpoint  

Redis setup is **optional** but highly recommended for production deployments with multiple server instances! 🚀
