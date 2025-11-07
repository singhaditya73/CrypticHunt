package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/sessions"
	"github.com/joho/godotenv"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/namishh/holmes/database"
	"github.com/namishh/holmes/handlers"
	"github.com/namishh/holmes/services"
)

func initMinioClient() (*minio.Client, error) {
	endpoint := os.Getenv("BUCKET_ENDPOINT")
	accessKeyID := os.Getenv("BUCKET_ACCESSKEY")
	secretAccessKey := os.Getenv("BUCKET_SECRETKEY")
	bucketName := os.Getenv("BUCKET_NAME")
	
	// If MinIO is not configured, return nil (optional dependency)
	if endpoint == "" {
		log.Println("MinIO not configured (BUCKET_ENDPOINT not set) - file uploads will be disabled")
		return nil, nil
	}
	
	// Use SSL only if explicitly enabled, default to false for local development
	useSSL := os.Getenv("BUCKET_USE_SSL") == "true"
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Printf("Warning: Failed to initialize MinIO client: %v - file uploads will be disabled", err)
		return nil, nil
	}

	// Create bucket if it doesn't exist
	ctx := context.Background()
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		log.Printf("Warning: Failed to check if bucket exists: %v - file uploads will be disabled", err)
		return nil, nil
	}
	
	if !exists {
		err = minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			log.Printf("Warning: Failed to create bucket '%s': %v - file uploads will be disabled", bucketName, err)
			return nil, nil
		}
		log.Printf("Successfully created MinIO bucket: %s", bucketName)
		
		// Set bucket policy to make it publicly readable
		policy := fmt.Sprintf(`{
			"Version": "2012-10-17",
			"Statement": [{
				"Effect": "Allow",
				"Principal": {"AWS": ["*"]},
				"Action": ["s3:GetObject"],
				"Resource": ["arn:aws:s3:::%s/*"]
			}]
		}`, bucketName)
		
		err = minioClient.SetBucketPolicy(ctx, bucketName, policy)
		if err != nil {
			log.Printf("Warning: Failed to set public policy on bucket: %v", err)
		} else {
			log.Printf("Bucket '%s' is now publicly readable", bucketName)
		}
	}

	log.Printf("Successfully connected to MinIO bucket '%s' at %s (SSL: %v)", bucketName, endpoint, useSSL)

	return minioClient, nil
}

func main() {
	if os.Getenv("ENVIRONMENT") == "DEV" {
		err := godotenv.Load()
		if err != nil {
			log.Fatal("Error loading .env file")
		}
	}

	minioClient, err := initMinioClient()
	if err != nil {
		log.Printf("Warning: MinIO initialization failed: %v", err)
		// Continue anyway - MinIO is optional
	}

	e := echo.New()
	SECRET_KEY := os.Getenv("SECRET")
	DB_NAME := os.Getenv("DB_NAME")
	e.HTTPErrorHandler = handlers.CustomHTTPErrorHandler

	e.Use(middleware.Logger())
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(20)))
	
	// CORS protection
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{os.Getenv("ALLOWED_ORIGIN")}, // Set in .env
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
		AllowCredentials: true,
	}))
	
	// Security headers
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "SAMEORIGIN",
		HSTSMaxAge:            31536000,
		ContentSecurityPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:;",
	}))
	
	e.Use(session.Middleware(sessions.NewCookieStore([]byte(SECRET_KEY))))

	e.Static("/static", "public")

	store, err := database.NewDatabaseStore(DB_NAME)
	if err != nil {
		e.Logger.Fatalf("failed to create store: %s", err)
	}

	// Initialize broadcaster for SSE (with optional Redis support)
	redisAddr := os.Getenv("REDIS_ADDR")       // e.g., "localhost:6379"
	redisPassword := os.Getenv("REDIS_PASSWORD") // leave empty if no password
	redisDB := 0                                  // default DB
	
	broadcaster := services.NewBroadcaster(redisAddr, redisPassword, redisDB)
	log.Println("Broadcaster initialized for real-time updates")

	us := services.NewUserService(services.User{}, store, minioClient)
	ah := handlers.NewAuthHandler(us, broadcaster)
	
	// Start periodic cleanup of stale question locks (every 1 minute)
	// Locks older than 2 minutes are automatically removed
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		
		// Run cleanup immediately on startup
		if err := us.CleanupStaleLocks(); err != nil {
			log.Printf("Error in initial lock cleanup: %v", err)
		}
		
		// Then run periodically
		for range ticker.C {
			if err := us.CleanupStaleLocks(); err != nil {
				log.Printf("Error in periodic lock cleanup: %v", err)
			}
		}
	}()
	
	// Start periodic cleanup of admin rate limiter (every 30 minutes)
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		
		// Run periodically
		for range ticker.C {
			handlers.CleanupAdminRateLimiter()
		}
	}()

	handlers.SetupRoutes(e, ah)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "4200"
	}
	log.Printf("Starting server on :%s", port)
	e.Logger.Fatal(e.Start(":" + port))
}
