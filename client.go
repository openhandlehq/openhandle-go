package openhandle

import (
	"errors"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"
)

const (
	defaultBaseURL    = "https://api.openhandle.dev"
	defaultMaxRetries = 2
	defaultTimeout    = 30 * time.Second
	sdkModulePath     = "github.com/openhandlehq/openhandle-go"
)

type clientCore struct {
	apiKey     string
	baseURL    *url.URL
	httpClient *http.Client
	maxRetries int
	timeout    time.Duration
}

func sdkVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "0.0.0"
	}
	if info.Main.Path == sdkModulePath && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	for _, dependency := range info.Deps {
		if dependency.Path == sdkModulePath && dependency.Version != "" && dependency.Version != "(devel)" {
			return strings.TrimPrefix(dependency.Version, "v")
		}
	}
	return "0.0.0"
}

// Option configures a Client.
type Option func(*clientConfig) error

type clientConfig struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
	timeout    time.Duration
}

// New creates a reusable Openhandle client. The API key selects the Test or
// Live environment; there is no separate environment option.
func New(apiKey string, options ...Option) (*Client, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("openhandle: API key must not be empty")
	}

	config := clientConfig{
		baseURL:    defaultBaseURL,
		maxRetries: defaultMaxRetries,
		timeout:    defaultTimeout,
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&config); err != nil {
			return nil, err
		}
	}

	baseURL, err := url.Parse(config.baseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, errors.New("openhandle: base URL must be an absolute HTTP or HTTPS URL")
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, errors.New("openhandle: base URL must use HTTP or HTTPS")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/")

	httpClient := config.httpClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	core := &clientCore{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
		maxRetries: config.maxRetries,
		timeout:    config.timeout,
	}
	return newGeneratedClient(core), nil
}

// WithBaseURL overrides the API origin, primarily for proxies and tests.
func WithBaseURL(value string) Option {
	return func(config *clientConfig) error {
		config.baseURL = strings.TrimRight(strings.TrimSpace(value), "/")
		return nil
	}
}

// WithHTTPClient supplies the HTTP client used for requests.
func WithHTTPClient(value *http.Client) Option {
	return func(config *clientConfig) error {
		if value == nil {
			return errors.New("openhandle: HTTP client must not be nil")
		}
		config.httpClient = value
		return nil
	}
}

// WithMaxRetries sets retry attempts after the initial request.
func WithMaxRetries(value int) Option {
	return func(config *clientConfig) error {
		if value < 0 {
			return errors.New("openhandle: max retries must not be negative")
		}
		config.maxRetries = value
		return nil
	}
}

// WithTimeout sets the default timeout for each request attempt.
func WithTimeout(value time.Duration) Option {
	return func(config *clientConfig) error {
		if value <= 0 {
			return errors.New("openhandle: timeout must be positive")
		}
		config.timeout = value
		return nil
	}
}
