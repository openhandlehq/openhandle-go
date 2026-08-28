package reference_test

import (
	"context"
	"errors"
	"time"

	openhandle "github.com/openhandlehq/openhandle-go"
)

func profileGet(ctx context.Context, client *openhandle.Client) (openhandle.InstagramProfile, error) {
	response, err := client.Instagram.Profile("northstar_forge_test").Get(ctx, &openhandle.InstagramProfileOptions{
		Freshness: openhandle.FreshnessTwentyFourH,
	})
	if err != nil {
		return openhandle.InstagramProfile{}, err
	}
	return response.Data, nil
}

func explicitReferences(ctx context.Context, client *openhandle.Client) error {
	if _, err := client.TikTok.Profile(openhandle.ID("25025320")).Get(ctx, nil); err != nil {
		return err
	}
	_, err := client.Instagram.Post(openhandle.URL("https://www.instagram.com/p/Db04otPRpRH/")).Get(ctx, nil)
	return err
}

func rawURLReference(ctx context.Context, client *openhandle.Client) (*openhandle.InstagramProfileResponse, error) {
	return client.Instagram.Profile("https://www.instagram.com/openai/").Get(ctx, nil)
}

func collectPostIDs(ctx context.Context, client *openhandle.Client) ([]string, error) {
	var ids []string
	page, err := client.Instagram.Profile("northstar_forge_test").Posts.List(ctx, nil)
	for page != nil && err == nil {
		for _, post := range page.Data {
			ids = append(ids, post.ID)
		}
		page, err = page.Next(ctx)
	}
	return ids, err
}

func nestedReplies(ctx context.Context, client *openhandle.Client) (*openhandle.InstagramPostCommentRepliesPage, error) {
	return client.Instagram.
		Post("Db04otPRpRH").
		Comment(openhandle.ID("18120112390529134")).
		Replies.List(ctx, nil)
}

func searchTikTokPosts(ctx context.Context, client *openhandle.Client) (*openhandle.TikTokSearchPostsPage, error) {
	return client.TikTok.Search.Posts.List(ctx, &openhandle.TikTokSearchPostsOptions{
		Q:         "synthetic",
		Freshness: openhandle.FreshnessTwentyFourH,
	})
}

func fetchUnknownURL(ctx context.Context, client *openhandle.Client) (*openhandle.FetchResponse, error) {
	return client.Fetch(ctx, "https://www.instagram.com/p/Db04otPRpRH/", nil)
}

func errorFields(err error) (code, requestID string, retryable bool) {
	var apiError *openhandle.Error
	if errors.As(err, &apiError) {
		return apiError.Code, apiError.RequestID, apiError.Retryable
	}
	return "", "", false
}

func requestControls(ctx context.Context, client *openhandle.Client) (*openhandle.TwitterProfileResponse, error) {
	zeroRetries := 0
	return client.Twitter.Profile("openai").Get(ctx, &openhandle.TwitterProfileOptions{
		RequestOptions: openhandle.RequestOptions{
			MaxRetries: &zeroRetries,
			Timeout:    5 * time.Second,
		},
	})
}

func billingMetadata(ctx context.Context, client *openhandle.Client) (cost, environment, requestID string, err error) {
	response, err := client.Instagram.Profile("northstar_forge_test").Get(ctx, nil)
	if err != nil {
		return "", "", "", err
	}
	return response.Billing.Cost, response.Billing.Environment, response.RequestID, nil
}

func reusableClient(apiKey string) (*openhandle.Client, error) {
	return openhandle.New(apiKey)
}
