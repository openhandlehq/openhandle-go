package openhandle

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

const (
	webhookSignatureHeader  = "X-Openhandle-Signature"
	webhookTimestampHeader  = "X-Openhandle-Timestamp"
	webhookSignaturePrefix  = "sha256="
	defaultWebhookTolerance = 300 * time.Second
)

// WebhookVerificationCode identifies why a webhook failed verification.
type WebhookVerificationCode string

const (
	WebhookMissingSignature        WebhookVerificationCode = "missing_signature"
	WebhookMissingTimestamp        WebhookVerificationCode = "missing_timestamp"
	WebhookInvalidTimestamp        WebhookVerificationCode = "invalid_timestamp"
	WebhookTimestampOutOfTolerance WebhookVerificationCode = "timestamp_out_of_tolerance"
	WebhookInvalidSignature        WebhookVerificationCode = "invalid_signature"
	WebhookInvalidPayload          WebhookVerificationCode = "invalid_payload"
)

// WebhookVerificationError reports a webhook that failed verification.
// Callers should branch on Code.
type WebhookVerificationError struct {
	Code  WebhookVerificationCode
	Cause error
}

func (e *WebhookVerificationError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return "openhandle: webhook verification failed: " + string(e.Code)
}

func (e *WebhookVerificationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// VerifyWebhookOptions configures VerifyWebhook. Zero values use a 300 second
// tolerance and the current time.
type VerifyWebhookOptions struct {
	Tolerance time.Duration
	Now       time.Time
}

// VerifyWebhook checks the signature and timestamp of a pushed webhook and
// returns the decoded event. Pass the raw request body exactly as received and
// the endpoint's signing secret. It performs no I/O and needs no API key.
func VerifyWebhook(body []byte, header http.Header, secret string, options *VerifyWebhookOptions) (*WebhookEvent, error) {
	tolerance := defaultWebhookTolerance
	now := time.Now()
	if options != nil && options.Tolerance > 0 {
		tolerance = options.Tolerance
	}
	if options != nil && !options.Now.IsZero() {
		now = options.Now
	}

	signature := header.Get(webhookSignatureHeader)
	if signature == "" {
		return nil, &WebhookVerificationError{Code: WebhookMissingSignature}
	}
	timestamp := header.Get(webhookTimestampHeader)
	if timestamp == "" {
		return nil, &WebhookVerificationError{Code: WebhookMissingTimestamp}
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || timestamp[0] < '0' || timestamp[0] > '9' {
		return nil, &WebhookVerificationError{Code: WebhookInvalidTimestamp, Cause: err}
	}
	age := time.Unix(now.Unix(), 0).Sub(time.Unix(seconds, 0))
	if age > tolerance || age < -tolerance {
		return nil, &WebhookVerificationError{Code: WebhookTimestampOutOfTolerance}
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	expected := webhookSignaturePrefix + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return nil, &WebhookVerificationError{Code: WebhookInvalidSignature}
	}

	if !bytes.HasPrefix(bytes.TrimLeft(body, " \t\r\n"), []byte("{")) {
		return nil, &WebhookVerificationError{Code: WebhookInvalidPayload}
	}
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, &WebhookVerificationError{Code: WebhookInvalidPayload, Cause: err}
	}
	if event.ID == "" || event.Type == "" || event.Environment == "" || event.CreatedAt.IsZero() || event.Data.Platform == "" {
		return nil, &WebhookVerificationError{Code: WebhookInvalidPayload}
	}
	return &event, nil
}
