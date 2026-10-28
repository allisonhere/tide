package greader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client speaks the Google Reader-compatible API used by FreshRSS and similar servers. -allie
type Client struct {
	BaseURL    string
	Login      string
	Password   string
	HTTPClient *http.Client

	authToken string
	csrfToken string
}

// Subscription is the remote feed shape Tide imports into its local sidebar model. -allie
type Subscription struct {
	ID       string
	Title    string
	FeedURL  string
	HTMLURL  string
	Category string
}

// Entry is a remote article after Reader API fields are normalized for Tide. -allie
type Entry struct {
	ID          string
	StreamID    string
	Title       string
	Link        string
	ContentHTML string
	PublishedAt time.Time
	UpdatedAt   time.Time
	Read        bool
	Author      string
	Categories  []string
}

// QuickAddResult captures the stream identity returned when a server subscribes to a feed URL. -allie
type QuickAddResult struct {
	Query      string
	StreamID   string
	StreamName string
}

func New(baseURL, login, password string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Login:      strings.TrimSpace(login),
		Password:   password,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

const (
	readTag       = "user/-/state/com.google/read"
	keptUnreadTag = "user/-/state/com.google/kept-unread"
)

func (c *Client) QuickAdd(ctx context.Context, feedURL string) (QuickAddResult, error) {
	feedURL = strings.TrimSpace(feedURL)
	if feedURL == "" {
		return QuickAddResult{}, fmt.Errorf("feed URL is required")
	}

	var resp struct {
		NumResults int    `json:"numResults"`
		Query      string `json:"query"`
		StreamID   string `json:"streamId"`
		StreamName string `json:"streamName"`
		Error      string `json:"error"`
	}
	if err := c.postFormJSON(ctx, "/reader/api/0/subscription/quickadd", url.Values{
		"quickadd": []string{feedURL},
	}, &resp); err != nil {
		return QuickAddResult{}, err
	}
	if resp.NumResults < 1 || strings.TrimSpace(resp.StreamID) == "" {
		if strings.TrimSpace(resp.Error) != "" {
			return QuickAddResult{}, fmt.Errorf("quickadd failed: %s", strings.TrimSpace(resp.Error))
		}
		return QuickAddResult{}, fmt.Errorf("quickadd failed for %s", feedURL)
	}
	return QuickAddResult{
		Query:      UnescapeAPIString(resp.Query),
		StreamID:   strings.TrimSpace(resp.StreamID),
		StreamName: UnescapeAPIString(resp.StreamName),
	}, nil
}

func (c *Client) MarkEntryRead(ctx context.Context, itemID string, read bool) error {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return fmt.Errorf("item ID is required")
	}

	form := url.Values{
		"i": []string{itemID},
	}
	if !read {
		form["r"] = []string{readTag}
		return c.postFormText(ctx, "/reader/api/0/edit-tag", form, true)
	}
	form["a"] = []string{readTag}
	form["r"] = []string{keptUnreadTag}
	err := c.postFormText(ctx, "/reader/api/0/edit-tag", form, true)
	// Miniflux refuses to add read and remove kept-unread together ("should
	// not be supplied simultaneously"); adding read alone says the same. Any
	// other failure is returned as is.
	if err != nil && strings.Contains(err.Error(), "simultaneously") {
		if err2 := c.postFormText(ctx, "/reader/api/0/edit-tag", url.Values{
			"i": []string{itemID},
			"a": []string{readTag},
		}, true); err2 != nil {
			return err2
		}
		return nil
	}
	return err
}

func (c *Client) MarkAllRead(ctx context.Context, streamID string) error {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return fmt.Errorf("stream ID is required")
	}
	return c.postFormText(ctx, "/reader/api/0/mark-all-as-read", url.Values{
		"s": []string{streamID},
	}, true)
}

