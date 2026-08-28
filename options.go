package openhandle

import "time"

// RequestOptions contains controls common to every terminal operation.
// Context cancellation remains the preferred way to cancel a request.
type RequestOptions struct {
	MaxRetries *int          `query:"-"`
	Timeout    time.Duration `query:"-"`
}

// FetchOptions configures Client.Fetch.
type FetchOptions struct {
	RequestOptions
	Freshness Freshness `json:"freshness,omitempty"`
}

// Freshness controls the maximum acceptable age of returned data.
type Freshness string

const (
	FreshnessLive        Freshness = "live"
	FreshnessTwentyFourH Freshness = "24h"
	FreshnessSevenDays   Freshness = "7d"
	FreshnessThirtyDays  Freshness = "30d"
)

// SortOrder represents the common latest/top and recent/top sort options.
type SortOrder string

// Platform identifies a supported social platform.
type Platform string

const (
	PlatformInstagram Platform = "instagram"
	PlatformTikTok    Platform = "tiktok"
	PlatformTwitter   Platform = "twitter"
)

// Resource identifies an Openhandle resource family.
type Resource string
