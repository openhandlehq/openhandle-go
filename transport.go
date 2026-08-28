package openhandle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 32 << 20

var pathParameterPattern = regexp.MustCompile(`\{([^}]+)\}`)

type errorEnvelope struct {
	Error struct {
		Code      string         `json:"code"`
		Details   map[string]any `json:"details"`
		Message   string         `json:"message"`
		RequestID string         `json:"request_id"`
		Retryable bool           `json:"retryable"`
	} `json:"error"`
}

func (c *clientCore) do(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	body any,
	controls RequestOptions,
	result any,
) error {
	if ctx == nil {
		return errors.New("openhandle: context must not be nil")
	}
	var encodedBody []byte
	var err error
	if body != nil {
		encodedBody, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("openhandle: encode request body: %w", err)
		}
	}

	maxRetries := c.maxRetries
	if controls.MaxRetries != nil {
		if *controls.MaxRetries < 0 {
			return errors.New("openhandle: max retries must not be negative")
		}
		maxRetries = *controls.MaxRetries
	}

	for attempt := 0; ; attempt++ {
		requestTimeout := controls.Timeout
		if requestTimeout <= 0 {
			requestTimeout = c.timeout
		}
		err = c.doAttempt(ctx, method, path, query, encodedBody, requestTimeout, result)
		if err == nil {
			return nil
		}
		if attempt >= maxRetries || ctx.Err() != nil || !isRetryable(err) {
			return err
		}
		delay := retryDelay(err, attempt)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *clientCore) doAttempt(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	body []byte,
	timeout time.Duration,
	result any,
) error {
	requestContext := ctx
	cancel := func() {}
	if timeout > 0 {
		requestContext, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	target, err := url.Parse(strings.TrimRight(c.baseURL.String(), "/") + path)
	if err != nil {
		return fmt.Errorf("openhandle: build request URL: %w", err)
	}
	target.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(requestContext, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("openhandle: create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("X-OpenHandle-Client", "openhandle-go/"+sdkVersion())
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return &Error{Code: "TRANSPORT_ERROR", Message: "Openhandle request failed.", Retryable: requestContext.Err() == nil, Cause: err}
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return &Error{Code: "INVALID_RESPONSE", Message: "Openhandle response could not be read.", RequestID: response.Header.Get("X-Request-ID"), Status: response.StatusCode, Retryable: true, Cause: err}
	}
	if len(payload) > maxResponseBytes {
		return &Error{Code: "INVALID_RESPONSE", Message: "Openhandle response exceeded the maximum supported size.", RequestID: response.Header.Get("X-Request-ID"), Status: response.StatusCode}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeAPIError(response, payload)
	}
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	if err := json.Unmarshal(payload, result); err != nil {
		return &Error{Code: "INVALID_RESPONSE", Message: "Openhandle returned invalid JSON.", RequestID: response.Header.Get("X-Request-ID"), Status: response.StatusCode, Cause: err}
	}
	if setter, ok := result.(responseMetadataSetter); ok {
		setter.setResponseMetadata(response.Header.Get("X-Request-ID"), billingFromHeaders(response.Header))
	}
	return nil
}

func decodeAPIError(response *http.Response, payload []byte) error {
	var envelope errorEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return &Error{
			Code:       fmt.Sprintf("HTTP_%d", response.StatusCode),
			Message:    fmt.Sprintf("Openhandle request failed with status %d.", response.StatusCode),
			RequestID:  response.Header.Get("X-Request-ID"),
			Retryable:  response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500,
			RetryAfter: parseRetryAfter(response.Header.Get("Retry-After")),
			Status:     response.StatusCode,
			Cause:      err,
		}
	}
	requestID := envelope.Error.RequestID
	if requestID == "" {
		requestID = response.Header.Get("X-Request-ID")
	}
	code := envelope.Error.Code
	if code == "" {
		code = fmt.Sprintf("HTTP_%d", response.StatusCode)
	}
	message := envelope.Error.Message
	if message == "" {
		message = fmt.Sprintf("Openhandle request failed with status %d.", response.StatusCode)
	}
	return &Error{
		Code:       code,
		Details:    envelope.Error.Details,
		Message:    message,
		RequestID:  requestID,
		Retryable:  envelope.Error.Retryable || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500,
		RetryAfter: parseRetryAfter(response.Header.Get("Retry-After")),
		Status:     response.StatusCode,
	}
}

func isRetryable(err error) bool {
	var apiError *Error
	return errors.As(err, &apiError) && apiError.Retryable
}

func retryDelay(err error, attempt int) time.Duration {
	var apiError *Error
	if errors.As(err, &apiError) && apiError.RetryAfter > 0 {
		return apiError.RetryAfter
	}
	base := min(4*time.Second, 250*time.Millisecond*time.Duration(1<<min(attempt, 4)))
	return time.Duration(float64(base) * (0.75 + rand.Float64()*0.5))
}

func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		return max(0, time.Duration(seconds*float64(time.Second)))
	}
	if target, err := http.ParseTime(value); err == nil {
		return max(0, time.Until(target))
	}
	return 0
}

