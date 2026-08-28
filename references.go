package openhandle

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ProfileReference is accepted by profile selectors. Raw strings use username
// shorthand unless they match a supported social URL.
type ProfileReference interface {
	string | Username | ID | URL
}

// ResourceReference is accepted by selectors whose natural shorthand is not a
// username. Raw strings use that shorthand unless they match a supported URL.
type ResourceReference interface {
	string | ID | URL
}

// Username selects a profile by username. A leading @ is accepted.
type Username string

// ID selects a resource by an opaque platform ID or native shorthand.
type ID string

// URL selects a resource by a social URL whose platform and resource are
// validated locally.
type URL string

var (
	numericID     = regexp.MustCompile(`^[0-9]+$`)
	instagramName = regexp.MustCompile(`^[A-Za-z0-9._]{1,30}$`)
	tikTokName    = regexp.MustCompile(`^[A-Za-z0-9._]{2,24}$`)
	twitterName   = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)
	shortcode     = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

type socialURLResolution struct {
	identifier string
	platform   string
	resource   string
}

func resolveReference[T ProfileReference](input T, platform, resource string) (string, error) {
	value := strings.TrimSpace(string(input))
	if value == "" {
		return "", &ReferenceError{Message: "reference values must not be empty"}
	}

	switch any(input).(type) {
	case Username:
		if resource != "profile" {
			return "", &ReferenceError{Message: fmt.Sprintf("the %s resource does not accept username references", resource)}
		}
		return usernameReference(value, platform)
	case ID:
		return value, nil
	case URL:
		return resolveURLReference(value, platform, resource)
	default:
		return resolveRawReference(value, platform, resource)
	}
}

func resolveRawReference(value, platform, resource string) (string, error) {
	if looksLikeSupportedSocialURL(value) {
		return resolveURLReference(value, platform, resource)
	}
	if resource == "profile" {
		return usernameReference(value, platform)
	}
	return value, nil
}

func resolveURLReference(value, platform, resource string) (string, error) {
	resolution, err := resolveSocialURL(value)
	if err != nil {
		return "", err
	}
	if resolution.platform != platform || resolution.resource != resource {
		return "", &ReferenceMismatchError{
			ExpectedPlatform: platform,
			ExpectedResource: resource,
			ActualPlatform:   resolution.platform,
			ActualResource:   resolution.resource,
		}
	}
	return resolution.identifier, nil
}

func looksLikeSupportedSocialURL(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	hasScheme := strings.Contains(value, "://")
	if hasScheme {
		value = value[strings.Index(value, "://")+3:]
	}
	separator := strings.IndexAny(value, "/?#")
	if !hasScheme && separator < 0 {
		return false
	}
	authority := value
	if separator >= 0 {
		authority = value[:separator]
	}
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		authority = authority[at+1:]
	}
	if colon := strings.LastIndex(authority, ":"); colon >= 0 {
		authority = authority[:colon]
	}
	authority = strings.TrimPrefix(authority, "www.")
	return supportedSocialHost(authority)
}

func supportedSocialHost(host string) bool {
	switch host {
	case "instagram.com", "tiktok.com", "m.tiktok.com", "x.com", "twitter.com", "mobile.twitter.com":
		return true
	default:
		return false
	}
}

func usernameReference(value, platform string) (string, error) {
	username := strings.TrimPrefix(value, "@")
	var valid bool
	switch platform {
	case "instagram":
		valid = instagramName.MatchString(username)
	case "tiktok":
		valid = tikTokName.MatchString(username)
	case "twitter":
		valid = twitterName.MatchString(username)
	}
	if !valid {
		return "", &ReferenceError{Message: fmt.Sprintf("invalid %s username", platform)}
	}
	return "@" + username, nil
}

