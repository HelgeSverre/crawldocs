# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

crawldocs is a production-ready Go-based website crawler that uses the Colly framework to crawl entire websites and convert them to clean markdown files. It supports both sequential numbering (0001.md) and URL-based slug filenames (installation.md). Successfully tested on major documentation sites including Laravel, CodeIgniter, and curl docs.

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
./crawldocs -url=https://example.com/docs [-o=output_dir] [-max=5000] [-slug] [-v]

# Examples:
./crawldocs -url=https://laravel.com/docs -max=100 -v          # Sequential naming
./crawldocs -url=https://docs.python.org -slug -max=0 -v      # URL-based slugs, unlimited pages
```

### Development
```bash
make deps                # Update dependencies (go mod tidy)
go test -v -run TestCleanHTMLSimple  # Run specific test
make test-cover          # Run tests with coverage
```

## Architecture

### Core Components

1. **main.go**: Contains the crawler implementation using Colly
   - `Crawler` struct manages state with mutex protection and useSlug flag
   - Concurrent crawling limited to 2 parallel requests
   - Domain-restricted crawling (stays within same host)
   - `slugify()` function converts URLs to safe filenames

2. **clean.go**: HTML cleaning pipeline
   - `cleanHTMLSimple()` is the active cleaning function
   - Removes CSS blocks, scripts, HTML tags, and web artifacts
   - Intelligent paragraph formatting with regex-based cleaning

3. **Output Structure**: 
   - Creates directory named after domain (or custom via -o flag)
   - Two naming modes:
     - Sequential: 0001.md, 0002.md, etc. (default)
     - URL-based slugs: installation.md, getting-started.md (with -slug flag)
   - Each file contains title, source URL, and cleaned content
   - Duplicate slug handling with automatic counter appending

### Key Technical Decisions

- Uses `e.DOM.Text()` for initial text extraction, then applies extensive cleaning
- Regex-based cleaning pipeline to handle modern web artifacts
- Stateful crawler with visited URL tracking to prevent duplicates
- Maximum page limit (default 5000, 0 = unlimited) to prevent runaway crawling
- URL slugification with safe character replacement and length limits
- Filename collision detection and resolution for slug mode

### Testing

Tests focus on the HTML cleaning functionality. When modifying `cleanHTMLSimple()`, ensure tests pass:
- CSS removal (font-face, style blocks)
- Script removal
- Whitespace normalization
- Artifact removal

Run tests with: `go test -v` or `make test`

### Production Testing

Successfully tested on major documentation sites:
- **Laravel.com/docs**: 374 pages crawled
- **CodeIgniter.com/user_guide**: 364 pages crawled  
- **curl.se/docs**: 1096+ pages crawled (partial due to timeout)

### Common Issues

1. If HTML cleaning is too aggressive, check the regex patterns in `clean.go`
2. For sites with heavy JavaScript, content may be missing (Colly doesn't execute JS)
3. Rate limiting can be adjusted via `collector.Limit()` in main.go
4. For slug filename conflicts, the tool automatically appends counters (e.g., installation-2.md)
5. Use `max=0` for unlimited crawling on large documentation sites

### CLI Flags Reference

- `-url`: Target URL to crawl (required)
- `-o`: Output directory name (defaults to domain name)
- `-max`: Maximum pages to crawl (default: 5000, 0 = unlimited)  
- `-slug`: Use URL-based filenames instead of sequential numbers
- `-v`: Enable verbose logging