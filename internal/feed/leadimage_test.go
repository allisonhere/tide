package feed

import (
	"testing"

	"github.com/mmcdole/gofeed"
	ext "github.com/mmcdole/gofeed/extensions"
)

func mediaExt(name string, attrs map[string]string) ext.Extension {
	return ext.Extension{Name: name, Attrs: attrs}
}

func TestLeadImageURL_Priority(t *testing.T) {
	tests := []struct {
		name string
		item *gofeed.Item
		want string
	}{
		{
			name: "media:content beats everything",
			item: &gofeed.Item{
				Content: `<p><img src="https://example.com/body.jpg"></p>`,
				Image:   &gofeed.Image{URL: "https://example.com/item-image.jpg"},
				Enclosures: []*gofeed.Enclosure{
					{URL: "https://example.com/enc.jpg", Type: "image/jpeg"},
				},
				Extensions: ext.Extensions{
					"media": {
						"content": []ext.Extension{
							mediaExt("content", map[string]string{"url": "https://example.com/media.jpg", "medium": "image", "width": "600"}),
						},
					},
				},
			},
			want: "https://example.com/media.jpg",
		},
		{
			name: "largest media:content width wins",
			item: &gofeed.Item{
				Extensions: ext.Extensions{
					"media": {
						"content": []ext.Extension{
							mediaExt("content", map[string]string{"url": "https://example.com/small.jpg", "medium": "image", "width": "200"}),
							mediaExt("content", map[string]string{"url": "https://example.com/big.jpg", "medium": "image", "width": "1600"}),
							mediaExt("content", map[string]string{"url": "https://example.com/mid.jpg", "medium": "image", "width": "800"}),
						},
					},
				},
			},
			want: "https://example.com/big.jpg",
		},
		{
			name: "media:content inside media:group",
			item: &gofeed.Item{
				Extensions: ext.Extensions{
					"media": {
						"group": []ext.Extension{
							{
								Name: "group",
								Children: map[string][]ext.Extension{
									"content": {
										mediaExt("content", map[string]string{"url": "https://example.com/grouped.png", "type": "image/png", "width": "900"}),
									},
								},
							},
						},
					},
				},
			},
			want: "https://example.com/grouped.png",
		},
		{
			name: "media:thumbnail when no content",
			item: &gofeed.Item{
				Extensions: ext.Extensions{
					"media": {
						"thumbnail": []ext.Extension{
							mediaExt("thumbnail", map[string]string{"url": "https://example.com/thumb.jpg"}),
						},
					},
				},
			},
			want: "https://example.com/thumb.jpg",
		},
		{
			name: "enclosure image when no media ext",
			item: &gofeed.Item{
				Enclosures: []*gofeed.Enclosure{
					{URL: "https://example.com/podcast.mp3", Type: "audio/mpeg"},
					{URL: "https://example.com/enc.jpg", Type: "image/jpeg"},
				},
			},
			want: "https://example.com/enc.jpg",
		},
		{
			name: "item.Image before body scan",
			item: &gofeed.Item{
				Image:   &gofeed.Image{URL: "https://example.com/item-image.jpg"},
				Content: `<img src="https://example.com/body.jpg">`,
			},
			want: "https://example.com/item-image.jpg",
		},
		{
			name: "first body img as last resort",
			item: &gofeed.Item{
				Content: `<p>hi</p><img src="https://example.com/hero.jpg" width="800">`,
			},
			want: "https://example.com/hero.jpg",
		},
		{
			name: "description used when content empty",
			item: &gofeed.Item{
				Description: `<img src="https://example.com/desc.jpg">`,
			},
			want: "https://example.com/desc.jpg",
		},
		{
			name: "nothing usable",
			item: &gofeed.Item{Content: "<p>just text</p>"},
			want: "",
		},
		{
			name: "protocol-relative url is normalised",
			item: &gofeed.Item{Content: `<img src="//cdn.example.com/x.jpg" width="500">`},
			want: "https://cdn.example.com/x.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := leadImageURL(tt.item); got != tt.want {
				t.Fatalf("leadImageURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLeadImageURL_JunkFiltered(t *testing.T) {
	junkBodies := []string{
		`<img src="https://example.com/pixel.gif">`,
		`<img src="https://example.com/1x1.png">`,
		`<img src="https://track.example.com/tracking?id=1">`,
		`<img src="https://example.com/spacer.gif">`,
		`<img src="https://www.gravatar.com/avatar/abc">`,
		`<img src="https://example.com/assets/icons/rss.png">`,
		`<img src="https://example.com/logo.svg">`,
		`<img src="data:image/png;base64,iVBORw0KGgo=">`,
		`<img src="https://example.com/hero.jpg" width="1" height="1">`,
		`<img src="https://example.com/tiny.jpg" width="8">`,
	}
	for _, body := range junkBodies {
		t.Run(body, func(t *testing.T) {
			if got := leadImageURL(&gofeed.Item{Content: body}); got != "" {
				t.Fatalf("expected junk image to be filtered, got %q", got)
			}
		})
	}
}

func TestLeadImageURL_JunkThenRealImage(t *testing.T) {
	item := &gofeed.Item{Content: `
		<img src="https://track.example.com/beacon.gif" width="1" height="1">
		<p>intro</p>
		<img src="https://example.com/real-hero.jpg" width="1200">
	`}
	if got := leadImageURL(item); got != "https://example.com/real-hero.jpg" {
		t.Fatalf("leadImageURL() = %q, want the real hero image", got)
	}
}

func TestIsJunkImageURL(t *testing.T) {
	junk := []string{
		"data:image/gif;base64,R0lGOD",
		"https://example.com/logo.svg",
		"https://example.com/img/spacer.gif",
		"https://example.com/1x1.png",
		"https://feeds.feedburner.com/~ff/pixel",
		"https://example.com/social-icon.png",
		"https://example.com/favicons/x.ico",
	}
	for _, u := range junk {
		if !isJunkImageURL(u) {
			t.Errorf("isJunkImageURL(%q) = false, want true", u)
		}
	}
	ok := []string{
		"https://example.com/2024/09/hero-photo.jpg",
		"https://cdn.example.com/media/story.png?w=1200",
		"https://example.com/uploads/large_image.webp",
	}
	for _, u := range ok {
		if isJunkImageURL(u) {
			t.Errorf("isJunkImageURL(%q) = true, want false", u)
		}
	}
}

func TestLeadImageFromHTML(t *testing.T) {
	if got := LeadImageFromHTML(`<p>x</p><img src="https://example.com/a.jpg" width="700">`); got != "https://example.com/a.jpg" {
		t.Fatalf("LeadImageFromHTML() = %q", got)
	}
	if got := LeadImageFromHTML(`<p>no images here</p>`); got != "" {
		t.Fatalf("LeadImageFromHTML() = %q, want empty", got)
	}
}