func resolveSocialURL(input string) (socialURLResolution, error) {
	parsed, err := url.Parse(input)
	if err != nil || parsed.Host == "" {
		parsed, err = url.Parse("https://" + input)
	}
	if err != nil || parsed.Host == "" {
		return socialURLResolution{}, &ReferenceError{Message: "invalid social URL", Cause: err}
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return socialURLResolution{}, &ReferenceError{Message: "social URLs must use HTTP or HTTPS and cannot contain credentials"}
	}

	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	parts := strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' })
	switch host {
	case "instagram.com":
		return resolveInstagramURL(parts)
	case "tiktok.com", "m.tiktok.com":
		return resolveTikTokURL(parts)
	case "x.com", "twitter.com", "mobile.twitter.com":
		return resolveTwitterURL(parts)
	default:
		return socialURLResolution{}, &ReferenceError{Message: fmt.Sprintf("unsupported social domain %s", host)}
	}
}

func resolveInstagramURL(parts []string) (socialURLResolution, error) {
	if len(parts) == 2 && contains([]string{"p", "reel", "tv"}, parts[0]) && shortcode.MatchString(parts[1]) {
		return socialURLResolution{platform: "instagram", resource: "post", identifier: parts[1]}, nil
	}
	if len(parts) == 3 && parts[0] == "stories" && parts[1] == "highlights" && numericID.MatchString(parts[2]) {
		return socialURLResolution{platform: "instagram", resource: "highlight", identifier: parts[2]}, nil
	}
	if len(parts) == 3 && parts[0] == "stories" && instagramName.MatchString(parts[1]) && numericID.MatchString(parts[2]) {
		return socialURLResolution{platform: "instagram", resource: "story", identifier: parts[2]}, nil
	}
	if len(parts) == 1 && instagramName.MatchString(parts[0]) && !contains([]string{"p", "reel", "reels", "tv", "explore", "accounts", "direct", "stories"}, strings.ToLower(parts[0])) {
		return socialURLResolution{platform: "instagram", resource: "profile", identifier: "@" + parts[0]}, nil
	}
	return socialURLResolution{}, &ReferenceError{Message: "unsupported Instagram URL"}
}

func resolveTikTokURL(parts []string) (socialURLResolution, error) {
	if len(parts) == 0 {
		return socialURLResolution{}, &ReferenceError{Message: "unsupported TikTok URL"}
	}
	username := strings.TrimPrefix(parts[0], "@")
	if !strings.HasPrefix(parts[0], "@") || !tikTokName.MatchString(username) {
		return socialURLResolution{}, &ReferenceError{Message: "unsupported TikTok URL"}
	}
	if len(parts) == 3 && parts[1] == "video" && numericID.MatchString(parts[2]) {
		return socialURLResolution{platform: "tiktok", resource: "post", identifier: parts[2]}, nil
	}
	if len(parts) == 1 {
		return socialURLResolution{platform: "tiktok", resource: "profile", identifier: "@" + username}, nil
	}
	return socialURLResolution{}, &ReferenceError{Message: "unsupported TikTok URL"}
}

func resolveTwitterURL(parts []string) (socialURLResolution, error) {
	if len(parts) >= 3 && strings.EqualFold(parts[1], "status") && twitterName.MatchString(parts[0]) && numericID.MatchString(parts[2]) {
		return socialURLResolution{platform: "twitter", resource: "post", identifier: parts[2]}, nil
	}
	if len(parts) == 4 && parts[0] == "i" && parts[1] == "web" && parts[2] == "status" && numericID.MatchString(parts[3]) {
		return socialURLResolution{platform: "twitter", resource: "post", identifier: parts[3]}, nil
	}
	if len(parts) == 1 && twitterName.MatchString(parts[0]) && !contains([]string{"home", "explore", "search", "settings", "messages", "notifications", "i", "intent", "share"}, strings.ToLower(parts[0])) {
		return socialURLResolution{platform: "twitter", resource: "profile", identifier: "@" + parts[0]}, nil
	}
	return socialURLResolution{}, &ReferenceError{Message: "unsupported Twitter URL"}
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
