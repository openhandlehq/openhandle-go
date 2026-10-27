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

func TestYouTubePostFilesDecodeEveryStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/youtube/posts/dQw4w9WgXcQ/files" {
			t.Errorf("unexpected request: %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"platform":"youtube","resource":"files","data":{"postId":"dQw4w9WgXcQ","author":null,"duration":213,"renditions":[`+
			`{"itag":18,"quality":"360p","mimeType":"video/mp4","codecs":"avc1.42001E, mp4a.40.2","width":640,"height":360,"fps":25,"bitrate":443000,"size":null,"audioQuality":"low","audioLanguage":null,"hasVideo":true,"hasAudio":true,"url":"https://media.openhandle.dev/a"},`+
			`{"itag":140,"quality":"audio","mimeType":"audio/mp4","codecs":"mp4a.40.2","width":null,"height":null,"fps":null,"bitrate":130677,"size":3449447,"audioQuality":"medium","audioLanguage":null,"hasVideo":false,"hasAudio":true,"url":"https://media.openhandle.dev/b"}]}}`)
	}))
	defer server.Close()
	client, err := New("oh_test_sdk", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	files, err := client.YouTube.Post("https://www.youtube.com/shorts/dQw4w9WgXcQ").Files.Get(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if files.Data.PostID != "dQw4w9WgXcQ" || len(files.Data.Renditions) != 2 {
		t.Fatalf("unexpected files: %#v", files.Data)
	}
	audio := files.Data.Renditions[1]
	if audio.Itag != 140 || audio.HasVideo || !audio.HasAudio || audio.Size == nil || *audio.Size != 3449447 {
		t.Fatalf("unexpected audio stream: %#v", audio)
	}
}

func TestYouTubePostStoryboardDecodesSheetsAndFramePositions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/youtube/posts/dQw4w9WgXcQ/storyboard" {
			t.Errorf("unexpected request: %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"platform":"youtube","resource":"storyboard","data":{"postId":"dQw4w9WgXcQ","author":null,"intervalMs":2000,"frameWidth":160,"frameHeight":90,"columns":5,"rows":5,"frameCount":2,`+
			`"sheets":[{"index":0,"url":"https://media.openhandle.dev/s","width":800,"height":90,"startMs":0,"frameCount":2}],`+
			`"frames":[{"timeMs":0,"sheet":0,"x":0,"y":0},{"timeMs":2000,"sheet":0,"x":160,"y":0}]}}`)
	}))
	defer server.Close()
	client, err := New("oh_test_sdk", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	storyboard, err := client.YouTube.Post("dQw4w9WgXcQ").Storyboard.Get(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(storyboard.Data.Sheets) != 1 || len(storyboard.Data.Frames) != 2 || storyboard.Data.Frames[1].X != 160 {
		t.Fatalf("unexpected storyboard: %#v", storyboard.Data)
	}
}

func TestYouTubePostFrameSendsTheMomentInSeconds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/youtube/posts/dQw4w9WgXcQ/frame" || request.URL.Query().Get("at") != "43" {
			t.Errorf("unexpected request: %s", request.URL.String())
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"platform":"youtube","resource":"frame","data":{"postId":"dQw4w9WgXcQ","at":43,"renditions":[{"quality":"1080p","height":1080,"url":"https://media.openhandle.dev/f"}]}}`)
	}))
	defer server.Close()
	client, err := New("oh_test_sdk", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	frame, err := client.YouTube.Post("dQw4w9WgXcQ").Frame.Get(t.Context(), &YouTubePostFrameOptions{At: 43})
	if err != nil {
		t.Fatal(err)
	}
	if frame.Data.At != 43 || len(frame.Data.Renditions) != 1 || frame.Data.Renditions[0].Quality == nil || *frame.Data.Renditions[0].Quality != "1080p" {
		t.Fatalf("unexpected frame: %#v", frame.Data)
	}
}
