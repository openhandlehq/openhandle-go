# Openhandle Go SDK

The official Go client for the Openhandle public API. It requires Go 1.27 or
newer.

## Installation

```bash
go get github.com/openhandlehq/openhandle-go
```

## Usage

Create a Test key in the [Openhandle dashboard](https://app.openhandle.dev),
store it as `OPENHANDLE_TEST_KEY`, and create one reusable client:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	openhandle "github.com/openhandlehq/openhandle-go"
)

func main() {
	client, err := openhandle.New(os.Getenv("OPENHANDLE_TEST_KEY"))
	if err != nil {
		log.Fatal(err)
	}

	profile := client.Instagram.Profile("northstar_forge_test")
	response, err := profile.Get(context.Background(), &openhandle.InstagramProfileOptions{
		Freshness: openhandle.FreshnessTwentyFourH,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Data.Handle, response.RequestID, response.Billing.Cost)
}
```

The key selects the environment. `oh_test_` keys return deterministic synthetic
data with a `$0.000` actual charge; `oh_live_` keys use real public identifiers
and normal billing. Never ship an API key in a browser or mobile binary.

## Resource selection

The SDK follows one predictable grammar:

```text
client.<Platform>.<Resource>(reference).<Subresource>.<Operation>(ctx, options)
```

Only terminal operations such as `Get`, `List`, `Search`, and `Fetch` perform a
network request. Selectors are synchronous and reusable:

```go
post := client.Instagram.Post("Db04otPRpRH")

details, err := post.Get(ctx, nil)
comments, err := post.Comments.List(ctx, nil)
```

Raw strings use the selected resource's natural shorthand. Supported social
URLs are recognized and validated locally before a request:

```go
client.Instagram.Profile("openai")
client.Instagram.Profile("https://www.instagram.com/openai/")
client.Instagram.Post("Db04otPRpRH")
```

Numeric profile strings are usernames. Platform IDs must be explicit so the
SDK never guesses between a numeric username and an opaque ID:

```go
client.Instagram.Profile("12356")
client.Instagram.Profile(openhandle.Username("12356"))
client.Instagram.Profile(openhandle.ID("25025320"))
client.Instagram.Profile(openhandle.URL("https://www.instagram.com/openai/"))
```

Raw strings and explicit references are compile-time checked by Go 1.27 generic
selector methods. Resolution is entirely local: the SDK never follows
redirects, makes hidden lookup requests, or tries multiple interpretations.

Use `Fetch` when the URL's platform or resource is not known:

```go
response, err := client.Fetch(ctx, "https://www.instagram.com/p/Db04otPRpRH/", nil)

switch resource := response.Data.(type) {
case *openhandle.InstagramPost:
	fmt.Println(resource.ID)
}
```

## Pagination

Pages preserve the public envelope and expose the opaque next cursor:

```go
page, err := client.Instagram.Profile(
	"northstar_forge_test",
).Posts.List(ctx, nil)

for page != nil {
	for _, post := range page.Data {
		fmt.Println(post.ID)
	}
	page, err = page.Next(ctx)
	if err != nil {
		return err
	}
}
```

The iterator is lazy and requests one page at a time:

```go
iterator := client.Instagram.Profile(
	"northstar_forge_test",
).Posts.Items(nil)

for iterator.Next(ctx) {
	fmt.Println(iterator.Value().ID)
}
if err := iterator.Err(); err != nil {
	return err
}
```

## Errors and retries

API and transport failures return `*openhandle.Error`. Branch on `Code`, never
`Message`, and include `RequestID` in logs or support requests:

```go
var apiError *openhandle.Error
if errors.As(err, &apiError) {
	log.Printf("code=%s request_id=%s retryable=%t", apiError.Code, apiError.RequestID, apiError.Retryable)
}
```

The client retries explicitly retryable API failures and transient transport
failures twice by default. It honors `Retry-After`, uses capped exponential
backoff with jitter, and obeys context cancellation and request timeouts.

## Configuration

```go
client, err := openhandle.New(
	apiKey,
	openhandle.WithBaseURL("https://api.openhandle.dev"),
	openhandle.WithHTTPClient(httpClient),
	openhandle.WithMaxRetries(2),
	openhandle.WithTimeout(30*time.Second),
)
```

Per-operation controls override client defaults:

```go
zeroRetries := 0
response, err := client.Twitter.Profile("openai").Get(ctx,
	&openhandle.TwitterProfileOptions{
		RequestOptions: openhandle.RequestOptions{
			MaxRetries: &zeroRetries,
			Timeout:    5 * time.Second,
		},
	},
)
```

## Generation

The pinned [`openapi/openhandle.json`](./openapi/openhandle.json) document is the
source for all models, operation options, responses, pages, and resource
methods. Generated Go is committed so normal users do not need generator tools.

```bash
go generate ./...
go run ./internal/cmd/generate --check
go test -race ./...
```

## License

[MIT](./LICENSE)
