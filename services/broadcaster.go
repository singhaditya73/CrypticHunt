package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// EventType represents different types of events that can be broadcast
type EventType string

const (
	EventQuestionLocked   EventType = "question_locked"
	EventQuestionUnlocked EventType = "question_unlocked"
	EventQuestionSolved   EventType = "question_solved"
	EventLeaderboardUpdate EventType = "leaderboard_update"
)

// Event represents a broadcast event
type Event struct {
	Type      EventType              `json:"type"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// Client represents an SSE client connection
type Client struct {
	ID         string
	Channel    chan Event
	Disconnect chan bool
}

// Broadcaster manages SSE connections and Redis pub/sub
type Broadcaster struct {
	clients      map[string]*Client
	clientsMutex sync.RWMutex
	
	redisClient  *redis.Client
	ctx          context.Context
	
	// Channels for internal communication
	register     chan *Client
	unregister   chan *Client
	broadcast    chan Event
}

// NewBroadcaster creates a new broadcaster instance
func NewBroadcaster(redisAddr string, redisPassword string, redisDB int) *Broadcaster {
	ctx := context.Background()
	
	var redisClient *redis.Client
	if redisAddr != "" {
		// Initialize Redis client
		redisClient = redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       redisDB,
		})
		
		// Test connection
		if err := redisClient.Ping(ctx).Err(); err != nil {
			log.Printf("Warning: Redis connection failed: %v. Running in standalone mode.", err)
			redisClient = nil
		} else {
			log.Println("Successfully connected to Redis for pub/sub")
		}
	}
	
	b := &Broadcaster{
		clients:      make(map[string]*Client),
		redisClient:  redisClient,
		ctx:          ctx,
		register:     make(chan *Client, 100),
		unregister:   make(chan *Client, 100),
		broadcast:    make(chan Event, 1000),
	}
	
	// Start the broadcast loop
	go b.run()
	
	// Start Redis subscriber if available
	if b.redisClient != nil {
		go b.subscribeToRedis()
	}
	
	return b
}

// run is the main event loop for the broadcaster
func (b *Broadcaster) run() {
	for {
		select {
		case client := <-b.register:
			b.clientsMutex.Lock()
			b.clients[client.ID] = client
			b.clientsMutex.Unlock()
			log.Printf("Client registered: %s. Total clients: %d", client.ID, len(b.clients))
			
		case client := <-b.unregister:
			b.clientsMutex.Lock()
			if _, ok := b.clients[client.ID]; ok {
				delete(b.clients, client.ID)
				close(client.Channel)
				log.Printf("Client unregistered: %s. Total clients: %d", client.ID, len(b.clients))
			}
			b.clientsMutex.Unlock()
			
		case event := <-b.broadcast:
			// Publish to Redis for multi-instance support
			if b.redisClient != nil {
				go b.publishToRedis(event)
			}
			
			// Broadcast to all connected clients
			b.broadcastToClients(event)
		}
	}
}

// subscribeToRedis listens for events from Redis pub/sub
func (b *Broadcaster) subscribeToRedis() {
	pubsub := b.redisClient.Subscribe(b.ctx, "hunt_events")
	defer pubsub.Close()
	
	ch := pubsub.Channel()
	
	for msg := range ch {
		var event Event
		if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
			log.Printf("Error unmarshaling Redis message: %v", err)
			continue
		}
		
		// Broadcast to local clients (don't re-publish to Redis)
		b.broadcastToClients(event)
	}
}

// publishToRedis publishes an event to Redis
func (b *Broadcaster) publishToRedis(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Error marshaling event for Redis: %v", err)
		return
	}
	
	if err := b.redisClient.Publish(b.ctx, "hunt_events", data).Err(); err != nil {
		log.Printf("Error publishing to Redis: %v", err)
	}
}

// broadcastToClients sends an event to all connected SSE clients
func (b *Broadcaster) broadcastToClients(event Event) {
	b.clientsMutex.RLock()
	defer b.clientsMutex.RUnlock()
	
	for _, client := range b.clients {
		select {
		case client.Channel <- event:
			// Successfully sent
		case <-time.After(100 * time.Millisecond):
			// Timeout - client might be slow or disconnected
			log.Printf("Timeout sending to client %s", client.ID)
		}
	}
}

// RegisterClient adds a new SSE client
func (b *Broadcaster) RegisterClient(clientID string) *Client {
	client := &Client{
		ID:         clientID,
		Channel:    make(chan Event, 100),
		Disconnect: make(chan bool),
	}
	
	b.register <- client
	return client
}

// UnregisterClient removes an SSE client
func (b *Broadcaster) UnregisterClient(client *Client) {
	b.unregister <- client
}

// Broadcast sends an event to all clients
func (b *Broadcaster) Broadcast(eventType EventType, data map[string]interface{}) {
	event := Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now(),
	}
	
	select {
	case b.broadcast <- event:
		// Successfully queued
	case <-time.After(100 * time.Millisecond):
		log.Printf("Warning: Broadcast channel full, dropping event: %s", eventType)
	}
}

// GetClientCount returns the number of connected clients
func (b *Broadcaster) GetClientCount() int {
	b.clientsMutex.RLock()
	defer b.clientsMutex.RUnlock()
	return len(b.clients)
}

// FormatSSE formats an event as SSE message
func FormatSSE(event Event) string {
	data, _ := json.Marshal(event)
	return fmt.Sprintf("data: %s\n\n", data)
}
