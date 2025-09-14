package lpoll

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type Event struct {
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

// ClientState holds the channel and a timestamp for a specific client.
type ClientState struct {
	Channel  chan Event
	LastSeen time.Time
}

var (
	// clientChannels maps a client ID to its state.
	clientChannels map[string]*ClientState
	// mutex for safe concurrent access to the clientChannels map.
	mu sync.RWMutex
	// Define the timeout duration for client inactivity.
	clientTimeout = 1 * time.Minute
)

func init() {
	clientChannels = make(map[string]*ClientState)
}

func PollHandler(c *gin.Context) {
	clientId := c.Param("clientId")
	if clientId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "clientId is required"})
		return
	}

	mu.Lock()
	client, ok := clientChannels[clientId]
	if !ok {
		clientChan := make(chan Event, 1)
		client = &ClientState{
			Channel:  clientChan,
			LastSeen: time.Now(),
		}
		clientChannels[clientId] = client
		log.Printf("Client subscribed: %s", clientId)
	} else {
		client.LastSeen = time.Now()
	}
	mu.Unlock()

	timeout := time.After(30 * time.Second)

	select {
	case event := <-client.Channel:
		c.JSON(http.StatusOK, event)
		return
	case <-timeout:
		c.JSON(http.StatusNoContent, nil)
		log.Printf("Poll timeout for client: %s", clientId)
		return
	}
}

func PublishHandler(c *gin.Context) {
	clientId := c.Param("clientId")
	if clientId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "clientId is required"})
		return
	}

	var req struct {
		Message string `json:"message" binding:"required"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	mu.RLock()
	client, ok := clientChannels[clientId]
	mu.RUnlock()

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Client not found"})
		return
	}

	select {
	case client.Channel <- Event{Message: req.Message, Time: time.Now()}:
		c.JSON(http.StatusOK, gin.H{"message": "Event published."})
	default:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Client channel is full, skipping event."})
	}
}

func CleanUpInactiveClients() {
	for {
		time.Sleep(1 * time.Minute) // Check for inactive clients every minute.

		mu.Lock()
		for clientId, clientState := range clientChannels {
			// Check if the client's last seen time is older than the timeout.
			if time.Since(clientState.LastSeen) > clientTimeout {
				delete(clientChannels, clientId)
				log.Printf("Cleaned up inactive client: %s", clientId)
				log.Printf("Active clients remaining: %d", len(clientChannels))
				// Close the channel to release resources.
				close(clientState.Channel)
			}
		}
		mu.Unlock()
	}
}