func (c *Client) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	var resp struct {
		Subscriptions []struct {
			ID         string `json:"id"`
			Title      string `json:"title"`
			URL        string `json:"url"`
			HTMLURL    string `json:"htmlUrl"`
			Categories []struct {
				ID    string `json:"id"`
				Label string `json:"label"`
			} `json:"categories"`
		} `json:"subscriptions"`
	}
	if err := c.getJSON(ctx, "/reader/api/0/subscription/list", url.Values{"output": []string{"json"}}, &resp); err != nil {
		return nil, err
	}

	subs := make([]Subscription, 0, len(resp.Subscriptions))
	for _, sub := range resp.Subscriptions {
		category := ""
		for _, cat := range sub.Categories {
			if cat.Label != "" {
				category = cat.Label
				break
			}
			if cat.ID != "" {
				category = cat.ID
				break
			}
		}

		feedURL := strings.TrimSpace(sub.URL)
		if feedURL == "" && strings.HasPrefix(sub.ID, "feed/") {
			feedURL = strings.TrimPrefix(sub.ID, "feed/")
		}

		subs = append(subs, Subscription{
			ID:       sub.ID,
			Title:    UnescapeAPIString(sub.Title),
			FeedURL:  feedURL,
			HTMLURL:  sub.HTMLURL,
			Category: UnescapeAPIString(category),
		})
	}
	return subs, nil
}

