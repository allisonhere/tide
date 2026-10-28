package greader

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestUnescapeAPIString(t *testing.T) {
	if got := UnescapeAPIString(`  NASA&#039;s feed  `); got != "NASA's feed" {
		t.Fatalf("apostrophe entity: got %q", got)
	}
	if got := UnescapeAPIString(`Tom &amp; Jerry`); got != "Tom & Jerry" {
		t.Fatalf("ampersand entity: got %q", got)
	}
	if got := UnescapeAPIString(`NASA&amp;#039;s Podcast`); got != "NASA's Podcast" {
		t.Fatalf("double-encoded apostrophe: got %q", got)
	}
}

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
						"title": "NASA&#039;s Example Feed",
						"url": "http://example.com/feed.xml",
						"htmlUrl": "http://example.com/",
						"categories": [{"id":"user/-/label/Tech","label":"Tom &amp; Jerry"}]
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
	if subscriptions[0].Title != "NASA's Example Feed" {
		t.Fatalf("expected decoded title, got %q", subscriptions[0].Title)
	}
	if subscriptions[0].Category != "Tom & Jerry" {
		t.Fatalf("expected decoded category label, got %q", subscriptions[0].Category)
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

func TestUnreadCountsFallBackToItemIDsWhenTheServerAnswersAnArray(t *testing.T) {
	client := New("https://rss.example.com", "alice", "secret")
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/accounts/ClientLogin":
			return responseWithBody(http.StatusOK, "SID=alice/token\nAuth=alice/token\n"), nil
		case "/reader/api/0/unread-count":
			// What Miniflux answers: it has no unread-count endpoint.
			return responseWithJSON(http.StatusOK, `[]`), nil
		case "/reader/api/0/subscription/list":
			return responseWithJSON(http.StatusOK, `{"subscriptions":[
				{"id":"feed/1","title":"A","url":"http://a/feed"},
				{"id":"feed/2","title":"B","url":"http://b/feed"},
				{"id":"feed/3","title":"C","url":"http://c/feed"}]}`), nil
		case "/reader/api/0/stream/items/ids":
			q := req.URL.Query()
			if q.Get("xt") != "user/-/state/com.google/read" {
				t.Fatalf("unread items should exclude read ones, got xt=%q", q.Get("xt"))
			}
			switch q.Get("s") {
			case "feed/1":
				return responseWithJSON(http.StatusOK, `{"itemRefs":[{"id":"10"},{"id":"11"},{"id":"12"}]}`), nil
			case "feed/2":
				return responseWithJSON(http.StatusInternalServerError, `{"error":"boom"}`), nil
			default:
				return responseWithJSON(http.StatusOK, `{"itemRefs":[]}`), nil
			}
		default:
			t.Fatalf("unexpected request %s", req.URL.String())
			return nil, nil
		}
	})}

	counts, err := client.UnreadCounts(context.Background())
	if err != nil {
		t.Fatalf("UnreadCounts returned error: %v", err)
	}
	if len(counts) != 1 || counts["feed/1"] != 3 {
		t.Fatalf("expected only feed/1 with 3 unread (a failing feed counts as none), got %v", counts)
	}
}

func TestStreamContentsFallBackToItemContentsWhenTheServerAnswersAnArray(t *testing.T) {
	client := New("https://rss.example.com", "alice", "secret")
	var posted url.Values
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/accounts/ClientLogin":
			return responseWithBody(http.StatusOK, "SID=alice/token\nAuth=alice/token\n"), nil
		case "/reader/api/0/stream/contents/feed/7":
			// What Miniflux answers: it has no stream/contents endpoint.
			return responseWithJSON(http.StatusOK, `[]`), nil
		case "/reader/api/0/stream/items/ids":
			q := req.URL.Query()
			if q.Get("s") != "feed/7" || q.Get("xt") != "user/-/state/com.google/read" || q.Get("n") != "50" {
				t.Fatalf("unexpected ids query %s", req.URL.RawQuery)
			}
			return responseWithJSON(http.StatusOK, `{"itemRefs":[{"id":"8717"},{"id":"8707"}]}`), nil
		case "/reader/api/0/token":
			return responseWithBody(http.StatusOK, "csrf-token\n"), nil
		case "/reader/api/0/stream/items/contents":
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", req.Method)
			}
			body, _ := io.ReadAll(req.Body)
			posted, _ = url.ParseQuery(string(body))
			return responseWithJSON(http.StatusOK, `{"items":[
				{"id":"tag:google.com,2005:reader/item/00000000000021fd","title":"First","published":1700000000,
				 "categories":["user/-/state/com.google/reading-list"],
				 "alternate":[{"href":"http://example.com/1"}],
				 "summary":{"content":"<p>one</p>"},"origin":{"streamId":"feed/7"}},
				{"id":"tag:google.com,2005:reader/item/00000000000021f3","title":"Second","published":1700000100,
				 "categories":["user/-/state/com.google/reading-list","user/-/state/com.google/read"],
				 "alternate":[{"href":"http://example.com/2"}],
				 "summary":{"content":"<p>two</p>"},"origin":{"streamId":"feed/7"}}]}`), nil
		default:
			t.Fatalf("unexpected request %s", req.URL.String())
			return nil, nil
		}
	})}

	entries, err := client.StreamContents(context.Background(), "feed/7", 50)
	if err != nil {
		t.Fatalf("StreamContents returned error: %v", err)
	}
	if posted.Get("T") != "csrf-token" || strings.Join(posted["i"], ",") != "8717,8707" {
		t.Fatalf("unexpected items/contents form %v", posted)
	}
	if len(entries) != 2 || entries[0].Title != "First" || entries[0].Link != "http://example.com/1" ||
		entries[0].ContentHTML != "<p>one</p>" || entries[0].StreamID != "feed/7" || entries[0].Read || !entries[1].Read {
		t.Fatalf("unexpected entries %+v", entries)
	}
}