func billingFromHeaders(headers http.Header) Billing {
	return Billing{
		Cost:           headers.Get("Openhandle-Cost"),
		DatasetVersion: headers.Get("Openhandle-Dataset-Version"),
		Disposition:    headers.Get("Openhandle-Billing-Disposition"),
		Environment:    headers.Get("Openhandle-Environment"),
		ListPrice:      headers.Get("Openhandle-List-Price"),
	}
}

func encodeOptions(options any) (url.Values, RequestOptions, error) {
	query := url.Values{}
	if options == nil {
		return query, RequestOptions{}, nil
	}
	value := reflect.ValueOf(options)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return query, RequestOptions{}, nil
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil, RequestOptions{}, errors.New("openhandle: operation options must be a struct or pointer to a struct")
	}
	controls := RequestOptions{}
	if field := value.FieldByName("RequestOptions"); field.IsValid() && field.CanInterface() {
		controls, _ = field.Interface().(RequestOptions)
	}
	valueType := value.Type()
	for index := 0; index < value.NumField(); index++ {
		fieldType := valueType.Field(index)
		name := fieldType.Tag.Get("query")
		if name == "" || name == "-" {
			continue
		}
		field := value.Field(index)
		if fieldType.Tag.Get("required") == "true" && isZero(field) {
			return nil, controls, fmt.Errorf("openhandle: option %s is required", fieldType.Name)
		}
		if encoded, ok := encodeQueryValue(field); ok {
			query.Set(name, encoded)
		}
	}
	return query, controls, nil
}

func encodeQueryValue(value reflect.Value) (string, bool) {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "", false
		}
		value = value.Elem()
	}
	if value.IsZero() {
		return "", false
	}
	if value.CanInterface() {
		if timestamp, ok := value.Interface().(time.Time); ok {
			return timestamp.Format(time.RFC3339), true
		}
	}
	switch value.Kind() {
	case reflect.String:
		return value.String(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(value.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(value.Uint(), 10), true
	case reflect.Bool:
		return strconv.FormatBool(value.Bool()), true
	default:
		return fmt.Sprint(value.Interface()), true
	}
}

func isZero(value reflect.Value) bool {
	if value.Kind() == reflect.Pointer {
		return value.IsNil()
	}
	return value.IsZero()
}

func bindPath(path string, bindings map[string]string, referenceErr error) (string, error) {
	if referenceErr != nil {
		return "", referenceErr
	}
	var missing string
	result := pathParameterPattern.ReplaceAllStringFunc(path, func(match string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(match, "{"), "}")
		value := bindings[name]
		if value == "" {
			missing = name
			return match
		}
		return url.PathEscape(value)
	})
	if missing != "" {
		return "", fmt.Errorf("openhandle: missing bound reference %s", missing)
	}
	return result, nil
}

func cloneBindings(values map[string]string) map[string]string {
	result := make(map[string]string, len(values)+1)
	for key, value := range values {
		result[key] = value
	}
	return result
}
