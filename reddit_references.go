package openhandle

import "regexp"

var redditPostID = regexp.MustCompile(`^[a-z0-9]+$`)

func resolveRedditURL(host string, parts []string) (socialURLResolution, error) {
	if host == "redd.it" && len(parts) == 1 && redditPostID.MatchString(parts[0]) {
		return socialURLResolution{platform: "reddit", resource: "post", identifier: parts[0]}, nil
	}
	if len(parts) == 2 && (parts[0] == "u" || parts[0] == "user") && redditName.MatchString(parts[1]) {
		return socialURLResolution{platform: "reddit", resource: "profile", identifier: "@" + parts[1]}, nil
	}
	if len(parts) == 2 && parts[0] == "r" && redditName.MatchString(parts[1]) {
		return socialURLResolution{platform: "reddit", resource: "subreddit", identifier: parts[1]}, nil
	}
	if len(parts) >= 4 && parts[0] == "r" && parts[2] == "comments" && redditPostID.MatchString(parts[3]) {
		return socialURLResolution{platform: "reddit", resource: "post", identifier: parts[3]}, nil
	}
	if len(parts) >= 2 && parts[0] == "comments" && redditPostID.MatchString(parts[1]) {
		return socialURLResolution{platform: "reddit", resource: "post", identifier: parts[1]}, nil
	}
	return socialURLResolution{}, &ReferenceError{Message: "unsupported Reddit URL"}
}
