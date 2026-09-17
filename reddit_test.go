package openhandle

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedditWikiSelectorsBindNestedPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/reddit/subreddits/python/wiki-pages":
			_, _ = fmt.Fprint(writer, `{"platform":"reddit","resource":"wiki","data":[{"title":"config/sidebar"}],"meta":{"cursors":{"next":null}}}`)
		case "/v1/reddit/subreddits/python/wiki-pages/config/sidebar":
			_, _ = fmt.Fprint(writer, `{"platform":"reddit","resource":"wiki","data":{"title":"config/sidebar","content":{"markdown":"Sidebar content"}}}`)
		default:
			t.Errorf("unexpected request: %s", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := New("oh_test_sdk", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	pages, err := client.Reddit.Subreddit("python").WikiPages.List(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages.Data) != 1 || pages.Data[0].Title != "config/sidebar" {
		t.Fatalf("unexpected pages: %#v", pages.Data)
	}
	page, err := client.Reddit.Subreddit("python").WikiPage("config/sidebar").Get(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Data.Title != "config/sidebar" {
		t.Fatalf("unexpected page: %#v", page.Data)
	}
}

func TestFetchDecodesRedditProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"platform":"reddit","resource":"profile","data":{"handle":"reddit-user","commentKarma":-2}}`)
	}))
	defer server.Close()
	client, err := New("oh_test_sdk", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Fetch(t.Context(), "https://reddit.com/user/reddit-user", nil)
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := response.Data.(*RedditProfile)
	if !ok || profile.Handle != "reddit-user" {
		t.Fatalf("unexpected profile: %#v", response.Data)
	}
}
