package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/namishh/holmes/services"
)

// SSEHandler handles Server-Sent Events for real-time updates
func (ah *AuthHandler) SSEHandler(c echo.Context) error {
	// Set CORS headers
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Response().Header().Set("Access-Control-Allow-Headers", "Content-Type")
	
	// Set headers for SSE
	c.Response().Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	c.Response().Header().Set("Cache-Control", "no-cache, no-transform")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
	
	// Write the headers immediately
	c.Response().WriteHeader(http.StatusOK)
	c.Response().Flush()

	// Create a unique client ID
	clientID := uuid.New().String()
	
	// Register the client with the broadcaster
	client := ah.Broadcaster.RegisterClient(clientID)
	defer ah.Broadcaster.UnregisterClient(client)

	// Send initial connection event
	initialEvent := services.Event{
		Type: "connected",
		Data: map[string]interface{}{
			"client_id": clientID,
			"message":   "Connected to real-time updates",
		},
		Timestamp: time.Now(),
	}
	
	sseData := services.FormatSSE(initialEvent)
	if _, err := c.Response().Write([]byte(sseData)); err != nil {
		return err
	}
	c.Response().Flush()

	// Send current state immediately
	locks, err := ah.UserServices.GetAllLockedQuestions()
	if err == nil {
		stateEvent := services.Event{
			Type: services.EventQuestionLocked,
			Data: map[string]interface{}{
				"locks": locks,
			},
			Timestamp: time.Now(),
		}
		sseData := services.FormatSSE(stateEvent)
		if _, err := c.Response().Write([]byte(sseData)); err != nil {
			return err
		}
		c.Response().Flush()
	}

	// Listen for events or client disconnect
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event := <-client.Channel:
			// Send event to client
			sseData := services.FormatSSE(event)
			if _, err := c.Response().Write([]byte(sseData)); err != nil {
				return err
			}
			c.Response().Flush()

		case <-ticker.C:
			// Send heartbeat to keep connection alive
			heartbeat := services.Event{
				Type: "heartbeat",
				Data: map[string]interface{}{
					"timestamp": time.Now().Unix(),
				},
				Timestamp: time.Now(),
			}
			sseData := services.FormatSSE(heartbeat)
			if _, err := c.Response().Write([]byte(sseData)); err != nil {
				return err
			}
			c.Response().Flush()

		case <-c.Request().Context().Done():
			// Client disconnected
			return nil
		}
	}
}

// generateETag creates a hash for the given data
func generateETag(data interface{}) string {
	jsonData, _ := json.Marshal(data)
	hash := sha256.Sum256(jsonData)
	return hex.EncodeToString(hash[:])
}

// GetLockedQuestions returns JSON of all currently locked questions
// Now includes ETag support for conditional GET requests
func (ah *AuthHandler) GetLockedQuestionsAPI(c echo.Context) error {
	locks, err := ah.UserServices.GetAllLockedQuestions()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch locked questions",
		})
	}

	// Generate ETag for the current state
	etag := generateETag(locks)
	
	// Check if client has the same ETag
	clientETag := c.Request().Header.Get("If-None-Match")
	if clientETag == etag {
		return c.NoContent(http.StatusNotModified)
	}
	
	// Set caching headers
	c.Response().Header().Set("ETag", etag)
	c.Response().Header().Set("Cache-Control", "public, max-age=5") // 5 second cache
	c.Response().Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))

	return c.JSON(http.StatusOK, locks)
}

// GetQuestionStatus returns JSON of a specific question's status (locked/unlocked)
func (ah *AuthHandler) GetQuestionStatusAPI(c echo.Context) error {
	questionID := c.Param("id")
	
	var id int
	if _, err := fmt.Sscanf(questionID, "%d", &id); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid question ID",
		})
	}

	isLocked, lockInfo, err := ah.UserServices.IsQuestionLocked(id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to check question status",
		})
	}

	if isLocked {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"locked":         true,
			"locked_by_team": lockInfo.LockedByTeamID,
			"locked_by_name": lockInfo.LockedByName,
			"locked_at":      lockInfo.LockedAt,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"locked": false,
	})
}
