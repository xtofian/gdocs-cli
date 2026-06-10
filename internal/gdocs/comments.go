package gdocs

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"gopkg.in/yaml.v3"
)

// Comment represents a simplified Google Docs comment thread.
type Comment struct {
	ID          string  `json:"id" yaml:"id"`
	Anchor      string  `json:"anchor,omitempty" yaml:"anchor,omitempty"`
	Author      string  `json:"author,omitempty" yaml:"author,omitempty"`
	AuthorEmail string  `json:"author-email,omitempty" yaml:"author-email,omitempty"`
	Content     string  `json:"comment" yaml:"comment"`
	QuotedText  string  `json:"quoted-text,omitempty" yaml:"quoted-text,omitempty"`
	CreatedTime string  `json:"created-time,omitempty" yaml:"created-time,omitempty"`
	Resolved    bool    `json:"resolved,omitempty" yaml:"resolved,omitempty"`
	Replies     []Reply `json:"replies,omitempty" yaml:"replies,omitempty"`
	NewReply    string  `json:"new-reply,omitempty" yaml:"new-reply"`
	Status      string  `json:"status,omitempty" yaml:"status"`
}

// Reply represents a reply to a comment.
type Reply struct {
	Author      string `json:"commenter" yaml:"commenter"`
	AuthorEmail string `json:"commenter-email,omitempty" yaml:"commenter-email,omitempty"`
	Content     string `json:"reply" yaml:"reply"`
	CreatedTime string `json:"date,omitempty" yaml:"date,omitempty"`
}

// IsDraft returns true if the update is marked as a draft or has no content.
func (u *Comment) IsDraft() bool {
	return u.Status == "draft" || u.NewReply == ""
}

// ParseCommentsFromFile reads a local file (Markdown or raw YAML), checks for matching frontmatter,
// and parses any YAML comment blocks contained within.
func ParseCommentsFromFile(filePath string, expectedDocID string) ([]Comment, error) {
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read comments file %s: %w", filePath, err)
	}

	contentStr := string(contentBytes)

	// 1. Check for YAML frontmatter
	if strings.HasPrefix(contentStr, "---") {
		// Find second "---"
		firstLineEnd := strings.Index(contentStr, "\n")
		if firstLineEnd != -1 {
			nextDash := strings.Index(contentStr[firstLineEnd+1:], "---")
			if nextDash != -1 {
				fmEnd := firstLineEnd + 1 + nextDash
				fmBlock := contentStr[firstLineEnd+1 : fmEnd]

				type fmStruct struct {
					GdocID string `yaml:"gdoc_id"`
				}
				var fm fmStruct
				if err := yaml.Unmarshal([]byte(fmBlock), &fm); err == nil {
					if fm.GdocID != "" && expectedDocID != "" && fm.GdocID != expectedDocID {
						return nil, fmt.Errorf("document ID mismatch: URL specifies '%s', but file frontmatter specifies '%s'", expectedDocID, fm.GdocID)
					}
				}
				// Skip past the closing "---" and any trailing newline
				postDash := fmEnd + 3
				if postDash < len(contentStr) && contentStr[postDash] == '\r' {
					postDash++
				}
				if postDash < len(contentStr) && contentStr[postDash] == '\n' {
					postDash++
				}
				contentStr = contentStr[postDash:]
			}
		}
	}

	// 2. Extract YAML comment blocks
	var updates []Comment
	hasEmbeddedComments := false

	startTag := "<!-- gdoc-comment-content:"
	endTag := "-->"

	searchPos := 0
	for {
		idx := strings.Index(contentStr[searchPos:], startTag)
		if idx < 0 {
			break
		}
		hasEmbeddedComments = true
		blockStart := searchPos + idx + len(startTag)

		endIdx := strings.Index(contentStr[blockStart:], endTag)
		if endIdx < 0 {
			return nil, fmt.Errorf("malformed comment block: missing closing '-->'")
		}
		blockEnd := blockStart + endIdx

		yamlBlock := contentStr[blockStart:blockEnd]

		var parsed []Comment
		if err := yaml.Unmarshal([]byte(yamlBlock), &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse YAML inside comment block: %w\nBlock content:\n%s", err, yamlBlock)
		}
		updates = append(updates, parsed...)

		searchPos = blockEnd + len(endTag)
	}

	// 3. Fallback to parsing entire file as raw YAML comments if no HTML comments are found
	if !hasEmbeddedComments {
		remaining := strings.TrimSpace(contentStr)
		if remaining != "" {
			var parsed []Comment
			if err := yaml.Unmarshal([]byte(remaining), &parsed); err != nil {
				return nil, fmt.Errorf("failed to parse file as raw YAML comments: %w", err)
			}
			updates = append(updates, parsed...)
		}
	}

	// 4. Validate parsed comments
	for i, u := range updates {
		if !u.IsDraft() && u.ID == "" {
			return nil, fmt.Errorf("validation error at index %d: comment thread ID ('id') is required for non-draft comments", i)
		}
	}

	return updates, nil
}

