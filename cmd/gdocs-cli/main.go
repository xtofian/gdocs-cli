package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/famasya/gdocs-cli/internal/auth"
	"github.com/famasya/gdocs-cli/internal/gdocs"
	"github.com/famasya/gdocs-cli/internal/markdown"
)

//go:embed instruction.txt
var instructionText string

// commentsMode is a flag value that accepts bare --comments (meaning "all")
// or --comments=open / --comments=all.
type commentsMode string

func (c *commentsMode) String() string { return string(*c) }

func (c *commentsMode) Set(s string) error {
	switch s {
	case "true", "all": // "true" is what flag sends for a bare boolean-style flag
		*c = "all"
	case "open":
		*c = "open"
	default:
		return fmt.Errorf("must be 'all' or 'open'")
	}
	return nil
}

// IsBoolFlag allows --comments without a value (treated as --comments=all).
func (c *commentsMode) IsBoolFlag() bool { return true }

func main() {
	// Define flags
	urlFlag := flag.String("url", "", "Google Docs URL (required for normal operation)")
	configFlag := flag.String("config", "", "Path to OAuth credentials JSON file (defaults to ~/.config/gdocs-cli/config.json)")
	accessTokenFlag := flag.String("access_token", "", "Path to OAuth access token JSON file (bypasses normal OAuth flow)")
	initFlag := flag.Bool("init", false, "Initialize OAuth and save token to default location")
	cleanFlag := flag.Bool("clean", false, "Clean output (suppress all logs, only output markdown)")
	outputJSONFlag := flag.Bool("output-json", false, "Emit the exact structure of the doc received from the Docs API as pretty-printed JSON")
	var comments commentsMode
	flag.Var(&comments, "comments", "Include comments: --comments (all) or --comments=open (unresolved only)")
	commentsSkipOlderThanFlag := flag.Int("comments-skip-older-than", 0, "Omit comments where the last update is older than the specified number of days")
	uploadCommentsFlag := flag.Bool("upload-comments", false, "Upload comment replies from the file specified by --file")
	fileFlag := flag.String("file", "", "Path to local markdown/YAML file containing comment blocks for uploading")
	dryRunFlag := flag.Bool("dry-run", false, "Simulate comment uploading and print reconciled updates without saving")
	instructionFlag := flag.Bool("instruction", false, "Print integration instructions for AI coding agents")
	flag.Parse()

	// Handle instruction mode - print instructions and exit
	if *instructionFlag {
		fmt.Print(instructionText)
		return
	}

	// Handle clean mode - suppress all logs
	if *cleanFlag {
		log.SetOutput(io.Discard)
	}

	// If uploading comments, --file is required
	if *uploadCommentsFlag && *fileFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: --file flag is required when using --upload-comments")
		os.Exit(1)
	}

	// Determine config path (use default if not specified), unless bypassed by --access_token
	configPath := ""
	if *accessTokenFlag == "" {
		configPath = *configFlag
		if configPath == "" {
			defaultPath, err := getDefaultConfigPath()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			configPath = defaultPath
		}
	}

	// Handle init mode (incompatible with --access_token)
	if *initFlag {
		if *accessTokenFlag != "" {
			fmt.Fprintln(os.Stderr, "Error: --init and --access_token are mutually exclusive")
			os.Exit(1)
		}
		if err := initAuth(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Validate flags for normal operation
	if *urlFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: --url flag is required")
		fmt.Fprintln(os.Stderr)
		flag.Usage()
		os.Exit(1)
	}

	// Run the main logic
	if err := run(*urlFlag, configPath, *accessTokenFlag, *fileFlag, *uploadCommentsFlag, *dryRunFlag, *commentsSkipOlderThanFlag, comments, *outputJSONFlag); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run executes the main logic of the CLI.
// It handles authentication, document fetching, and markdown conversion.
// If accessTokenPath is non-empty it is used directly (bypassing credPath and the OAuth flow).
func run(docURL, credPath, accessTokenPath, filePath string, uploadComments, dryRun bool, commentsSkipOlderThan int, comments commentsMode, outputJSON bool) error {
	ctx := context.Background()

	// Extract document ID from URL
	docID, err := gdocs.ExtractDocumentID(docURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Extract tab ID from URL (may be empty)
	tabID := gdocs.ExtractTabID(docURL)

	// Get authenticated HTTP client
	var httpClient *http.Client
	if accessTokenPath != "" {
		httpClient, err = auth.GetClientFromTokenFile(ctx, accessTokenPath)
		if err != nil {
			return fmt.Errorf("access token error: %w", err)
		}
	} else {
		authenticator, err := auth.NewAuthenticator(credPath)
		if err != nil {
			return fmt.Errorf("authentication setup failed: %w", err)
		}
		httpClient, err = authenticator.GetClient(ctx)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	// Upload comments if requested
	if uploadComments {
		log.Printf("Parsing comment updates from %s...", filePath)
		updates, err := gdocs.ParseCommentsFromFile(filePath, docID)
		if err != nil {
			return fmt.Errorf("failed to parse comments: %w", err)
		}

		if dryRun {
			log.Println("Simulating comment updates (dry-run)...")
		} else {
			log.Printf("Uploading %d comment update(s) to document %s...", len(updates), docID)
		}
		if err := gdocs.UploadComments(ctx, httpClient, docID, updates, dryRun); err != nil {
			return fmt.Errorf("failed to upload comments: %w", err)
		}
		if dryRun {
			log.Println("Finished simulating comment updates.")
		} else {
			log.Println("Finished uploading comment updates.")
		}
		return nil
	}

	// Create Google Docs API client
	client, err := gdocs.NewClient(ctx, httpClient)
	if err != nil {
		return fmt.Errorf("failed to create Docs client: %w", err)
	}

	// Fetch document
	log.Printf("Fetching document %s...", docID)
	doc, err := client.FetchDocument(docID)
	if err != nil {
		return fmt.Errorf("failed to fetch document: %w", err)
	}

	if outputJSON {
		jsonData, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal document: %w", err)
		}
		fmt.Println(string(jsonData))
		return nil
	}

	// Convert to markdown
	var converter *markdown.Converter
	if tabID != "" {
		// Find the specific tab
		tab := gdocs.FindTab(doc, tabID)
		if tab == nil {
			return fmt.Errorf("tab '%s' not found in document", tabID)
		}
		if tab.DocumentTab == nil || tab.DocumentTab.Body == nil {
			return fmt.Errorf("tab '%s' has no document content", tabID)
		}
		tabTitle := tabID
		if tab.TabProperties != nil {
			tabTitle = tab.TabProperties.Title
		}
		log.Printf("Using tab: %s", tabTitle)
		converter = markdown.NewConverterFromTab(doc, tab)
	} else {
		converter = markdown.NewConverter(doc)
	}

	// Fetch and attach comments if requested
	if comments != "" {
		log.Println("Fetching comments...")
		allComments, err := gdocs.FetchComments(ctx, httpClient, docID)
		if err != nil {
			return fmt.Errorf("failed to fetch comments: %w", err)
		}

		filtered := allComments
		if comments == "open" {
			filtered = filtered[:0]
			for _, c := range allComments {
				if !c.Resolved {
					filtered = append(filtered, c)
				}
			}
		}

		if commentsSkipOlderThan > 0 {
			cutoff := time.Now().AddDate(0, 0, -commentsSkipOlderThan)
			ageFiltered := filtered[:0]
			for _, c := range filtered {
				if c.LastUpdateTime().After(cutoff) {
					ageFiltered = append(ageFiltered, c)
				}
			}
			filtered = ageFiltered
		}

		log.Printf("Found %d comment(s) (%d total)", len(filtered), len(allComments))

		if comments == "open" {
			converter.SetOpenCommentsOnly(true)
		}

		log.Println("Fetching mobilebasic HTML for comment placement...")
		mobileBasicHTML, mbErr := gdocs.FetchMobileBasicHTML(ctx, httpClient, docID)
		if mbErr != nil {
			log.Printf("Warning: failed to fetch mobilebasic HTML (%v); comments will all be listed as unattached", mbErr)
		}

		converter.SetComments(filtered, mobileBasicHTML)

		// Check for concurrent modification race
		if doc.RevisionId != "" {
			doc2, err := client.FetchDocument(docID)
			if err == nil && doc2.RevisionId != "" && doc2.RevisionId != doc.RevisionId {
				return fmt.Errorf("document was concurrently modified during processing (revision changed from %s to %s); please retry", doc.RevisionId, doc2.RevisionId)
			}
		}
	}

	markdownOutput, err := converter.Convert()
	if err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	// Print to stdout
	fmt.Print(markdownOutput)

	return nil
}

// initAuth initializes OAuth authentication and saves the token.
func initAuth(credPath string) error {
	ctx := context.Background()

	fmt.Println("Initializing OAuth authentication...")
	fmt.Println()

	// Create authenticator
	authenticator, err := auth.NewAuthenticator(credPath)
	if err != nil {
		return fmt.Errorf("authentication setup failed: %w", err)
	}

	// Get authenticated HTTP client (this will trigger OAuth flow)
	_, err = authenticator.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ Authentication successful!")
	fmt.Println("✓ Token saved to ~/.config/gdocs-cli/token.json")
	fmt.Println()
	fmt.Println("You can now use the CLI without the --init flag:")
	fmt.Println("  ./gdocs-cli --url=\"https://docs.google.com/document/d/DOC_ID/edit\"")

	return nil
}

// getDefaultConfigPath returns the default path for the config file.
func getDefaultConfigPath() (string, error) {
	configDir, err := auth.EnsureConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config directory: %w", err)
	}
	return configDir + "/config.json", nil
}
