package scraper

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

type DistilledPage struct {
	URL         string
	Title       string
	TextContent string
	Excerpt     string
}

type DocumentChunk struct {
	ID          string            `json:"id"`
	Content     string            `json:"content"`
	ChunkIndex  int               `json:"chunk_index"`
	TotalChunks int               `json:"total_chunks"`
	SourceURL   string            `json:"source_url"`
	Title       string            `json:"title"`
	Metadata    map[string]string `json:"metadata"`
}

// DistillHTML parses HTML using net/html tokenizer and extracts clean readable text.
func DistillHTML(rawHTML []byte, pageURL string) (*DistilledPage, error) {
	_, err := url.Parse(pageURL)
	if err != nil {
		pageURL = "http://localhost"
	}

	doc, err := html.Parse(bytes.NewReader(rawHTML))
	if err != nil {
		clean := cleanTextFallback(rawHTML)
		return &DistilledPage{
			URL:         pageURL,
			Title:       pageURL,
			TextContent: clean,
			Excerpt:     firstNChars(clean, 200),
		}, nil
	}

	var title string
	var textBuf strings.Builder

	var extractNode func(*html.Node)
	extractNode = func(n *html.Node) {
		if n.Type == html.ElementNode {
			// Skip script, style, noscript, nav, footer, header elements
			tag := strings.ToLower(n.Data)
			if tag == "script" || tag == "style" || tag == "noscript" || tag == "svg" || tag == "iframe" {
				return
			}
			if tag == "title" && n.FirstChild != nil {
				title = strings.TrimSpace(n.FirstChild.Data)
			}
		}

		if n.Type == html.TextNode {
			txt := strings.TrimSpace(n.Data)
			if len(txt) > 0 {
				textBuf.WriteString(txt)
				textBuf.WriteString(" ")
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractNode(c)
		}
	}

	extractNode(doc)

	cleanContent := strings.Join(strings.Fields(textBuf.String()), " ")
	if title == "" {
		title = pageURL
	}

	return &DistilledPage{
		URL:         pageURL,
		Title:       title,
		TextContent: cleanContent,
		Excerpt:     firstNChars(cleanContent, 200),
	}, nil
}

// ChunkDocument breaks a DistilledPage into overlapping text window chunks.
func ChunkDocument(page *DistilledPage, chunkSizeWords int, overlapWords int) []DocumentChunk {
	if chunkSizeWords <= 0 {
		chunkSizeWords = 500
	}
	if overlapWords < 0 || overlapWords >= chunkSizeWords {
		overlapWords = 100
	}

	words := strings.Fields(page.TextContent)
	if len(words) == 0 {
		return nil
	}

	step := chunkSizeWords - overlapWords
	if step <= 0 {
		step = 1
	}

	var rawChunks [][]string
	for i := 0; i < len(words); i += step {
		end := i + chunkSizeWords
		if end > len(words) {
			end = len(words)
		}
		chunkWords := words[i:end]
		rawChunks = append(rawChunks, chunkWords)
		if end == len(words) {
			break
		}
	}

	totalChunks := len(rawChunks)
	chunks := make([]DocumentChunk, totalChunks)

	for idx, cw := range rawChunks {
		chunkText := strings.Join(cw, " ")
		chunkID := fmt.Sprintf("%s#chunk-%d", page.URL, idx+1)
		chunks[idx] = DocumentChunk{
			ID:          chunkID,
			Content:     chunkText,
			ChunkIndex:  idx + 1,
			TotalChunks: totalChunks,
			SourceURL:   page.URL,
			Title:       page.Title,
			Metadata: map[string]string{
				"source":      page.URL,
				"title":       page.Title,
				"chunk_index": fmt.Sprintf("%d", idx+1),
				"total_chunks": fmt.Sprintf("%d", totalChunks),
			},
		}
	}

	return chunks
}

func cleanTextFallback(raw []byte) string {
	return strings.Join(strings.Fields(string(raw)), " ")
}

func firstNChars(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

