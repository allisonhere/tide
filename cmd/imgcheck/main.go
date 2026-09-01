// Command imgcheck is a diagnostic for Tide's article-image subsystem. It runs
// each stage in isolation (terminal detection, feed image extraction, HTTP
// fetch, decode, and an actual Kitty draw) and prints what happened, so a
// "no images" report can be pinned to a specific stage.
//
//	go run ./cmd/imgcheck [feed-url]
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/allisonhere/tide/internal/feed"
	tideimage "github.com/allisonhere/tide/internal/image"
)

func main() {
	url := "https://www.nasa.gov/rss/dyn/lg_image_of_the_day.rss"
	if len(os.Args) > 1 {
		url = os.Args[1]
	}

	fmt.Println("== 1. terminal detection ==")
	for _, k := range []string{"TERM", "TERM_PROGRAM", "KITTY_WINDOW_ID", "GHOSTTY_RESOURCES_DIR", "WEZTERM_EXECUTABLE", "KONSOLE_VERSION"} {
		fmt.Printf("  %-22s = %q\n", k, os.Getenv(k))
	}
	capEnv := tideimage.Detect(os.Getenv, false)
	fmt.Printf("  env-only   : supported=%v protocol=%q cell=%dx%d reason=%q\n",
		capEnv.Supported, capEnv.Protocol, capEnv.CellW, capEnv.CellH, capEnv.Reason)
	capProbe := tideimage.Detect(os.Getenv, true)
	fmt.Printf("  with probe : supported=%v protocol=%q cell=%dx%d reason=%q\n",
		capProbe.Supported, capProbe.Protocol, capProbe.CellW, capProbe.CellH, capProbe.Reason)

	fmt.Println("\n== 2. feed image extraction ==")
	parsed, _, err := feed.FetchAndParse(url)
	if err != nil {
		fmt.Printf("  FEED FETCH FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  feed: %q, %d items\n", parsed.Title, len(parsed.Items))
	var imgURL string
	for i, it := range parsed.Items {
		if i < 5 {
			fmt.Printf("  [%d] image_url=%q  (%s)\n", i, it.ImageURL, it.Title)
		}
		if imgURL == "" && it.ImageURL != "" {
			imgURL = it.ImageURL
		}
	}
	if imgURL == "" {
		fmt.Println("  NO ITEM YIELDED AN IMAGE URL — extraction is the problem.")
		os.Exit(1)
	}
	fmt.Printf("  -> first usable image: %s\n", imgURL)

	fmt.Println("\n== 3. fetch + decode ==")
	fetcher := tideimage.NewFetcher(tideimage.NewCache(""))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	data, ct, err := fetcher.Fetch(ctx, imgURL)
	if err != nil {
		fmt.Printf("  FETCH FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  downloaded %d bytes, content-type %q\n", len(data), ct)
	decoded, err := tideimage.Decode(data)
	if err != nil {
		fmt.Printf("  DECODE FAILED: %v\n", err)
		os.Exit(1)
	}
	b := decoded.Bounds()
	fmt.Printf("  decoded %dx%d\n", b.Dx(), b.Dy())

	fmt.Println("\n== 4. resize ==")
	cw, ch := capProbe.CellSize()
	resized := tideimage.Fit(decoded, 60*cw, tideimage.DefaultMaxRows*ch)
	rb := resized.Bounds()
	rows := tideimage.RowsFor(rb.Dy(), ch, tideimage.DefaultMaxRows)
	cols := tideimage.ColsFor(rb.Dx(), cw, 60)
	fmt.Printf("  cell size %dx%d -> resized %dx%d -> %d cols x %d rows\n", cw, ch, rb.Dx(), rb.Dy(), cols, rows)

	fmt.Println("\n== 5. real Kitty draw to stdout ==")
	if !capProbe.Supported {
		fmt.Println("  SKIPPED: terminal not detected as Kitty-capable (see stage 1).")
		return
	}
	r := tideimage.NewKittyRenderer()
	fmt.Printf("  drawing %d cols x %d rows at row 12, col 3 ... you should see the image below.\n", cols, rows)
	for i := 0; i < rows+2; i++ {
		fmt.Println()
	}
	// Frame() returns the transmit + placement escape string; write it directly.
	fmt.Print(r.Frame(resized, 3, 12, cols, rows))
	time.Sleep(4 * time.Second)
	fmt.Print(r.ClearAll())
	fmt.Println("\n  cleared. If you saw the image here but not in Tide, the bug is in the UI wiring.")
}