func TestStreamContentsWithNothingUnreadMakesNoContentsRequest(t *testing.T) {
	client := New("https://rss.example.com", "alice", "secret")
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/accounts/ClientLogin":
			return responseWithBody(http.StatusOK, "SID=alice/token\nAuth=alice/token\n"), nil
		case "/reader/api/0/stream/contents/feed/7":
			return responseWithJSON(http.StatusOK, `[]`), nil
		case "/reader/api/0/stream/items/ids":
			return responseWithJSON(http.StatusOK, `{"itemRefs":[]}`), nil
		default:
			t.Fatalf("unexpected request %s", req.URL.String())
			return nil, nil
		}
	})}
	entries, err := client.StreamContents(context.Background(), "feed/7", 0)
	if err != nil || len(entries) != 0 {
		t.Fatalf("entries %v, err %v", entries, err)
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
						"title": "It&#039;s a match",
						"author": "Ada Lovelace",
						"published": 1710000000,
						"updated": 1710200000,
						"categories": ["user/-/state/com.google/read", "user/-/label/Tech", "user/-/label/Science"],
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
	if entries[0].Title != "It's a match" {
		t.Fatalf("expected decoded article title, got %q", entries[0].Title)
	}
	if !strings.Contains(entries[0].ContentHTML, "Hello world") {
		t.Fatalf("unexpected content %q", entries[0].ContentHTML)
	}
	if entries[0].Author != "Ada Lovelace" {
		t.Fatalf("unexpected author %q", entries[0].Author)
	}
	if got := entries[0].Categories; len(got) != 2 || got[0] != "Tech" || got[1] != "Science" {
		t.Fatalf("Categories = %#v, want user label names only", got)
	}
	if entries[0].UpdatedAt.Unix() != 1710200000 {
		t.Fatalf("UpdatedAt = %v, want unix 1710200000", entries[0].UpdatedAt)
	}
	wantURL := "https://rss.example.com/api/greader.php/reader/api/0/stream/contents/feed%2Fhttp:%2F%2Fexample.com%2Ffeed.xml?n=25&output=json&xt=user%2F-%2Fstate%2Fcom.google%2Fread"
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

func TestMarkEntryReadUsesTokenAndEditTag(t *testing.T) {
	client := New("https://rss.example.com/api/greader.php", "alice", "secret")
	tokenCalls := 0
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://rss.example.com/api/greader.php/accounts/ClientLogin":
			return responseWithBody(http.StatusOK, "Auth=alice/token\n"), nil
		case "https://rss.example.com/api/greader.php/reader/api/0/token":
			tokenCalls++
			if got := req.Header.Get("Authorization"); got != "GoogleLogin auth=alice/token" {
				t.Fatalf("unexpected Authorization header %q", got)
			}
			return responseWithBody(http.StatusOK, "csrf-token"), nil
		case "https://rss.example.com/api/greader.php/reader/api/0/edit-tag":
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", req.Method)
			}
			body, _ := io.ReadAll(req.Body)
			if got := string(body); got != "T=csrf-token&a=user%2F-%2Fstate%2Fcom.google%2Fread&i=tag%3Agoogle.com%2C2005%3Areader%2Fitem%2Fabc123&r=user%2F-%2Fstate%2Fcom.google%2Fkept-unread" {
				t.Fatalf("unexpected edit-tag body %q", got)
			}
			return responseWithBody(http.StatusOK, "OK"), nil
		default:
			t.Fatalf("unexpected request %s", req.URL.String())
			return nil, nil
		}
	})}

	if err := client.MarkEntryRead(context.Background(), "tag:google.com,2005:reader/item/abc123", true); err != nil {
		t.Fatalf("MarkEntryRead returned error: %v", err)
	}
	if err := client.MarkEntryRead(context.Background(), "tag:google.com,2005:reader/item/abc123", true); err != nil {
		t.Fatalf("MarkEntryRead returned error on cached token: %v", err)
	}
	if tokenCalls != 1 {
		t.Fatalf("expected token endpoint to be called once, got %d", tokenCalls)
	}
}

