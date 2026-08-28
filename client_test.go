package openhandle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewValidatesConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := New(" "); err == nil {
		t.Fatal("New accepted an empty API key")
	}
	if _, err := New("oh_test_key", WithBaseURL("relative")); err == nil {
		t.Fatal("New accepted a relative base URL")
	}
	if _, err := New("oh_test_key", WithMaxRetries(-1)); err == nil {
		t.Fatal("New accepted negative retries")
	}
}

func TestInstagramProfileGet(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/instagram/profiles/@northstar_forge_test" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if got := request.URL.Query().Get("freshness"); got != "24h" {
			t.Errorf("freshness = %q", got)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer oh_test_key" {
			t.Errorf("authorization = %q", got)
		}
		if got := request.Header.Get("X-OpenHandle-Client"); got != "openhandle-go/0.0.0" {
			t.Errorf("client header = %q", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-ID", "request_123")
		writer.Header().Set("Openhandle-Cost", "0.000")
		writer.Header().Set("Openhandle-Environment", "test")
		_, _ = fmt.Fprint(writer, `{"platform":"instagram","resource":"profile","captured_at":"2026-08-27T12:00:00Z","source":"cache","data":{"id":"profile_1","handle":"northstar_forge_test","display_name":"Northstar","bio":"","is_business":false,"is_private":false,"is_verified":false,"metrics":{}}}`)
	}))
	defer server.Close()

	client, err := New("oh_test_key", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Instagram.Profile("northstar_forge_test").Get(t.Context(), &InstagramProfileOptions{Freshness: FreshnessTwentyFourH})
	if err != nil {
		t.Fatal(err)
	}
	if response.Data.Handle != "northstar_forge_test" || response.RequestID != "request_123" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if response.Billing.Cost != "0.000" || response.Billing.Environment != "test" {
		t.Fatalf("unexpected billing metadata: %#v", response.Billing)
	}
}

func TestReferenceFailureMakesNoRequest(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client, err := New("oh_test_key", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Instagram.Profile("https://x.com/openai").Get(t.Context(), nil)
	var mismatch *ReferenceMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("error = %T %v, want ReferenceMismatchError", err, err)
	}
	if requests.Load() != 0 {
		t.Fatalf("made %d requests", requests.Load())
	}
}

func TestRetriesRetryableAPIError(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		attempt := requests.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		if attempt == 1 {
			writer.Header().Set("Retry-After", "0.001")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(writer, `{"error":{"code":"UPSTREAM_UNAVAILABLE","message":"try again","request_id":"request_retry","retryable":true}}`)
			return
		}
		_, _ = fmt.Fprint(writer, `{"platform":"instagram","resource":"profile","captured_at":"2026-08-27T12:00:00Z","source":"live","data":{"id":"profile_1","handle":"openai","display_name":"OpenAI","bio":"","is_business":false,"is_private":false,"is_verified":true,"metrics":{}}}`)
	}))
	defer server.Close()
	client, err := New("oh_live_key", WithBaseURL(server.URL), WithMaxRetries(1))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.Instagram.Profile("https://www.instagram.com/openai/").Get(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

func TestAPIErrorFields(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-ID", "request_header")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(writer, `{"error":{"code":"INVALID_REFERENCE","message":"invalid","request_id":"request_body","retryable":false,"details":{"field":"identifier"}}}`)
	}))
	defer server.Close()
	client, err := New("oh_test_key", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Instagram.Profile("openai").Get(t.Context(), nil)
	var apiError *Error
	if !errors.As(err, &apiError) {
		t.Fatalf("error = %T %v, want *Error", err, err)
	}
	if apiError.Code != "INVALID_REFERENCE" || apiError.RequestID != "request_body" || apiError.Status != 400 || apiError.Retryable {
		t.Fatalf("unexpected API error: %#v", apiError)
	}
}

