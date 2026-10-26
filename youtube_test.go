package openhandle

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestYouTubeSelectorsResolveURLsAndBindNestedComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/youtube/profiles/@MrBeast/videos":
			_, _ = fmt.Fprint(writer, `{"platform":"youtube","resource":"post","data":[{"id":"dQw4w9WgXcQ"}],"meta":{"cursors":{"next":null}}}`)
		case "/v1/youtube/posts/dQw4w9WgXcQ/comments/UgzComment.123/replies":
			if request.URL.Query().Get("token") != "replies-token" {
				t.Errorf("token = %q, want replies-token", request.URL.Query().Get("token"))
			}
			_, _ = fmt.Fprint(writer, `{"platform":"youtube","resource":"comment","data":[{"id":"UgzReply.456"}],"meta":{"cursors":{"next":null}}}`)
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
	videos, err := client.YouTube.Profile("https://www.youtube.com/@MrBeast/videos").Videos.List(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(videos.Data) != 1 || videos.Data[0].ID != "dQw4w9WgXcQ" {
		t.Fatalf("unexpected videos: %#v", videos.Data)
	}
	replies, err := client.YouTube.Post("https://youtu.be/dQw4w9WgXcQ").Comment("UgzComment.123").Replies.List(t.Context(), &YouTubePostCommentRepliesOptions{Token: "replies-token"})
	if err != nil {
		t.Fatal(err)
	}
	if len(replies.Data) != 1 || replies.Data[0].ID != "UgzReply.456" {
		t.Fatalf("unexpected replies: %#v", replies.Data)
	}
}

func TestYouTubeProfileSelectorRejectsVideoURLBeforeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Errorf("unexpected request: %s", request.URL.Path)
	}))
	defer server.Close()
	client, err := New("oh_test_sdk", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.YouTube.Profile(URL("https://www.youtube.com/watch?v=dQw4w9WgXcQ")).Get(t.Context(), nil)
	var mismatch *ReferenceMismatchError
	if !errors.As(err, &mismatch) || mismatch.ActualPlatform != "youtube" || mismatch.ActualResource != "post" {
		t.Fatalf("error = %T %v, want a YouTube post mismatch", err, err)
	}
}

func TestFetchDecodesYouTubeProfileAndPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		if body.URL == "https://youtu.be/dQw4w9WgXcQ" {
			_, _ = fmt.Fprint(writer, `{"platform":"youtube","resource":"post","data":{"id":"dQw4w9WgXcQ","title":"Never Gonna Give You Up"}}`)
			return
		}
		_, _ = fmt.Fprint(writer, `{"platform":"youtube","resource":"profile","data":{"id":"UCX6OQ3DkcsbYNE6H8uQQuVA","handle":"MrBeast"}}`)
	}))
	defer server.Close()
	client, err := New("oh_test_sdk", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	profileResponse, err := client.Fetch(t.Context(), "https://www.youtube.com/@MrBeast", nil)
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := profileResponse.Data.(*YouTubeProfile)
	if !ok || profile.ID != "UCX6OQ3DkcsbYNE6H8uQQuVA" {
		t.Fatalf("unexpected profile: %#v", profileResponse.Data)
	}
	postResponse, err := client.Fetch(t.Context(), "https://youtu.be/dQw4w9WgXcQ", nil)
	if err != nil {
		t.Fatal(err)
	}
	post, ok := postResponse.Data.(*YouTubePost)
	if !ok || post.ID != "dQw4w9WgXcQ" {
		t.Fatalf("unexpected post: %#v", postResponse.Data)
	}
}
