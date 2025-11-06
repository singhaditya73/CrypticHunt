# 🔧 Troubleshooting Guide

## Admin Login Issues

### Problem: "Wrong password" when logging in with ADMIN_PASS

**Root Cause:** The `.env` file was not being loaded because `ENVIRONMENT=DEV` was not set.

**Solution:**
Always start the server using the startup script:
```powershell
.\start-server.ps1
```

Or manually set the environment variable:
```powershell
$env:ENVIRONMENT = "DEV"
$env:REDIS_ADDR = "localhost:6379"
.\holmes.exe
```

### Verify Admin Password
Check your `.env` file has:
```
ADMIN_PASS=holmes
ENVIRONMENT=DEV
```

### Test Admin Login
1. Go to `http://localhost:4200/sudo`
2. Enter password: `holmes`
3. Should redirect to `/su` (admin dashboard)

---

## User Registration/Login Issues

### Problem: "Wrong password" or "Unauthorized" on user login

**Possible Causes:**

#### 1. Session Cookie Issues
**Symptoms:** Can register but can't login, or login works sometimes but not always

**Solution:** Clear browser cookies and cache
- Chrome: Ctrl+Shift+Delete → Clear cookies
- Or use Incognito/Private mode for testing

#### 2. Database Issues
**Symptoms:** User can't login immediately after registration

**Check Database:**
```powershell
# Install SQLite if needed
# winget install SQLite.SQLite

# Open database
sqlite3 database.db

# Check users table
SELECT id, name, email, substr(password, 1, 20) as pwd_hash FROM teams;

# Exit
.quit
```

#### 3. Password Hashing Issues
**Symptoms:** Password comparison fails even with correct credentials

**Verify:**
1. Passwords are being hashed during registration (bcrypt)
2. Password comparison uses bcrypt.CompareHashAndPassword()

**Test Fix:**
```powershell
# Delete database and start fresh
Remove-Item database.db
.\start-server.ps1
```

The database will be recreated automatically with migrations.

---

## Common Issues

### Issue: "Connection lost" in SSE test page

**Cause:** Server not running or CORS issues

**Solution:**
1. Check server is running: `Get-Process holmes`
2. Check health endpoint: `curl http://localhost:4200/api/health`
3. Use the test endpoint: `http://localhost:4200/api/events-test` (no auth required)

### Issue: MinIO errors on startup

**Cause:** BUCKET_ENDPOINT is set but MinIO is not running

**Solution 1:** Comment out MinIO vars in `.env`:
```properties
# BUCKET_ENDPOINT=localhost:9000
# BUCKET_ACCESSKEY=minioadmin
# BUCKET_SECRETKEY=minioadmin
```

**Solution 2:** Start MinIO in Docker (WSL):
```bash
docker run -d -p 9000:9000 -p 9001:9001 \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  minio/minio server /data --console-address ":9001"
```

### Issue: Redis warnings on startup

**Message:** `maintnotifications disabled due to handshake error`

**Status:** ⚠️ This is a harmless warning - Redis pub/sub still works!

**Cause:** Redis client version compatibility

**Impact:** None - SSE and pub/sub work perfectly

---

## Debug Commands

### Check Server Status
```powershell
# Is server running?
Get-Process holmes

# Check health
curl http://localhost:4200/api/health

# Test SSE (no auth)
curl -N http://localhost:4200/api/events-test
```

### Check Database
```powershell
# Count users
sqlite3 database.db "SELECT COUNT(*) as total_users FROM teams;"

# List all users
sqlite3 database.db "SELECT id, name, email FROM teams;"

# Check question locks
sqlite3 database.db "SELECT * FROM question_locks;"
```

### Check Environment Variables
```powershell
# While server is running, in same PowerShell window:
$env:ENVIRONMENT
$env:ADMIN_PASS
$env:REDIS_ADDR
```

---

## Clean Slate Reset

If nothing works, start fresh:

```powershell
# 1. Stop server
Stop-Process -Name holmes -Force

# 2. Backup database (optional)
Copy-Item database.db database.db.backup

# 3. Delete database
Remove-Item database.db

# 4. Clear browser cookies
# Go to browser settings → Clear cookies for localhost:4200

# 5. Rebuild and start
go build -o holmes.exe ./app
.\start-server.ps1

# 6. Register new user
# Go to http://localhost:4200/register
```

---

## Still Having Issues?

### Enable Debug Logging

Add to `handlers/auth.go` after line 174 (in LoginHandler):
```go
log.Printf("DEBUG: User found: %+v", user)
log.Printf("DEBUG: Input password: %s", c.FormValue("password"))
log.Printf("DEBUG: Stored hash: %s", user.Password)
```

Rebuild and check logs:
```powershell
go build -o holmes.exe ./app
.\start-server.ps1
```

### Check Session Storage

Add to `handlers/auth.go` after session save:
```go
log.Printf("DEBUG: Session values set: %+v", sess.Values)
```

---

## Quick Tests

### Test 1: Environment Variables
```powershell
$env:ENVIRONMENT = "DEV"
go run -c "package main; import (\"fmt\"; \"os\"; \"github.com/joho/godotenv\"); func main() { godotenv.Load(); fmt.Println(\"ADMIN_PASS:\", os.Getenv(\"ADMIN_PASS\")) }"
```

Should output: `ADMIN_PASS: holmes`

### Test 2: Admin Endpoint
```powershell
curl http://localhost:4200/sudo
```

Should return HTML with admin login form

### Test 3: SSE Endpoint
```powershell
curl -N -H "Accept: text/event-stream" http://localhost:4200/api/events-test
```

Should stream events (press Ctrl+C to stop)

---

## Contact & Support

If issues persist:
1. Check all environment variables are set
2. Verify `.env` file exists and has correct values
3. Clear browser cookies completely
4. Try incognito/private mode
5. Check server logs for errors
6. Delete and recreate database
