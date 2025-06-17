package main

import (
	"html"
	"regexp"
	"strings"
)

func cleanHTMLSimple(text string) string {
	// Remove CSS blocks (@font-face, @media, etc.)
	fontFaceRe := regexp.MustCompile(`@font-face\s*{[^}]*}`)
	text = fontFaceRe.ReplaceAllString(text, "")
	
	// Remove other CSS at-rules  
	cssRuleRe := regexp.MustCompile(`@[a-zA-Z-]+[^{]*{[^}]*}`)
	text = cssRuleRe.ReplaceAllString(text, "")
	
	// Remove CSS properties
	cssPropertyRe := regexp.MustCompile(`[a-zA-Z-]+\s*:\s*[^;]+;`)
	text = cssPropertyRe.ReplaceAllString(text, "")
	
	// Remove URLs in CSS format
	urlRe := regexp.MustCompile(`url\([^)]+\)`)
	text = urlRe.ReplaceAllString(text, "")
	
	// Remove format declarations
	formatRe := regexp.MustCompile(`format\([^)]+\)`)
	text = formatRe.ReplaceAllString(text, "")
	
	// Remove CSS comments
	cssCommentRe := regexp.MustCompile(`/\*[^*]*\*+(?:[^/*][^*]*\*+)*/`)
	text = cssCommentRe.ReplaceAllString(text, "")
	
	// Remove excessive whitespace and clean up text
	wsRe := regexp.MustCompile(`\s+`)
	text = wsRe.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	
	// Remove common UI artifacts and navigation items
	artifacts := []string{
		"[x-cloak]", "display: none", "wire:loading", "wire:offline", 
		"Toggle Menu", "/* vietnamese */", "/* latin-ext */", "/* latin */",
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
	
	// Add remaining sentences as final paragraph
	if len(currentParagraph) > 0 {
		paragraphs = append(paragraphs, strings.Join(currentParagraph, ". ")+".")
	}
	
	return strings.Join(paragraphs, "\n\n")
}