package openhandle

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"
)

type webhookConformanceFixture struct {
	Version                 int                      `json:"version"`
	Secret                  string                   `json:"secret"`
	DefaultToleranceSeconds int                      `json:"defaultToleranceSeconds"`
	Cases                   []webhookConformanceCase `json:"cases"`
}

type webhookConformanceCase struct {
	Name             string            `json:"name"`
	Body             string            `json:"body"`
	Headers          map[string]string `json:"headers"`
	Now              int64             `json:"now"`
	ToleranceSeconds *int              `json:"toleranceSeconds"`
	Event            json.RawMessage   `json:"event"`
	Error            string            `json:"error"`
}

func TestWebhookConformance(t *testing.T) {
	t.Parallel()

	encoded, err := os.ReadFile("testdata/webhook-conformance.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture webhookConformanceFixture
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 1 {
		t.Fatalf("fixture version = %d, want 1", fixture.Version)
	}
	if fixture.DefaultToleranceSeconds != 300 {
		t.Fatalf("fixture default tolerance = %d, want 300", fixture.DefaultToleranceSeconds)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}

	for _, test := range fixture.Cases {
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()

			header := http.Header{}
			for name, value := range test.Headers {
				header.Set(name, value)
			}
			options := &VerifyWebhookOptions{Now: time.Unix(test.Now, 0)}
			if test.ToleranceSeconds != nil {
				options.Tolerance = time.Duration(*test.ToleranceSeconds) * time.Second
			}

			event, err := VerifyWebhook([]byte(test.Body), header, fixture.Secret, options)

			if test.Error != "" {
				var verificationError *WebhookVerificationError
				if !errors.As(err, &verificationError) {
					t.Fatalf("error = %T %v, want *WebhookVerificationError %q", err, err, test.Error)
				}
				if string(verificationError.Code) != test.Error {
					t.Fatalf("code = %q, want %q", verificationError.Code, test.Error)
				}
				if event != nil {
					t.Fatalf("event = %+v, want nil", event)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var expected WebhookEvent
			if err := json.Unmarshal(test.Event, &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*event, expected) {
				t.Fatalf("event = %+v, want %+v", *event, expected)
			}
		})
	}
}
