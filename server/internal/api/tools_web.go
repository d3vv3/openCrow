// tools_web.go — Web search and URL fetching tool implementations.
package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ── web_search ───────────────────────────────────────────────────────────

func (s *Server) toolWebSearch(ctx context.Context, args map[string]any) (map[string]any, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return map[string]any{"success": false, "error": "query is required"}, nil
	}

	// Scrape DuckDuckGo Lite (no API key needed)
	url := "https://lite.duckduckgo.com/lite/?q=" + strings.ReplaceAll(query, " ", "+")
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; openCrow/1.0)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("search failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return map[string]any{"success": false, "error": "failed to read results"}, nil
	}

	// Parse the DuckDuckGo Lite HTML for results
	results := parseDuckDuckGoLite(string(body))

	return map[string]any{
		"success": true,
		"query":   query,
		"results": results,
	}, nil
}

// parseDuckDuckGoLite extracts search results from DDG Lite HTML.
func parseDuckDuckGoLite(html string) []map[string]string {
	var results []map[string]string

	// DDG Lite uses a table-based layout. Links are in <a> tags with class "result-link"
	// or just plain <a> tags in result rows. We do a simple extraction.
	lines := strings.Split(html, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		// Look for result links
		if !strings.Contains(line, "result-link") && !strings.Contains(line, "result__a") {
			continue
		}

		// Extract href
		hrefIdx := strings.Index(line, "href=\"")
		if hrefIdx == -1 {
			continue
		}
		hrefStart := hrefIdx + 6
		hrefEnd := strings.Index(line[hrefStart:], "\"")
		if hrefEnd == -1 {
			continue
		}
		href := line[hrefStart : hrefStart+hrefEnd]

		// Extract link text (title)
		title := extractTextContent(line)
		if title == "" {
			continue
		}

		// Look for snippet in nearby lines
		snippet := ""
		for j := i + 1; j < len(lines) && j < i+5; j++ {
			l := strings.TrimSpace(lines[j])
			if strings.Contains(l, "result-snippet") || strings.Contains(l, "result__snippet") {
				snippet = extractTextContent(l)
				break
			}
		}

		results = append(results, map[string]string{
			"title":   title,
			"url":     href,
			"snippet": snippet,
		})

		if len(results) >= 10 {
			break
		}
	}

	return results
}

// extractTextContent strips HTML tags from a string.
func extractTextContent(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// ── open_url ─────────────────────────────────────────────────────────────

func (s *Server) toolOpenURL(ctx context.Context, args map[string]any) (map[string]any, error) {
	rawURL, _ := args["url"].(string)
	if rawURL == "" {
		return map[string]any{"success": false, "error": "url is required"}, nil
	}

	if err := checkSSRF(ctx, rawURL); err != nil {
		return map[string]any{"success": false, "error": "URL not allowed: " + err.Error()}, nil
	}

	// Fetch page content and return it (server-side, we fetch instead of opening browser)
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; openCrow/1.0)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("fetch failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return map[string]any{"success": false, "error": "failed to read page"}, nil
	}

	content := extractTextContent(string(body))
	if len(content) > 20000 {
		content = content[:20000] + "..."
	}

	return map[string]any{
		"success":      true,
		"url":          rawURL,
		"status_code":  resp.StatusCode,
		"content_type": resp.Header.Get("Content-Type"),
		"content":      content,
	}, nil
}