func TestMarkEntryReadFallsBackToReadAloneWhenTheServerRefusesBothTags(t *testing.T) {
	client := New("https://rss.example.com", "alice", "secret")
	var forms []url.Values
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/accounts/ClientLogin":
			return responseWithBody(http.StatusOK, "SID=alice/token\nAuth=alice/token\n"), nil
		case "/reader/api/0/token":
			return responseWithBody(http.StatusOK, "csrf-token\n"), nil
		case "/reader/api/0/edit-tag":
			body, _ := io.ReadAll(req.Body)
			form, _ := url.ParseQuery(string(body))
			forms = append(forms, form)
			// What Miniflux does with a=read together with r=kept-unread.
			if form.Get("r") != "" {
				return responseWithJSON(http.StatusInternalServerError, `{"error_message":"googlereader: kept-unread and read should not be supplied simultaneously"}`), nil
			}
			return responseWithBody(http.StatusOK, "OK"), nil
		default:
			t.Fatalf("unexpected request %s", req.URL.String())
			return nil, nil
		}
	})}

	if err := client.MarkEntryRead(context.Background(), "tag:google.com,2005:reader/item/00000000000021fd", true); err != nil {
		t.Fatalf("MarkEntryRead returned error: %v", err)
	}
	if len(forms) != 2 || forms[0].Get("a") != "user/-/state/com.google/read" || forms[0].Get("r") != "user/-/state/com.google/kept-unread" {
		t.Fatalf("the first try should add read and remove kept-unread, got %v", forms)
	}
	if forms[1].Get("a") != "user/-/state/com.google/read" || forms[1].Get("r") != "" || forms[1].Get("T") != "csrf-token" {
		t.Fatalf("the retry should add read alone, with the token, got %v", forms[1])
	}
}

func TestMarkAllReadUsesTokenAndStreamID(t *testing.T) {
	client := New("https://rss.example.com/api/greader.php", "alice", "secret")
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://rss.example.com/api/greader.php/accounts/ClientLogin":
			return responseWithBody(http.StatusOK, "Auth=alice/token\n"), nil
		case "https://rss.example.com/api/greader.php/reader/api/0/token":
			return responseWithBody(http.StatusOK, "csrf-token"), nil
		case "https://rss.example.com/api/greader.php/reader/api/0/mark-all-as-read":
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", req.Method)
			}
			body, _ := io.ReadAll(req.Body)
			if got := string(body); got != "T=csrf-token&s=feed%2Fhttp%3A%2F%2Fexample.com%2Ffeed.xml" {
				t.Fatalf("unexpected mark-all-as-read body %q", got)
			}
			return responseWithBody(http.StatusOK, "OK"), nil
		default:
			t.Fatalf("unexpected request %s", req.URL.String())
			return nil, nil
		}
	})}

	if err := client.MarkAllRead(context.Background(), "feed/http://example.com/feed.xml"); err != nil {
		t.Fatalf("MarkAllRead returned error: %v", err)
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

func TestMarkEntryReadDoesNotRetryOtherFailures(t *testing.T) {
	client := New("https://rss.example.com", "alice", "secret")
	edits := 0
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/accounts/ClientLogin":
			return responseWithBody(http.StatusOK, "SID=alice/token\nAuth=alice/token\n"), nil
		case "/reader/api/0/token":
			return responseWithBody(http.StatusOK, "csrf-token\n"), nil
		case "/reader/api/0/edit-tag":
			edits++
			return responseWithJSON(http.StatusInternalServerError, `{"error":"boom"}`), nil
		}
		t.Fatalf("unexpected request %s", req.URL.String())
		return nil, nil
	})}

	if err := client.MarkEntryRead(context.Background(), "item", true); err == nil || edits != 1 {
		t.Fatalf("expected the first error and a single attempt, got err %v after %d attempts", err, edits)
	}
}

func TestUnreadCountsFallbackFollowsContinuationAndReportsTotalFailure(t *testing.T) {
	fail := false
	client := New("https://rss.example.com", "alice", "secret")
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/accounts/ClientLogin":
			return responseWithBody(http.StatusOK, "Auth=alice/token\n"), nil
		case "/reader/api/0/unread-count":
			return responseWithJSON(http.StatusOK, `[]`), nil
		case "/reader/api/0/subscription/list":
			return responseWithJSON(http.StatusOK, `{"subscriptions":[{"id":"feed/1","title":"A","url":"http://a/feed"}]}`), nil
		case "/reader/api/0/stream/items/ids":
			if fail {
				return responseWithJSON(http.StatusUnauthorized, `{}`), nil
			}
			if req.URL.Query().Get("c") == "" {
				return responseWithJSON(http.StatusOK, `{"itemRefs":[{"id":"1"},{"id":"2"}],"continuation":"next"}`), nil
			}
			return responseWithJSON(http.StatusOK, `{"itemRefs":[{"id":"3"}]}`), nil
		}
		t.Fatalf("unexpected request %s", req.URL.String())
		return nil, nil
	})}

	counts, err := client.UnreadCounts(context.Background())
	if err != nil || counts["feed/1"] != 3 {
		t.Fatalf("expected 3 unread across two pages, got %v, err %v", counts, err)
	}
	fail = true
	if _, err := client.UnreadCounts(context.Background()); err == nil {
		t.Fatal("expected an error when every feed fails")
	}
}
