package handlers

import (
	"github.com/labstack/echo/v4"
)

func SetupRoutes(e *echo.Echo, ah *AuthHandler) {
	e.GET("/", ah.flagsMiddleware(ah.HomeHandler))

	// AUTH ROUTES
	e.GET("/register", ah.flagsMiddleware(ah.RegisterHandler))
	e.POST("/register", ah.flagsMiddleware(ah.RegisterHandler))

	e.GET("/login", ah.flagsMiddleware(ah.LoginHandler))
	e.POST("/login", ah.flagsMiddleware(ah.LoginHandler))

	sugroup := e.Group("/sudo", csrfMiddleware())
	sugroup.GET("", ah.flagsMiddleware(ah.AdminHandler))
	sugroup.POST("", ah.flagsMiddleware(ah.AdminHandler))

	e.GET("/logout", ah.flagsMiddleware(ah.LogoutHandler))

	protectedgroup := e.Group("/hunt", ah.authMiddleware)
	protectedgroup.GET("", ah.Hunt)
	protectedgroup.GET("/leaderboard", ah.Leaderboard)
	protectedgroup.GET("/question/:id", ah.Question)
	protectedgroup.GET("/openhint/:id", ah.UnlockHint)
	protectedgroup.POST("/question/:id", ah.Question)

	// API endpoints for real-time updates
	apigroup := e.Group("/api", ah.authMiddleware)
	apigroup.GET("/events", ah.SSEHandler) // SSE endpoint for real-time updates
	apigroup.GET("/locked-questions", ah.GetLockedQuestionsAPI, ModerateRateLimitMiddleware())
	apigroup.GET("/question-status/:id", ah.GetQuestionStatusAPI, ModerateRateLimitMiddleware())
	
	// Public SSE endpoint for testing (no auth required)
	e.GET("/api/events-test", ah.SSEHandler)
	
	// Health check endpoints (no auth required for monitoring)
	e.GET("/api/health", ah.HealthCheckHandler)
	e.GET("/api/metrics", ah.MetricsHandler, ah.adminMiddleware) // Protected endpoint

	admingroup := e.Group("/su", ah.adminMiddleware)
	admingroup.GET("", ah.AdminPageHandler)
	admingroup.GET("/deleteteam/:id", ah.AdminDeleteTeam)
	admingroup.GET("/deletequestion/:id", ah.AdminDeleteQuestion)
	admingroup.GET("/question", ah.AdminQuestionHandler)
	admingroup.POST("/question", ah.AdminQuestionHandler)

	admingroup.GET("/hints", ah.AdminHintsHandler)
	admingroup.GET("/hints/new", ah.AdminHintNewHandler)
	admingroup.POST("/hints/new", ah.AdminHintNewHandler)

	admingroup.GET("/hints/delete/:id", ah.AdminDeleteHint)
	admingroup.GET("/editquestion/:id", ah.AdminEditQuestionHandler)
	admingroup.POST("/editquestion/:id", ah.AdminEditQuestionHandler)

	admingroup.GET("/editquestion/delimage/:name", ah.AdminDeleteImage)
	admingroup.GET("/editquestion/delvideo/:name", ah.AdminDeleteVideo)
	admingroup.GET("/editquestion/delaudio/:name", ah.AdminDeleteAudio)

	admingroup.GET("/solved-questions", ah.AdminSolvedQuestionsHandler)
	admingroup.GET("/unlock-question/:qid/:tid", ah.AdminUnlockQuestionHandler)
	admingroup.GET("/unlock-question-all/:qid", ah.AdminUnlockAllQuestionHandler)

	e.GET("/*", RouteNotFoundHandler)
}