func TestPageNextUsesOpaqueCursorAndStopsAtEnd(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attempt := requests.Add(1)
		cursor := request.URL.Query().Get("cursor")
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-ID", fmt.Sprintf("request_%d", attempt))
		if attempt == 1 {
			if cursor != "" {
				t.Errorf("first cursor = %q", cursor)
			}
			_, _ = fmt.Fprint(writer, pageJSON("post_1", "opaque+cursor/1", "request_1"))
			return
		}
		if cursor != "opaque+cursor/1" {
			t.Errorf("next cursor = %q", cursor)
		}
		_, _ = fmt.Fprint(writer, pageJSON("post_2", "", "request_2"))
	}))
	defer server.Close()
	client, err := New("oh_test_key", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}

	page, err := client.Instagram.Profile("northstar_forge_test").Posts.List(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasNextPage() || page.NextCursor() != "opaque+cursor/1" {
		t.Fatalf("unexpected first page cursor: %#v", page.Meta.Cursors)
	}
	next, err := page.Next(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || len(next.Data) != 1 || next.Data[0].ID != "post_2" {
		t.Fatalf("unexpected next page: %#v", next)
	}
	last, err := next.Next(t.Context())
	if err != nil || last != nil {
		t.Fatalf("last Next() = %#v, %v", last, err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

func TestIteratorIsLazy(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, pageJSON("post_1", "", "request_1"))
	}))
	defer server.Close()
	client, err := New("oh_test_key", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	iterator := client.Instagram.Profile("northstar_forge_test").Posts.Items(nil)
	if requests.Load() != 0 {
		t.Fatal("iterator requested a page before Next")
	}
	if !iterator.Next(t.Context()) || iterator.Value().ID != "post_1" {
		t.Fatalf("iterator failed: %v", iterator.Err())
	}
	if iterator.Next(t.Context()) {
		t.Fatal("iterator returned an unexpected second item")
	}
	if iterator.Err() != nil || requests.Load() != 1 {
		t.Fatalf("iterator error = %v, requests = %d", iterator.Err(), requests.Load())
	}
}

func TestPerRequestTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
			return
		case <-time.After(time.Second):
			writer.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	client, err := New("oh_test_key", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Instagram.Profile("openai").Get(context.Background(), &InstagramProfileOptions{
		RequestOptions: RequestOptions{Timeout: time.Millisecond},
	})
	var apiError *Error
	if !errors.As(err, &apiError) || apiError.Code != "TRANSPORT_ERROR" {
		t.Fatalf("error = %T %v, want transport Error", err, err)
	}
}

func TestDefaultTimeoutAppliesWithCustomHTTPClient(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
			return
		case <-time.After(time.Second):
			writer.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	client, err := New(
		"oh_test_key",
		WithBaseURL(server.URL),
		WithHTTPClient(&http.Client{}),
		WithTimeout(time.Millisecond),
		WithMaxRetries(0),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Instagram.Profile("openai").Get(context.Background(), nil)
	var apiError *Error
	if !errors.As(err, &apiError) || apiError.Code != "TRANSPORT_ERROR" {
		t.Fatalf("error = %T %v, want transport Error", err, err)
	}
}

func TestFetchDecodesConcreteVariant(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["url"] != "https://www.instagram.com/openai/" || body["freshness"] != "24h" {
			t.Errorf("body = %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"platform":"instagram","resource":"profile","captured_at":"2026-08-27T12:00:00Z","source":"cache","data":{"id":"profile_1","handle":"openai","display_name":"OpenAI","bio":"","is_business":false,"is_private":false,"is_verified":true,"metrics":{}}}`)
	}))
	defer server.Close()
	client, err := New("oh_test_key", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Fetch(t.Context(), "https://www.instagram.com/openai/", &FetchOptions{Freshness: FreshnessTwentyFourH})
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := response.Data.(*InstagramProfile)
	if !ok || profile.Handle != "openai" {
		t.Fatalf("fetch data = %T %#v", response.Data, response.Data)
	}
}

func pageJSON(id, cursor, requestID string) string {
	next := "null"
	if cursor != "" {
		next = fmt.Sprintf("%q", cursor)
	}
	return fmt.Sprintf(`{"platform":"instagram","resource":"post","captured_at":"2026-08-27T12:00:00Z","source":"cache","data":[{"id":%q}],"meta":{"cursors":{"next":%s}},"request_id":%q}`, id, next, requestID)
}
