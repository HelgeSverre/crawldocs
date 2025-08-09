# CrawlDocs

A Go-based web crawler that converts websites into markdown files.

## Description

CrawlDocs crawls websites and saves each page as a clean markdown file. It respects domain boundaries, handles duplicate
content, and can resume interrupted crawls.

## Installation

```bash
# Clone and build from source
git clone https://github.com/HelgeSverre/crawldocs.git
cd crawldocs
go build -o crawldocs

# Or install directly with Go
go install github.com/HelgeSverre/crawldocs@latest
```

## Quick Start

```bash
# Simplest form - just provide the URL
crawldocs https://docs.python.org

# Crawl with options
crawldocs https://example.com --max-pages 1000 --verbose

# Using short flags
crawldocs -u https://example.com -p 1000 -v

# Resume an interrupted crawl
crawldocs --resume --output docs_python_org

# Generate report from previous crawl
crawldocs --report --output docs_python_org

# Check version
crawldocs --version
```

## Command Line Options

| Option         | Short | Type   | Default     | Description                                     |
|----------------|-------|--------|-------------|-------------------------------------------------|
| `<URL>`        | -     | string | *required*  | Target URL to crawl (can be first argument)     |
| `--url`        | `-u`  | string | *required*  | Target URL to crawl (alternative to positional) |
| `--output`     | `-o`  | string | domain name | Output directory for markdown files             |
| `--max-pages`  | `-p`  | int    | 5000        | Maximum number of pages to crawl                |
| `--rate-limit` | `-r`  | int    | 10          | Maximum pages per second (0 = unlimited)        |
| `--workers`    | `-w`  | int    | 10          | Number of concurrent workers                    |
| `--verbose`    | `-v`  | bool   | false       | Enable verbose output                           |
| `--resume`     | -     | bool   | false       | Resume a previous crawl session                 |
| `--report`     | -     | bool   | false       | Generate a report from existing crawl data      |
| `--version`    | -     | bool   | false       | Display version information                     |

## Output Structure

```
output_dir/
├── 0001.md           # Crawled pages (or slug-based names)
├── 0002.md
├── ...
└── crawl-manifest.json   # Crawl metadata and statistics
```

Each markdown file contains:

- Page title as H1
- Source URL
- Cleaned text content

## How It Works

1. **Crawling**: Uses concurrent workers to fetch pages within the specified domain
2. **Content Processing**: Extracts text content, removes CSS/JavaScript artifacts
3. **Duplicate Detection**: Uses SHA-256 hashing to identify and skip duplicate content
4. **Progress Tracking**: Saves state to `crawl-manifest.json` for resumability

## Limitations

- Does not execute JavaScript (server-rendered content only)
- Text extraction only (no images, CSS, or other assets)
- Respects robots.txt and rate limits
- Single domain crawling only

## License

MIT License - see [LICENSE](LICENSE) file for details.