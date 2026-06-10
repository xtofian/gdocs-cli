package gdocs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
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
	NewReply    string  `json:"new-reply,omitempty" yaml:"new-reply,omitempty"`
	Status      string  `json:"status,omitempty" yaml:"status,omitempty"`
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

// ParseCommentUpdates parses and validates comment updates from an io.Reader.
func ParseCommentUpdates(r io.Reader) ([]Comment, error) {
	var updates []Comment
	dec := json.NewDecoder(r)
	if err := dec.Decode(&updates); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	for i, u := range updates {
		if !u.IsDraft() && u.ID == "" {
			return nil, fmt.Errorf("validation error at index %d: comment thread ID ('id') is required for non-draft comments", i)
		}
	}

	return updates, nil
}

// UploadComments reads a JSON file of comments and appends them as replies to existing threads.
func UploadComments(ctx context.Context, httpClient *http.Client, docID string, jsonPath string) error {
	f, err := os.Open(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to open comments file %s: %w", jsonPath, err)
	}
	defer f.Close()

	updates, err := ParseCommentUpdates(f)
	if err != nil {
		return fmt.Errorf("failed to parse comments: %w", err)
	}

	// Filter out drafts first to see if we have anything to upload
	var activeUpdates []Comment
	for _, u := range updates {
		if u.IsDraft() {
			continue
		}
		activeUpdates = append(activeUpdates, u)
	}

	if len(activeUpdates) == 0 {
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

	srv, err := drive.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return fmt.Errorf("unable to create Drive service for uploading comments: %w", err)
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
				log.Printf("Comment already exists in thread %s, skipping.", u.ID)
				continue
			}
		}

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
