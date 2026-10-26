package openhandle

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	youTubePostID    = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	youTubeProfileID = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)
)

func resolveYouTubeURL(host string, parsed *url.URL, parts []string) (socialURLResolution, error) {
	if host == "youtu.be" && len(parts) == 1 && youTubePostID.MatchString(parts[0]) {
		return socialURLResolution{platform: "youtube", resource: "post", identifier: parts[0]}, nil
	}
	if len(parts) == 1 && parts[0] == "watch" && youTubePostID.MatchString(parsed.Query().Get("v")) {
		return socialURLResolution{platform: "youtube", resource: "post", identifier: parsed.Query().Get("v")}, nil
	}
	if len(parts) == 2 && contains([]string{"shorts", "live", "embed"}, parts[0]) && youTubePostID.MatchString(parts[1]) {
		return socialURLResolution{platform: "youtube", resource: "post", identifier: parts[1]}, nil
	}
	if len(parts) >= 1 && strings.HasPrefix(parts[0], "@") && youTubeHandle.MatchString(strings.TrimPrefix(parts[0], "@")) {
		return socialURLResolution{platform: "youtube", resource: "profile", identifier: parts[0]}, nil
	}
	if len(parts) >= 2 && parts[0] == "channel" && youTubeProfileID.MatchString(parts[1]) {
		return socialURLResolution{platform: "youtube", resource: "profile", identifier: parts[1]}, nil
	}
	return socialURLResolution{}, &ReferenceError{Message: "unsupported YouTube URL"}
}
