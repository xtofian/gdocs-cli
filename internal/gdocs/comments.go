package gdocs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// Comment represents a simplified Google Docs comment.
type Comment struct {
	ID          string // Drive API comment ID
	Anchor      string // Internal kix.* anchor ID
	Author      string
	Content     string
	QuotedText  string
	CreatedTime string
	Resolved    bool
	Replies     []Reply
}

// Reply represents a reply to a comment.
type Reply struct {
	Author      string
	Content     string
	CreatedTime string
}

// CommentUpdate represents a comment append request parsed from JSON.
type CommentUpdate struct {
	ID            string        `json:"id"`
	CommentThread []interface{} `json:"comment-thread,omitempty"`
	NewComment    string        `json:"new-comment,omitempty"`
	Status        string        `json:"status,omitempty"`
}

// IsDraft returns true if the update is marked as a draft or has no content.
func (u *CommentUpdate) IsDraft() bool {
	return u.Status == "draft" || u.NewComment == ""
}

// ParseCommentUpdates parses and validates comment updates from an io.Reader.
func ParseCommentUpdates(r io.Reader) ([]CommentUpdate, error) {
	var updates []CommentUpdate
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
	var activeUpdates []CommentUpdate
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
				if t == u.NewComment {
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
			Content: u.NewComment,
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
		call := srv.Comments.List(docID).Fields("comments(id,anchor,author(displayName),content,quotedFileContent,createdTime,resolved,replies(author(displayName),content,createdTime)),nextPageToken").PageSize(100).Context(ctx)
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
					CreatedTime: r.CreatedTime,
				}
				if r.Author != nil {
					reply.Author = r.Author.DisplayName
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
