package main

import (
	"log"
	"os"

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
	
	// If MinIO is not configured, return nil (optional dependency)
	if endpoint == "" {
		log.Println("MinIO not configured (BUCKET_ENDPOINT not set) - file uploads will be disabled")
		return nil, nil
	}
	
	useSSL := true
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Printf("Warning: Failed to initialize MinIO client: %v - file uploads will be disabled", err)
		return nil, nil
	}

	log.Println("Successfully connected to the MinIO bucket")

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

	handlers.SetupRoutes(e, ah)

	// Start server
	log.Println("Starting server on :4200")
	e.Logger.Fatal(e.Start(":4200"))
}
