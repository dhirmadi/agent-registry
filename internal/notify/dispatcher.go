package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Event represents a webhook event to dispatch.
type Event struct {
	Type         string `json:"event"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Timestamp    string `json:"timestamp"`
	Actor        string `json:"actor"`
}

// Subscription holds webhook subscription data for delivery.
type Subscription struct {
	ID     uuid.UUID
	URL    string
	Secret string
	Events []string
}

// SubscriptionLoader loads active webhook subscriptions.
type SubscriptionLoader interface {
	ListActive(ctx context.Context) ([]Subscription, error)
}

// EventDispatcher is the interface handlers use to dispatch events.
type EventDispatcher interface {
	Dispatch(event Event)
}

// Config holds dispatcher configuration.
type Config struct {
	Workers    int
	MaxRetries int
	Timeout    time.Duration
}

// Dispatcher manages async webhook delivery via a worker pool.
type Dispatcher struct {
	loader     SubscriptionLoader
	client     *http.Client
	eventCh    chan Event
	workers    int
	maxRetries int
	wg         sync.WaitGroup
	stopped    atomic.Bool
}

// NewDispatcher creates a dispatcher with a buffered channel (size 1000).
func NewDispatcher(loader SubscriptionLoader, cfg Config) *Dispatcher {
	return &Dispatcher{
		loader:     loader,
		client:     &http.Client{Timeout: cfg.Timeout},
		eventCh:    make(chan Event, 1000),
		workers:    cfg.Workers,
		maxRetries: cfg.MaxRetries,
	}
}

// Start launches worker goroutines that consume from the event channel.
func (d *Dispatcher) Start() {
	for i := 0; i < d.workers; i++ {
		d.wg.Add(1)
		go d.worker()
	}
}

// Stop signals workers to drain remaining events and exit.
func (d *Dispatcher) Stop() {
	d.stopped.Store(true)
	close(d.eventCh)
	d.wg.Wait()
}

// Dispatch performs a non-blocking send to the event channel.
func (d *Dispatcher) Dispatch(event Event) {
	if d.stopped.Load() {
		return
	}
	select {
	case d.eventCh <- event:
	default:
		log.Printf("webhook event channel full, dropping event: %s", event.Type)
	}
}

func (d *Dispatcher) worker() {
	defer d.wg.Done()
	for event := range d.eventCh {
		d.processEvent(event)
	}
}

func (d *Dispatcher) processEvent(event Event) {
	subs, err := d.loader.ListActive(context.Background())
	if err != nil {
		log.Printf("failed to load webhook subscriptions: %v", err)
		return
	}

	for _, sub := range subs {
		if matchesEvent(sub.Events, event.Type) {
			d.deliver(sub, event)
		}
	}
}

func matchesEvent(events []string, eventType string) bool {
	for _, e := range events {
		if e == eventType {
			return true
		}
	}
	return false
}

func (d *Dispatcher) deliver(sub Subscription, event Event) {
	body, err := json.Marshal(event)
	if err != nil {
		log.Printf("failed to marshal webhook event: %v", err)
		return
	}
	deliveryID := uuid.New().String()

	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			time.Sleep(backoff)
		}

		req, err := http.NewRequest("POST", sub.URL, bytes.NewReader(body))
		if err != nil {
			log.Printf("failed to create webhook request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-Event", event.Type)
		req.Header.Set("X-Registry-Delivery", deliveryID)

		if sub.Secret != "" {
			sig := computeHMAC(sub.Secret, body)
			req.Header.Set("X-Webhook-Signature", "sha256="+sig)
		}

		resp, err := d.client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		log.Printf("webhook delivery %s attempt %d failed: url=%s event=%s err=%v",
			deliveryID, attempt+1, sub.URL, event.Type, err)
	}
	log.Printf("webhook delivery %s exhausted all retries: url=%s event=%s",
		deliveryID, sub.URL, event.Type)
}

func computeHMAC(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