// UploadComments takes parsed comments, reconciles them with existing threads on the server,
// and uploads them. If dryRun is true, it only prints what would be sent.
func UploadComments(ctx context.Context, httpClient *http.Client, docID string, updates []Comment, dryRun bool) error {
	// Filter out drafts first to see if we have anything to upload
	var activeUpdates []Comment
	for _, u := range updates {
		if u.IsDraft() {
			continue
		}
		activeUpdates = append(activeUpdates, u)
	}

	if len(activeUpdates) == 0 {
		log.Println("No active non-draft comments to upload.")
		return nil
	}

	// Fetch existing comments to support idempotence checking
	log.Println("Fetching existing comments to check for duplicates...")
	existingComments, err := FetchComments(ctx, httpClient, docID)
	if err != nil {
		return fmt.Errorf("failed to fetch existing comments for idempotency check: %w", err)
	}

	commentTexts := make(map[string][]string)
	for _, c := range existingComments {
		texts := []string{c.Content}
		for _, r := range c.Replies {
			texts = append(texts, r.Content)
		}
		commentTexts[c.ID] = texts
	}

	var srv *drive.Service
	if !dryRun {
		srv, err = drive.NewService(ctx, option.WithHTTPClient(httpClient))
		if err != nil {
			return fmt.Errorf("unable to create Drive service for uploading comments: %w", err)
		}
	}

	for _, u := range activeUpdates {
		// Check for duplicates
		if texts, exists := commentTexts[u.ID]; exists {
			alreadyPresent := false
			for _, t := range texts {
				if t == u.NewReply {
					alreadyPresent = true
					break
				}
			}
			if alreadyPresent {
				if dryRun {
					fmt.Printf("[Dry-run] Comment already exists in thread %s, would skip: %q\n", u.ID, u.NewReply)
				} else {
					log.Printf("Comment already exists in thread %s, skipping.", u.ID)
				}
				continue
			}
		}

		if dryRun {
			fmt.Printf("[Dry-run] Would upload reply to thread %s: %q\n", u.ID, u.NewReply)
		} else {
			log.Printf("Uploading comment to thread %s...", u.ID)
			replyBody := &drive.Reply{
				Content: u.NewReply,
			}
			_, err := srv.Replies.Create(docID, u.ID, replyBody).Fields("id").Context(ctx).Do()
			if err != nil {
				return fmt.Errorf("failed to append comment to thread %s: %w", u.ID, err)
			}
			log.Printf("Successfully uploaded comment to thread %s", u.ID)
		}
	}

	return nil
}

// FetchComments retrieves all comments for a document using the Drive API.
func FetchComments(ctx context.Context, httpClient *http.Client, docID string) ([]Comment, error) {
	srv, err := drive.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("unable to create Drive service: %w", err)
	}

	var comments []Comment
	pageToken := ""
	for {
		call := srv.Comments.List(docID).Fields("comments(id,anchor,author(displayName,emailAddress),content,quotedFileContent,createdTime,resolved,replies(author(displayName,emailAddress),content,createdTime,deleted)),nextPageToken").PageSize(100).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("unable to retrieve comments: %w", err)
		}

		for _, c := range resp.Comments {
			if c.Deleted {
				continue
			}
			comment := Comment{
				ID:          c.Id,
				Anchor:      c.Anchor,
				Content:     c.Content,
				CreatedTime: c.CreatedTime,
				Resolved:    c.Resolved,
			}
			if c.Author != nil {
				comment.Author = c.Author.DisplayName
				comment.AuthorEmail = c.Author.EmailAddress
				if c.Author.EmailAddress != "" {
					log.Printf("Fetched author email for comment %s: %s", c.Id, c.Author.EmailAddress)
				}
			}
			if c.QuotedFileContent != nil {
				comment.QuotedText = c.QuotedFileContent.Value
			}
			for _, r := range c.Replies {
				if r.Deleted {
					continue
				}
				reply := Reply{
					Content:     r.Content,
					CreatedTime: formatDate(r.CreatedTime),
				}
				if r.Author != nil {
					reply.Author = r.Author.DisplayName
					reply.AuthorEmail = r.Author.EmailAddress
					if r.Author.EmailAddress != "" {
						log.Printf("Fetched reply author email for comment %s: %s", c.Id, r.Author.EmailAddress)
					}
				}
				comment.Replies = append(comment.Replies, reply)
			}
			comments = append(comments, comment)
		}

		pageToken = resp.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return comments, nil
}

// FetchMobileBasicHTML retrieves the mobilebasic HTML view of a document.
func FetchMobileBasicHTML(ctx context.Context, httpClient *http.Client, docID string) (string, error) {
	url := fmt.Sprintf("https://docs.google.com/document/d/%s/mobilebasic", docID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch mobilebasic HTML: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch mobilebasic HTML: status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(bodyBytes), nil
}

// formatDate converts an RFC 3339 timestamp to a short date string (YYYY-MM-DD).
func formatDate(rfc3339 string) string {
	if rfc3339 == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02")
}
