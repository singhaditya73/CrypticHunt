# Startup script for Holmes server
# This ensures all environment variables from .env are loaded

Write-Host "Starting Holmes Server..." -ForegroundColor Green

# Set required environment variables
$env:ENVIRONMENT = "DEV"
$env:REDIS_ADDR = "localhost:6379"

# MinIO configuration (set BUCKET_USE_SSL to false for local development)
$env:BUCKET_USE_SSL = "false"

# Optional: Set these if you're not using .env file
# $env:ADMIN_PASS = "holmes"
# $env:DB_NAME = "database.db"
# $env:SECRET = "your-secret-key-change-this-in-production"
# $env:BUCKET_ENDPOINT = "localhost:9000"
# $env:BUCKET_ACCESSKEY = "minioadmin"
# $env:BUCKET_SECRETKEY = "minioadmin"
# $env:BUCKET_NAME = "holmes"

# Start the server
& ".\holmes.exe"
