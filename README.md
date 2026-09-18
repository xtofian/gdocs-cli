# Google Docs CLI

A command-line tool to fetch Google Docs content and convert it to Markdown with YAML frontmatter.

**Designed for AI Coding Agents**: This tool is specifically built to help AI coding agents fetch Google Docs documentation and convert it to clean markdown for analysis, context gathering, or integration into AI workflows. The stdout-based output and `--clean` flag make it ideal for piping into AI systems.

**📖 See [AGENTS.md](AGENTS.md)** for a comprehensive guide on integrating this tool with AI coding agents (Claude Code, Aider, Cursor, MCP servers, etc.).

## Features

- OAuth2 authentication with automatic token caching
- Converts Google Docs to clean Markdown format
- YAML frontmatter with document metadata
- Supports text formatting: bold, italic, strikethrough, links
- Supports document structure: headings, lists (bullet and numbered), tables
- Output to stdout for easy piping to files or other commands
- Round-trip-safe markup: literal metacharacters in the document are escaped,
  and a styled phrase always comes out as one span (see below)

## Prerequisites

- Go 1.24.1 or later
- A Google Cloud project with Google Docs API enabled
- OAuth 2.0 credentials (Desktop application type)

## Installation

### From Release Binaries

Prebuilt binaries for Linux, macOS, and Windows are available on the [GitHub Releases](https://github.com/famasya/gdocs-cli/releases) page.

```bash
# Download and install the latest release for your platform
# Linux (amd64)
curl -L https://github.com/famasya/gdocs-cli/releases/latest/download/gdocs-cli-linux-amd64 -o gdocs-cli
chmod +x gdocs-cli

# macOS (Apple Silicon)
curl -L https://github.com/famasya/gdocs-cli/releases/latest/download/gdocs-cli-darwin-arm64 -o gdocs-cli
chmod +x gdocs-cli

# macOS (Intel)
curl -L https://github.com/famasya/gdocs-cli/releases/latest/download/gdocs-cli-darwin-amd64 -o gdocs-cli
chmod +x gdocs-cli

# Windows (PowerShell)
Invoke-WebRequest -Uri "https://github.com/famasya/gdocs-cli/releases/latest/download/gdocs-cli-windows-amd64.exe" -OutFile "gdocs-cli.exe"
```

### From Source

```bash
git clone https://github.com/famasya/gdocs-cli.git
cd gdocs-cli
go build -o gdocs-cli cmd/gdocs-cli/main.go
```

### Using Go Install

```bash
go install github.com/famasya/gdocs-cli/cmd/gdocs-cli@latest
```

## Google Cloud Setup

Before using this tool, you need to set up OAuth2 credentials:

### 1. Create a Google Cloud Project

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select an existing one
3. Navigate to "APIs & Services" > "Library"
4. Search for "Google Docs API" and enable it

### 2. Create OAuth 2.0 Credentials

1. Go to "APIs & Services" > "Credentials"
2. Click "Create Credentials" > "OAuth client ID"
3. If prompted, configure the OAuth consent screen:
   - Choose "External" user type
   - Fill in required fields (app name, user support email)
   - Add your email as a test user
   - Save and continue
4. Choose "Desktop application" as the application type
5. Give it a name (e.g., "gdocs-cli")
6. Click "Create"
7. Download the credentials JSON file
8. Save it as `credentials.json` (or any name you prefer)

**Important:** Keep this file secure and never commit it to version control.

## Usage

### Initialize Authentication (Recommended First Step)

Before using the CLI for the first time, set up your credentials and initialize OAuth authentication:

**Option 1: Use default config location (recommended for AI agents)**
```bash
# Copy your credentials to the default location
mkdir -p ~/.config/gdocs-cli
cp credentials.json ~/.config/gdocs-cli/config.json

# Initialize OAuth (will use default config automatically)
./gdocs-cli --init
```

**Option 2: Specify config path**
```bash
./gdocs-cli --init --config="./credentials.json"
```

This will:
1. Open your browser for Google OAuth consent
2. Ask you to authorize the application
3. Save the token to `~/.config/gdocs-cli/token.json`

After initialization, you can use the CLI without re-authenticating.

### Basic Usage

**Using default config location:**
```bash
./gdocs-cli --url="https://docs.google.com/document/d/YOUR_DOC_ID/edit"
```

**Specifying config path:**
```bash
./gdocs-cli --url="https://docs.google.com/document/d/YOUR_DOC_ID/edit" --config="./credentials.json"
```

The tool will automatically use the cached token - no browser interaction needed.

**Note**: If `--config` is not provided, the tool looks for credentials at `~/.config/gdocs-cli/config.json` by default. This makes it easy for AI agents to use the tool without specifying the config path every time.

### Output to File

```bash
./gdocs-cli --url="https://docs.google.com/document/d/YOUR_DOC_ID/edit" > output.md
```

### Piping to Other Commands

```bash
./gdocs-cli --url="..." | less
./gdocs-cli --url="..." | grep "keyword"
```

### Include Comments

Use the `--comments` or `--comments=open` flag to include document comments in the markdown output:

```bash
# Include all comments (anchored in-line, unanchored at the end)
./gdocs-cli --url="https://docs.google.com/document/d/YOUR_DOC_ID/edit" --comments

# Include only unresolved (open) comments
./gdocs-cli --url="..." --comments=open

# Omit comments older than 30 days
./gdocs-cli --url="..." --comments --comments-skip-older-than=30
```

The tool places comment anchors inline by reading the document's mobilebasic
view, which marks every open thread at the point it is anchored, and matching
the text around each marker against the Docs API content. Any comments that
cannot be placed — resolved threads, threads whose anchor text has been deleted,
and threads anchored on another tab or inside a footnote — are appended under a
single flat section: `## Comments (unattached)`.

The Drive API cannot supply the positions directly: for Google Docs the `anchor`
field is an opaque `kix.*` identifier, and `quotedFileContent` is frequently a
single word (or a single period) that occurs all over the document. The Docs API
does have a `commentsViewMode` parameter that returns comment ranges, but as of
September 2026 it is limited to the [Workspace Developer Preview
program](https://developers.google.com/workspace/preview); once it is generally
available it should replace the mobilebasic path entirely.

Each comment block is embedded as a clean, human-readable **YAML** block inside HTML comment tags:
```html
<!-- gdoc-comment-content:
- id: AAAB9AVHdik
  author: Alice
  comment: This is a comment
  quoted-text: some text
  new-reply: ""
  status: draft
-->
```

**Note on Draft Fields:**
The tool automatically emits an empty `new-reply: ""` field and a `status: draft` field into each comment block inside the generated markdown. This allows you to easily type a reply inline, change `draft` to `ready`, and upload them back to Google Docs using the upload workflow!

> **⚠️ Important:** The `--comments` flag requires the `https://www.googleapis.com/auth/drive.readonly` scope. If you previously authenticated without this scope, you need to delete your cached token and re-authenticate:
>
> ```bash
> rm ~/.config/gdocs-cli/token.json
> ./gdocs-cli --init
> ```
>
> Also make sure you don't have non-HTTPS redirect URIs in any of your Google OAuth clients, as Google requires HTTPS for the Drive API scope.

### Clean Output (Suppress Logs)

Use the `--clean` flag to suppress all log output and only show the markdown:

```bash
./gdocs-cli --url="..." --clean
```

This is useful when:
- Piping output to AI systems or other tools
- Saving to a file without log messages
- Using in scripts where only the markdown is needed

**Example:**
```bash
# Without --clean: shows logs like "Fetching document..." to stderr
./gdocs-cli --url="..." > output.md

# With --clean: only markdown to stdout, no logs
./gdocs-cli --url="..." --clean > output.md

# Perfect for AI agents: clean output piped to processing
./gdocs-cli --url="..." --clean | your-ai-tool
```

### Upload Comments (Replies) to Existing Threads

Use the `--upload-comments` flag combined with `--file` to parse your local markdown or YAML file, find and reconcile any comments carrying replies, and append them back to the active Google Doc:

```bash
# Upload comment replies parsed from your local markdown file
./gdocs-cli --url="https://docs.google.com/document/d/YOUR_DOC_ID/edit" --file="sample.md" --upload-comments

# Simulate uploads (dry-run) without making live writes to the API
./gdocs-cli --url="https://docs.google.com/document/d/YOUR_DOC_ID/edit" --file="sample.md" --upload-comments --dry-run
```

**How It Works:**
1. **Doc ID Verification:** The tool automatically extracts the YAML frontmatter from the file. If the `gdoc_id` inside the frontmatter does not match the Google Doc ID specified in the `--url` flag, execution halts immediately with an error to prevent uploading replies to the wrong document.
2. **Comment Extraction:** It extracts and parses all YAML-serialized comment blocks. It supports both embedded comment blocks (`<!-- gdoc-comment-content:\n...-->`) and standalone `.yaml` files containing flat lists of comments.
3. **Idempotence & Safety:** The tool fetches the document's active comment threads from Google Drive, cross-references existing replies, and skips uploading if the reply has already been posted with the exact same content.
4. **Draft Skipping:** Comment threads in the file with `status: draft` are ignored and skipped. To send a reply, simply write your comment in the `new-reply:` field and flip `status` to `ready`.

**Example Comments/YAML Block inside the File:**
```yaml
- id: AAAxyzabc
  comment: "This is a comment thread on the doc"
  new-reply: "This is my ready response to Joe."
  status: ready
```

**Idempotence and Behavior:**
- **Idempotency**: The upload action is fully idempotent. The tool will automatically fetch the existing comment thread first and skip uploading any replies that already exist with the exact same content.
- **Early Exit**: When running with `--upload-comments`, the tool will exit immediately after processing comment uploads without fetching the document content or outputting markdown.

### Print Integration Instructions

Use the `--instruction` flag to print instructions for integrating this tool with AI coding agents:

```bash
./gdocs-cli --instruction
```

**Quick Integration**: Add this one-liner to your project's `AGENTS.md`, `CLAUDE.md`, or MCP config:

```
This project uses gdocs-cli. Run `gdocs-cli --instruction` for usage.
```

## Supported Google Docs Features

### Text Formatting
- **Bold** text
- *Italic* text
- ***Bold and italic***
- ~~Strikethrough~~
- [Links](https://example.com)

### Document Structure
- Headings (H1 through H6)
- Bullet lists
- Numbered lists
- Nested lists
- Paragraphs
- Tables

### Markup Fidelity

The output is meant to be parseable, not just readable — tools that diff it
against the document to push edits back upstream depend on markup meaning
exactly one thing. Two rules make that hold:

**Literal metacharacters are escaped.** A document containing the six
characters `*foo*` produces `\*foo\*`, not `*foo*`. Without this, a literal
asterisk is indistinguishable from real italics, and a round-trip turns the
one into the other. The escaped set is ``\ ` * _ [ ] < ~``, plus `|` inside
table cells. Underscores flanked by alphanumerics are left alone, since
CommonMark does not read `snake_case` as emphasis. Code spans are never
escaped — their content is literal already — so a backtick in the text widens
the fence instead.

**A styled phrase is one span.** Google Docs splits runs for reasons that have
nothing to do with appearance (edit boundaries, a font-size tweak), so one
italic phrase can arrive as three runs. Adjacent runs that render identically
are merged, giving `*continuous assurance at scale*` rather than
`*continuous* assurance at *scale*`. Delimiters also stay flush against
non-space text: a run whose styling covers a trailing space or the paragraph
mark emits `**AI.**\n`, never `**AI.\n**`.

A comment anchor landing inside a styled phrase still splits it, because an
HTML comment marker cannot sit between emphasis delimiters.

### YAML Frontmatter
The tool adds YAML frontmatter with document metadata:
```yaml
---
title: Document Title
gdoc_id: 166O6usLtU8iXbK8gDLmNrX_J1c-dfecq8AUdzje-t5I
revision_id: AIzaSyB...
author: (if available)
created: (if available)
modified: (if available)
---
```

**Note:** The Google Docs API v1 doesn't provide author or date information for the document. These fields may be empty in the frontmatter unless fetched from Google Drive API. Comments are supported via `--comments` flag (see below).

## Known Limitations

- **Tables:** Complex tables with merged cells may not convert perfectly to Markdown
- **Images:** Inline images are not currently supported
- **Drawings:** Not supported - will be skipped
- **Equations:** Not supported - will be skipped
- **Comments:** Supported via `--comments` flag (requires Drive API scope, see below)
- **Metadata:** Author and dates in frontmatter are not yet extracted (comments via Drive API are supported)

## Troubleshooting

### Error: Failed to read credentials file

**Cause:** The credentials file path is incorrect or the file doesn't exist.

**Solution:**
1. Verify the file path in the `--config` flag
2. Ensure you've downloaded the OAuth credentials JSON from Google Cloud Console
3. Use an absolute path or relative path from your current directory

### Error: Unable to access document

**Possible causes:**
1. The document is private and you don't have permission
2. The document doesn't exist
3. The document ID is incorrect

**Solutions:**
- Ensure the document is shared with your Google account
- Verify you're authenticated with the correct Google account
- Check that the URL is correct
- Try opening the document in your browser first

### Browser doesn't open during OAuth

**Solution:**
The authorization URL will be printed in the terminal. Copy and paste it into your browser manually.

### Token expired or invalid

**Solution:**
Delete the cached token and re-authenticate using the `--init` flag:
```bash
rm ~/.config/gdocs-cli/token.json
./gdocs-cli --init --config="credentials.json"
```

### Permission denied when creating config directory

**Solution:**
Ensure you have write permissions to `~/.config/`. Try creating it manually:
```bash
mkdir -p ~/.config/gdocs-cli
chmod 700 ~/.config/gdocs-cli
```

## Development

### Project Structure

```
gdocs-cli/
├── cmd/gdocs-cli/main.go              # CLI entry point
├── internal/
│   ├── auth/
│   │   ├── oauth.go                   # OAuth2 flow implementation
│   │   └── token.go                   # Token caching
│   ├── gdocs/
│   │   ├── client.go                  # Docs API client
│   │   └── url.go                     # URL parsing
│   └── markdown/
│       ├── converter.go               # Main converter
│       ├── text.go                    # Text formatting
│       ├── structure.go               # Structure conversion
│       └── frontmatter.go             # YAML frontmatter
├── go.mod
├── go.sum
└── README.md
```

### Building from Source

```bash
go build -o gdocs-cli cmd/gdocs-cli/main.go
```

### Running Tests

The project includes comprehensive unit tests for all core functionality:

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test ./... -v

# Run tests for a specific package
go test ./internal/gdocs -v
go test ./internal/markdown -v
go test ./internal/auth -v
```

**Test Coverage:**
- **CLI Integration** (`cmd/gdocs-cli/main_test.go`): End-to-end tests for CLI flags, error handling, and user flows
  - Help flag functionality
  - Missing required flags validation
  - Invalid URL handling
  - Missing credentials file errors
  - Clean flag recognition
- **URL Parsing** (`internal/gdocs/url_test.go`): Tests for extracting document IDs from various URL formats
- **Text Formatting** (`internal/markdown/text_test.go`): Tests for bold, italic, links, and text style conversion
- **Structure Conversion** (`internal/markdown/structure_test.go`): Tests for headings, lists, tables, and paragraph conversion
- **Token Handling** (`internal/auth/token_test.go`): Tests for token saving, loading, and file permissions

**Total: 45+ test cases** covering both unit and integration testing. All tests pass successfully and ensure the reliability of the CLI tool.

## Security Considerations

- **Credentials file:** Never commit your `credentials.json` to version control
- **Token cache:** Tokens are stored in `~/.config/gdocs-cli/token.json` with 0600 permissions (read/write for owner only)
- **OAuth scope:** The tool requests `documents.readonly` and `drive.readonly` scopes - no write access
- **Config directory:** Created with 0700 permissions (accessible only by owner)

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Credits

Built with:
- [Google Docs API](https://developers.google.com/docs/api)
- [golang.org/x/oauth2](https://pkg.go.dev/golang.org/x/oauth2)
- [gopkg.in/yaml.v3](https://gopkg.in/yaml.v3)
