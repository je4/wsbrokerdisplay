package rest

import (
	"sync"
	"time"

	"github.com/je4/wsbrokerclient/pkg/event"
)

// MessageLog stores a thread-safe rolling buffer of recent WebSocket messages.
type MessageLog struct {
	mu       sync.RWMutex
	capacity int
	entries  []WSMessage
}

// NewMessageLog creates a new MessageLog with the given maximum message capacity.
func NewMessageLog(capacity int) *MessageLog {
	if capacity <= 0 {
		capacity = 100
	}
	return &MessageLog{
		capacity: capacity,
		entries:  make([]WSMessage, 0, capacity),
	}
}

// Capacity returns the maximum number of entries held in the log.
func (ml *MessageLog) Capacity() int {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	return ml.capacity
}

// SetCapacity updates the maximum capacity and trims entries if necessary.
func (ml *MessageLog) SetCapacity(capacity int) {
	if capacity <= 0 {
		capacity = 100
	}
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.capacity = capacity
	if len(ml.entries) > ml.capacity {
		ml.entries = ml.entries[len(ml.entries)-ml.capacity:]
	}
}

// Add appends a WSMessage entry to the log, dropping the oldest if capacity is reached.
func (ml *MessageLog) Add(msg WSMessage) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	ml.mu.Lock()
	defer ml.mu.Unlock()
	if len(ml.entries) >= ml.capacity {
		ml.entries = ml.entries[len(ml.entries)-ml.capacity+1:]
	}
	ml.entries = append(ml.entries, msg)
}

// AddEvent parses and adds an event.Event to the log with the specified direction ("in" or "out").
func (ml *MessageLog) AddEvent(direction string, evt *event.Event) {
	if evt == nil {
		return
	}
	dataStr := ""
	if str, err := evt.GetStringData(); err == nil && str != "" {
		dataStr = str
	} else if len(evt.Data) > 0 {
		dataStr = string(evt.Data)
	}
	ml.Add(WSMessage{
		Timestamp: time.Now(),
		Direction: direction,
		Type:      string(evt.Type),
		Source:    evt.Source,
		Target:    evt.Target,
		Token:     evt.Token,
		Data:      dataStr,
	})
}

// GetAll returns a copy of all logged messages in reverse chronological order (newest first).
func (ml *MessageLog) GetAll() []WSMessage {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	n := len(ml.entries)
	result := make([]WSMessage, n)
	for i, entry := range ml.entries {
		result[n-1-i] = entry
	}
	return result
}

// GetAllChronological returns a copy of all logged messages in chronological order (oldest first).
func (ml *MessageLog) GetAllChronological() []WSMessage {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	result := make([]WSMessage, len(ml.entries))
	copy(result, ml.entries)
	return result
}

// Clear removes all stored messages from the log.
func (ml *MessageLog) Clear() {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.entries = ml.entries[:0]
}

// Len returns the current number of logged messages.
func (ml *MessageLog) Len() int {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	return len(ml.entries)
}
