package main

import (
	"html"
	"regexp"
	"strings"
	"sync"
)

// Pre-compiled regex patterns for performance
var (
	regexOnce     sync.Once
	fontFaceRe    *regexp.Regexp
	cssRuleRe     *regexp.Regexp
	cssPropertyRe *regexp.Regexp
	urlRe         *regexp.Regexp
	formatRe      *regexp.Regexp
	cssCommentRe  *regexp.Regexp
	wsRe          *regexp.Regexp
)

// initRegexPatterns initializes all regex patterns once
func initRegexPatterns() {
	regexOnce.Do(func() {
		fontFaceRe = regexp.MustCompile(`@font-face\s*{[^}]*}`)
		cssRuleRe = regexp.MustCompile(`@[a-zA-Z-]+[^{]*{[^}]*}`)
		cssPropertyRe = regexp.MustCompile(`[a-zA-Z-]+\s*:\s*[^;]+;`)
		urlRe = regexp.MustCompile(`url\([^)]+\)`)
		formatRe = regexp.MustCompile(`format\([^)]+\)`)
		cssCommentRe = regexp.MustCompile(`/\*[^*]*\*+(?:[^/*][^*]*\*+)*/`)
		wsRe = regexp.MustCompile(`\s+`)
	})
}

// ContentValidation holds validation results for page content
type ContentValidation struct {
	IsValid          bool
	ContentLength    int
	HasMeaningful    bool
	IsErrorPage      bool
	IsNavigationOnly bool
	CleanedContent   string
}

// cleanHTMLOptimized is an optimized version with pre-compiled regex
func cleanHTMLOptimized(text string) string {
	initRegexPatterns()

	// Remove CSS blocks
	text = fontFaceRe.ReplaceAllString(text, "")
	text = cssRuleRe.ReplaceAllString(text, "")
	text = cssPropertyRe.ReplaceAllString(text, "")
	text = urlRe.ReplaceAllString(text, "")
	text = formatRe.ReplaceAllString(text, "")
	text = cssCommentRe.ReplaceAllString(text, "")

	// Remove excessive whitespace
	text = wsRe.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	// Remove common UI artifacts - but be careful not to remove actual content
	artifacts := []string{
		"[x-cloak]", "wire:loading", "wire:offline",
		"/* vietnamese */", "/* latin-ext */", "/* latin */",
		"/* cyrillic */", "/* greek */", "/* devanagari */", "woff2", "woff",
	}

	for _, artifact := range artifacts {
		text = strings.ReplaceAll(text, artifact, "")
	}

	// Decode HTML entities
	text = html.UnescapeString(text)

	// Split into sentences and create paragraphs
	sentences := strings.Split(text, ". ")
	var paragraphs []string
	var currentParagraph []string

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if len(sentence) == 0 {
			continue
		}

		// Skip sentences that look like CSS or technical artifacts
		if strings.Contains(sentence, "{") || strings.Contains(sentence, "}") ||
			strings.Contains(sentence, "src:") || strings.Contains(sentence, "font-family") ||
			len(sentence) > 500 {
			continue
		}

		currentParagraph = append(currentParagraph, sentence)

		// Start new paragraph after 3-5 sentences
		if len(currentParagraph) >= 3 {
			paragraphs = append(paragraphs, strings.Join(currentParagraph, ". ")+".")
			currentParagraph = nil
		}
	}

	// Add remaining sentences
	if len(currentParagraph) > 0 {
		paragraphs = append(paragraphs, strings.Join(currentParagraph, ". ")+".")
	}

	return strings.Join(paragraphs, "\n\n")
}

// validateContent checks if the content is worth saving
func validateContent(text string, url string) ContentValidation {
	cleaned := cleanHTMLOptimized(text)
	contentLength := len(cleaned)

	validation := ContentValidation{
		ContentLength:  contentLength,
		CleanedContent: cleaned,
		IsValid:        contentLength >= 100, // Simple length check
		HasMeaningful:  contentLength >= 100,
	}

	return validation
}

// extractContentMetadata extracts useful metadata from HTML
func extractContentMetadata(e interface{}) map[string]string {
	metadata := make(map[string]string)

	// This would need to be adapted based on the actual HTML parsing library
	// For now, returning a basic structure
	metadata["description"] = ""
	metadata["keywords"] = ""
	metadata["author"] = ""

	return metadata
}

// cleanHTMLSimple is a simple version for backward compatibility
func cleanHTMLSimple(text string) string {
	return cleanHTMLOptimized(text)
}
