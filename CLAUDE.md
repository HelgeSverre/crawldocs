lets # CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

crawldocs is a Go-based website crawler that uses the Colly framework to crawl entire websites and convert them to numbered markdown files. It's designed to handle modern web content with extensive CSS/JavaScript cleaning.

## Key Commands

### Build and Run
```bash
make build                # Build the crawldocs binary
make clean && make build  # Clean build
make run                  # Run directly without building
make test                 # Run all tests
go test -v               # Run tests with verbose output
```

### Usage
```bash
./crawldocs -url=https://example.com/docs [-o=output_dir] [-max=5000] [-v]
```

### Development
```bash
make deps                # Update dependencies (go mod tidy)
go test -v -run TestCleanHTML  # Run specific test
```

## Architecture

### Core Components

1. **main.go**: Contains the crawler implementation using Colly
   - `Crawler` struct manages state with mutex protection
   - Concurrent crawling limited to 2 parallel requests
   - Domain-restricted crawling (stays within same host)

2. **clean.go**: HTML cleaning pipeline
   - `cleanHTMLNew()` is the active cleaning function
   - Removes CSS blocks, scripts, HTML tags, and web artifacts
   - Intelligent paragraph formatting

3. **Output Structure**: 
   - Creates directory named after domain (or custom via -o flag)
   - Files numbered sequentially: 0001.md, 0002.md, etc.
   - Each file contains title, source URL, and cleaned content

### Key Technical Decisions

- Uses `e.DOM.Text()` for initial text extraction, then applies extensive cleaning
- Regex-based cleaning pipeline to handle modern web artifacts
- Stateful crawler with visited URL tracking to prevent duplicates
- Maximum page limit (default 5000) to prevent runaway crawling

### Testing

Tests focus on the HTML cleaning functionality. When modifying `cleanHTMLNew()`, ensure tests pass:
- CSS removal (font-face, style blocks)
- Script removal
- Whitespace normalization
- Artifact removal

### Common Issues

1. If HTML cleaning is too aggressive, check the regex patterns in `clean.go`
2. For sites with heavy JavaScript, content may be missing (Colly doesn't execute JS)
3. Rate limiting can be adjusted via `collector.Limit()` in main.go