func (c *Client) UnreadCounts(ctx context.Context) (map[string]int64, error) {
	var raw json.RawMessage
	if err := c.getJSON(ctx, "/reader/api/0/unread-count", url.Values{"output": []string{"json"}}, &raw); err != nil {
		return nil, err
	}
	// Miniflux has no unread-count: its catch-all for endpoints it lacks
	// answers with an empty JSON array. Count the unread items feed by feed.
	if b := bytes.TrimSpace(raw); len(b) > 0 && b[0] == '[' {
		return c.countUnreadByFeed(ctx)
	}
	var resp struct {
		UnreadCounts []struct {
			ID    string `json:"id"`
			Count any    `json:"count"`
		} `json:"unreadcounts"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode /reader/api/0/unread-count: %w", err)
	}

	counts := make(map[string]int64, len(resp.UnreadCounts))
	for _, unread := range resp.UnreadCounts {
		counts[unread.ID] = toInt64(unread.Count)
	}
	return counts, nil
}

// countUnreadByFeed counts each feed's unread items from the ids of its items,
// for servers without unread-count. A feed whose ids can't be had counts as
// none, so one failing feed doesn't keep the rest from loading.
func (c *Client) countUnreadByFeed(ctx context.Context) (map[string]int64, error) {
	subs, err := c.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(subs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	var failed int
	var firstErr error
	sem := make(chan struct{}, 8)
	for _, sub := range subs {
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer func() { <-sem; wg.Done() }()
			n, err := c.countUnreadIDs(ctx, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			if n > 0 {
				counts[id] = n
			}
		}(sub.ID)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return counts, err
	}
	// Individual failures count as none, but if every feed failed (a bad
	// token, a dead server) say so instead of showing zeros everywhere.
	if len(subs) > 0 && failed == len(subs) {
		return nil, firstErr
	}
	return counts, nil
}

// unreadIDPages bounds how many pages of ids countUnreadIDs follows.
const unreadIDPages = 10

// countUnreadIDs counts a stream's unread items by paging through their ids.
func (c *Client) countUnreadIDs(ctx context.Context, streamID string) (int64, error) {
	var total int64
	continuation := ""
	for page := 0; page < unreadIDPages; page++ {
		var resp struct {
			ItemRefs []struct {
				ID string `json:"id"`
			} `json:"itemRefs"`
			Continuation string `json:"continuation"`
		}
		query := url.Values{
			"s":      []string{streamID},
			"xt":     []string{readTag},
			"n":      []string{"1000"},
			"output": []string{"json"},
		}
		if continuation != "" {
			query.Set("c", continuation)
		}
		if err := c.getJSON(ctx, "/reader/api/0/stream/items/ids", query, &resp); err != nil {
			return total, err
		}
		total += int64(len(resp.ItemRefs))
		if resp.Continuation == "" || len(resp.ItemRefs) == 0 {
			break
		}
		continuation = resp.Continuation
	}
	return total, nil
}

// streamContents is a reply with articles in it.
type streamContents struct {
	Items []struct {
		ID         string   `json:"id"`
		Title      string   `json:"title"`
		Author     string   `json:"author"`
		Published  int64    `json:"published"`
		Updated    int64    `json:"updated"`
		Categories []string `json:"categories"`
		Alternate  []struct {
			Href string `json:"href"`
		} `json:"alternate"`
		Canonical []struct {
			Href string `json:"href"`
		} `json:"canonical"`
		Summary *struct {
			Content string `json:"content"`
		} `json:"summary"`
		Content *struct {
			Content string `json:"content"`
		} `json:"content"`
		Origin struct {
			StreamID string `json:"streamId"`
		} `json:"origin"`
	} `json:"items"`
}

func (c *Client) StreamContents(ctx context.Context, streamID string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 100
	}

	q := url.Values{
		"output": []string{"json"},
		"n":      []string{strconv.Itoa(limit)},
		"xt":     []string{readTag},
	}
	var raw json.RawMessage
	if err := c.getJSON(ctx, "/reader/api/0/stream/contents/"+url.PathEscape(streamID), q, &raw); err != nil {
		return nil, err
	}
	var resp streamContents
	if b := bytes.TrimSpace(raw); len(b) > 0 && b[0] == '[' {
		// Miniflux has no stream/contents: its catch-all answers with an empty
		// JSON array. It does have the ids of a stream's items, and their contents.
		var err error
		if resp, err = c.itemContents(ctx, streamID, limit); err != nil {
			return nil, err
		}
	} else if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode /reader/api/0/stream/contents: %w", err)
	}

	entries := make([]Entry, 0, len(resp.Items))
	for _, item := range resp.Items {
		contentHTML := ""
		if item.Content != nil && item.Content.Content != "" {
			contentHTML = item.Content.Content
		} else if item.Summary != nil {
			contentHTML = item.Summary.Content
		}

		link := ""
		if len(item.Canonical) > 0 && item.Canonical[0].Href != "" {
			link = item.Canonical[0].Href
		} else if len(item.Alternate) > 0 {
			link = item.Alternate[0].Href
		}

		published := item.Published
		if published == 0 {
			published = item.Updated
		}
		var updatedAt time.Time
		if item.Updated > 0 {
			updatedAt = time.Unix(item.Updated, 0)
		}

		entryStreamID := item.Origin.StreamID
		if entryStreamID == "" {
			entryStreamID = streamID
		}

		entries = append(entries, Entry{
			ID:          item.ID,
			StreamID:    entryStreamID,
			Title:       UnescapeAPIString(item.Title),
			Link:        link,
			ContentHTML: contentHTML,
			PublishedAt: time.Unix(published, 0),
			UpdatedAt:   updatedAt,
			Read:        hasReadState(item.Categories),
			Author:      UnescapeAPIString(strings.TrimSpace(item.Author)),
			Categories:  labelCategories(item.Categories),
		})
	}
	return entries, nil
}

// itemContents gets a stream's unread articles in two steps, for servers without
// stream/contents: the ids of its items, then the contents of those items.
func (c *Client) itemContents(ctx context.Context, streamID string, limit int) (streamContents, error) {
	var out streamContents
	var ids struct {
		ItemRefs []struct {
			ID string `json:"id"`
		} `json:"itemRefs"`
	}
	q := url.Values{
		"s":      []string{streamID},
		"xt":     []string{readTag},
		"n":      []string{strconv.Itoa(limit)},
		"output": []string{"json"},
	}
	if err := c.getJSON(ctx, "/reader/api/0/stream/items/ids", q, &ids); err != nil {
		return out, err
	}
	if len(ids.ItemRefs) == 0 {
		return out, nil
	}
	token, err := c.ensureToken(ctx)
	if err != nil {
		return out, err
	}
	form := url.Values{"T": []string{token}, "output": []string{"json"}}
	for _, ref := range ids.ItemRefs {
		form.Add("i", ref.ID)
	}
	err = c.postFormJSON(ctx, "/reader/api/0/stream/items/contents", form, &out)
	return out, err
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, dst any) error {
	if err := c.ensureAuth(ctx); err != nil {
		return err
	}

	u := c.BaseURL + path
	if encoded := query.Encode(); encoded != "" {
		u += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "GoogleLogin auth="+c.authToken)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("greader %s failed: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func (c *Client) postFormJSON(ctx context.Context, path string, form url.Values, dst any) error {
	if err := c.ensureAuth(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "GoogleLogin auth="+c.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("greader %s failed: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func (c *Client) postFormText(ctx context.Context, path string, form url.Values, withToken bool) error {
	if err := c.ensureAuth(ctx); err != nil {
		return err
	}
	if withToken {
		token, err := c.ensureToken(ctx)
		if err != nil {
			return err
		}
		form = cloneValues(form)
		form.Set("T", token)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "GoogleLogin auth="+c.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("read %s response: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("greader %s failed: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *Client) ensureAuth(ctx context.Context) error {
	if c.authToken != "" {
		return nil
	}
	if c.BaseURL == "" || c.Login == "" || c.Password == "" {
		return fmt.Errorf("google reader source not configured: set API URL, login, and password")
	}

	form := url.Values{
		"Email":  []string{c.Login},
		"Passwd": []string{c.Password},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/accounts/ClientLogin", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return fmt.Errorf("read login response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ClientLogin failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(line, "Auth="); ok {
			c.authToken = strings.TrimSpace(value)
			c.csrfToken = ""
			break
		}
	}
	if c.authToken == "" {
		return fmt.Errorf("ClientLogin response missing Auth token")
	}
	return nil
}

func (c *Client) ensureToken(ctx context.Context) (string, error) {
	if c.csrfToken != "" {
		return c.csrfToken, nil
	}
	if err := c.ensureAuth(ctx); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/reader/api/0/token", nil)
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Authorization", "GoogleLogin auth="+c.authToken)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("greader /reader/api/0/token failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	c.csrfToken = strings.TrimSpace(string(body))
	if c.csrfToken == "" {
		return "", fmt.Errorf("greader token response was empty")
	}
	return c.csrfToken, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// UnescapeAPIString decodes HTML entities in text from the Reader API (e.g. &#039; → ').
// It runs multiple passes so double-encoded values (e.g. &amp;#039;) become a single apostrophe.
// A final literal pass catches refs that still appear verbatim in some JSON/XML pipelines.
func UnescapeAPIString(s string) string {
	s = strings.TrimSpace(s)
	for range 8 {
		next := strings.TrimSpace(html.UnescapeString(s))
		if next == s {
			break
		}
		s = next
	}
	for _, p := range literalEntityFixes {
		s = strings.ReplaceAll(s, p.from, p.to)
	}
	return s
}

// literalEntityFixes handles apostrophe-like sequences that can remain after html.UnescapeString
// (e.g. mixed encodings) or appear without semicolons in upstream data.
var literalEntityFixes = []struct{ from, to string }{
	{"&#039;", "'"},
	{"&#39;", "'"},
	{"&#x27;", "'"},
	{"&#X27;", "'"},
	{"&apos;", "'"},
	{"&#039", "'"},
	{"&#39", "'"},
}

func hasReadState(categories []string) bool {
	for _, category := range categories {
		if category == "user/-/state/com.google/read" {
			return true
		}
	}
	return false
}

// labelCategories pulls user tag labels ("user/-/label/Tech" -> "Tech") out of
// the Reader category list, ignoring the "user/-/state/..." control entries.
func labelCategories(categories []string) []string {
	var out []string
	for _, category := range categories {
		const prefix = "user/-/label/"
		if !strings.HasPrefix(category, prefix) {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(category, prefix))
		if name != "" {
			out = append(out, UnescapeAPIString(name))
		}
	}
	return out
}

func cloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for key, values := range v {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	case json.Number:
		if parsed, err := n.Int64(); err == nil {
			return parsed
		}
	case string:
		if parsed, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64); err == nil {
			return parsed
		}
	}
	return 0
}
