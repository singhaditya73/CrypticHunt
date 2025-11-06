# Startup script for Holmes server
# This ensures all environment variables from .env are loaded

Write-Host "Starting Holmes Server..." -ForegroundColor Green

# Set required environment variables
$env:ENVIRONMENT = "DEV"
$env:REDIS_ADDR = "localhost:6379"

# Optional: Set these if you're not using .env file
# $env:ADMIN_PASS = "holmes"
# $env:DB_NAME = "database.db"
# $env:SECRET = "your-secret-key-change-this-in-production"

# Start the server
& ".\holmes.exe"
