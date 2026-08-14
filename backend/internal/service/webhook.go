package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"

	"gorm.io/gorm"
)

var (
	ErrWebhookNotFound        = errors.New("webhook not found")
	ErrWebhookDeliveryNotFound = errors.New("webhook delivery not found")
)

type WebhookService interface {
	List() ([]model.WebhookSub, error)
	Create(name, url, events string, secret string, createdBy *uint64) (*model.WebhookSub, error)
	Delete(id uint64) error
	Dispatch(event string, payload map[string]interface{})
	TestPing(webhookID uint64) (*model.WebhookDelivery, error)
	RetryDelivery(deliveryID uint64) (*model.WebhookDelivery, error)
	ListDeliveries(page, perPage int, filters repository.WebhookDeliveryFilters) ([]model.WebhookDelivery, int64, error)
}

type webhookService struct {
	repo    repository.WebhookRepo
	client  *http.Client
}

func NewWebhookService(repo repository.WebhookRepo) WebhookService {
	return &webhookService{
		repo:   repo,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *webhookService) List() ([]model.WebhookSub, error) {
	return s.repo.ListByStatus(1)
}

func (s *webhookService) Create(name, url, events string, secret string, createdBy *uint64) (*model.WebhookSub, error) {
	// P1-5: reject webhook targets on private/internal hosts. A malicious admin
	// account could otherwise register 127.0.0.1:3306 or cloud metadata URLs.
	if err := validateURL(url); err != nil {
		return nil, errors.New("webhook url not allowed")
	}
	sub := &model.WebhookSub{
		Name:      name,
		URL:       url,
		Events:    events,
		Secret:    secret,
		Status:    1,
		CreatedBy: createdBy,
	}
	if err := s.repo.Create(sub); err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *webhookService) Delete(id uint64) error {
	if _, err := s.repo.FindByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrWebhookNotFound
		}
		return err
	}
	return s.repo.Delete(id)
}

// Dispatch asynchronously delivers an event to all active webhooks subscribed
// to that event. Signature is HMAC-SHA256 of the body using the webhook secret.
func (s *webhookService) ListDeliveries(page, perPage int, filters repository.WebhookDeliveryFilters) ([]model.WebhookDelivery, int64, error) {
	return s.repo.ListDeliveries(page, perPage, filters)
}

func (s *webhookService) Dispatch(event string, payload map[string]interface{}) {
	subs, err := s.repo.ListByStatus(1)
	if err != nil {
		return
	}
	for _, sub := range subs {
		if !subscribed(sub.Events, event) {
			continue
		}
		go safeDeliver(s.deliver, sub, event, payload)
	}
}

// safeDeliver runs a webhook delivery in its own goroutine with panic recovery,
// so a misbehaving endpoint cannot crash the whole process.
func safeDeliver(fn func(model.WebhookSub, string, map[string]interface{}), sub model.WebhookSub, event string, payload map[string]interface{}) {
	defer func() {
		_ = recover() // delivery failures are already recorded as 0-success records
	}()
	fn(sub, event, payload)
}

func (s *webhookService) deliver(sub model.WebhookSub, event string, payload map[string]interface{}) {
	body := struct {
		ID        string                 `json:"id"`
		Event     string                 `json:"event"`
		Timestamp int64                  `json:"timestamp"`
		Data      map[string]interface{} `json:"data"`
	}{
		ID:        fmt.Sprintf("wh_%d_%d", sub.ID, time.Now().UnixNano()),
		Event:     event,
		Timestamp: time.Now().Unix(),
		Data:      payload,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return
	}

	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		delivery := s.deliverOnce(sub, event, string(raw), attempt)
		_ = s.repo.CreateDelivery(delivery)
		if delivery.Success == 1 {
			return
		}
		if attempt < maxAttempts {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
}

// deliverOnce performs a single HTTP delivery attempt for a webhook subscriber,
// returning the populated delivery record (not yet persisted).
func (s *webhookService) deliverOnce(sub model.WebhookSub, event, body string, attempt int) *model.WebhookDelivery {
	delivery := &model.WebhookDelivery{
		WebhookID: sub.ID,
		Event:     event,
		Payload:   body,
		Attempt:   attempt,
	}
	req, err := http.NewRequest(http.MethodPost, sub.URL, bytes.NewReader([]byte(body)))
	if err != nil {
		return delivery
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "dwz-shorturl-webhook/1.0")
	if sub.Secret != "" {
		mac := hmac.New(sha256.New, []byte(sub.Secret))
		mac.Write([]byte(body))
		req.Header.Set("X-Webhook-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return delivery
	}
	defer resp.Body.Close()
	delivery.ResponseStatus = resp.StatusCode
	buf := make([]byte, 512)
	n, _ := resp.Body.Read(buf)
	delivery.ResponseBody = string(buf[:n])
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		delivery.Success = 1
	}
	return delivery
}

// TestPing sends a deterministic ping event to a webhook and returns the
// resulting delivery record, so admins can verify the endpoint and signature.
func (s *webhookService) TestPing(webhookID uint64) (*model.WebhookDelivery, error) {
	sub, err := s.repo.FindByID(webhookID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWebhookNotFound
		}
		return nil, err
	}
	body := struct {
		ID        string                 `json:"id"`
		Event     string                 `json:"event"`
		Timestamp int64                  `json:"timestamp"`
		Data      map[string]interface{} `json:"data"`
	}{
		ID:        fmt.Sprintf("ping_%d", time.Now().UnixNano()),
		Event:     "ping",
		Timestamp: time.Now().Unix(),
		Data:      map[string]interface{}{"message": "Webhook 配置测试", "webhook_id": sub.ID},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	delivery := s.deliverOnce(*sub, "ping", string(raw), 1)
	if err := s.repo.CreateDelivery(delivery); err != nil {
		return nil, err
	}
	return delivery, nil
}

// RetryDelivery re-sends the exact payload of a previously recorded delivery to
// its webhook and returns the new delivery record.
func (s *webhookService) RetryDelivery(deliveryID uint64) (*model.WebhookDelivery, error) {
	old, err := s.repo.FindDeliveryByID(deliveryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWebhookDeliveryNotFound
		}
		return nil, err
	}
	sub, err := s.repo.FindByID(old.WebhookID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWebhookNotFound
		}
		return nil, err
	}
	delivery := s.deliverOnce(*sub, old.Event, old.Payload, 1)
	if err := s.repo.CreateDelivery(delivery); err != nil {
		return nil, err
	}
	return delivery, nil
}

// subscribed reports whether the events JSON contains the given event.
func subscribed(eventsJSON, event string) bool {
	var events []string
	if err := json.Unmarshal([]byte(eventsJSON), &events); err != nil {
		return false
	}
	for _, e := range events {
		if e == event {
			return true
		}
	}
	return false
}