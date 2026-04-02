package greader

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestListSubscriptionsAndUnreadCounts(t *testing.T) {
	var authHeader string
	client := New("https://rss.example.com/api/greader.php", "alice", "secret")
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://rss.example.com/api/greader.php/accounts/ClientLogin":
			body, _ := io.ReadAll(req.Body)
			if got := string(body); got != "Email=alice&Passwd=secret" {
				t.Fatalf("unexpected login body %q", got)
			}
			return responseWithBody(http.StatusOK, "SID=alice/token\nAuth=alice/token\n"), nil
		case "https://rss.example.com/api/greader.php/reader/api/0/subscription/list?output=json":
			authHeader = req.Header.Get("Authorization")
			return responseWithJSON(http.StatusOK, `{
				"subscriptions": [
					{
						"id": "feed/http://example.com/feed.xml",
						"title": "Example Feed",
						"url": "http://example.com/feed.xml",
						"htmlUrl": "http://example.com/",
						"categories": [{"id":"user/-/label/Tech","label":"Tech"}]
					}
				]
			}`), nil
		case "https://rss.example.com/api/greader.php/reader/api/0/unread-count?output=json":
			return responseWithJSON(http.StatusOK, `{
				"unreadcounts": [
					{"id": "feed/http://example.com/feed.xml", "count": 7}
				]
			}`), nil
		default:
			t.Fatalf("unexpected request %s", req.URL.String())
			return nil, nil
		}
	})}

	subscriptions, err := client.ListSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("ListSubscriptions returned error: %v", err)
	}
	if len(subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subscriptions))
	}
	if subscriptions[0].Category != "Tech" {
		t.Fatalf("expected category Tech, got %q", subscriptions[0].Category)
	}
	if authHeader != "GoogleLogin auth=alice/token" {
		t.Fatalf("unexpected Authorization header %q", authHeader)
	}

	counts, err := client.UnreadCounts(context.Background())
	if err != nil {
		t.Fatalf("UnreadCounts returned error: %v", err)
	}
	if got := counts["feed/http://example.com/feed.xml"]; got != 7 {
		t.Fatalf("expected unread count 7, got %d", got)
	}
}

func TestStreamContentsParsesEntries(t *testing.T) {
	var requestedURL string
	client := New("https://rss.example.com/api/greader.php", "alice", "secret")
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://rss.example.com/api/greader.php/accounts/ClientLogin":
			return responseWithBody(http.StatusOK, "Auth=alice/token\n"), nil
		default:
			requestedURL = req.URL.String()
			return responseWithJSON(http.StatusOK, `{
				"items": [
					{
						"id": "tag:google.com,2005:reader/item/abc123",
						"title": "Remote Article",
						"published": 1710000000,
						"categories": ["user/-/state/com.google/read"],
						"alternate": [{"href":"https://example.com/articles/1"}],
						"summary": {"content":"<p>Hello world</p>"},
						"origin": {"streamId":"feed/http://example.com/feed.xml"}
					}
				]
			}`), nil
		}
	})}

	entries, err := client.StreamContents(context.Background(), "feed/http://example.com/feed.xml", 25)
	if err != nil {
		t.Fatalf("StreamContents returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Link != "https://example.com/articles/1" {
		t.Fatalf("unexpected link %q", entries[0].Link)
	}
	if !entries[0].Read {
		t.Fatal("expected entry to be marked read from categories")
	}
	if !strings.Contains(entries[0].ContentHTML, "Hello world") {
		t.Fatalf("unexpected content %q", entries[0].ContentHTML)
	}
	wantURL := "https://rss.example.com/api/greader.php/reader/api/0/stream/contents/feed%2Fhttp:%2F%2Fexample.com%2Ffeed.xml?n=25&output=json"
	if requestedURL != wantURL {
		t.Fatalf("unexpected stream request %q want %q", requestedURL, wantURL)
	}
}

func TestQuickAddReturnsStreamID(t *testing.T) {
	client := New("https://rss.example.com/api/greader.php", "alice", "secret")
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://rss.example.com/api/greader.php/accounts/ClientLogin":
			return responseWithBody(http.StatusOK, "Auth=alice/token\n"), nil
		case "https://rss.example.com/api/greader.php/reader/api/0/subscription/quickadd":
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", req.Method)
			}
			body, _ := io.ReadAll(req.Body)
			if got := string(body); got != "quickadd=https%3A%2F%2Fexample.com" {
				t.Fatalf("unexpected quickadd body %q", got)
			}
			if got := req.Header.Get("Authorization"); got != "GoogleLogin auth=alice/token" {
				t.Fatalf("unexpected Authorization header %q", got)
			}
			return responseWithJSON(http.StatusOK, `{
				"numResults": 1,
				"query": "https://example.com/feed.xml",
				"streamId": "feed/52",
				"streamName": "Example Feed"
			}`), nil
		default:
			t.Fatalf("unexpected request %s", req.URL.String())
			return nil, nil
		}
	})}

	result, err := client.QuickAdd(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("QuickAdd returned error: %v", err)
	}
	if result.StreamID != "feed/52" {
		t.Fatalf("unexpected stream id %q", result.StreamID)
	}
	if result.StreamName != "Example Feed" {
		t.Fatalf("unexpected stream name %q", result.StreamName)
	}
}

func TestClientLoginRequiresConfig(t *testing.T) {
	client := New("", "", "")

	err := client.ensureAuth(context.Background())
	if err == nil {
		t.Fatal("expected configuration error")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func responseWithBody(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func responseWithJSON(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
