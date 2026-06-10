package indexer

import (
	"strings"
)

const (
	MinChunkTokens = 400
	MaxChunkTokens = 800
	MaxChunkChars  = 3200
)

// Split divides the content into indexable chunks.
// It recursively splits on \n\n, \n, and spaces.
func Split(content string, sourceType string) []string {
	return recursiveSplit(content, []string{"\n\n", "\n", " "})
}

func recursiveSplit(text string, delimiters []string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	tokens := estimateTokens(text)
	if (tokens <= MaxChunkTokens && len(text) <= MaxChunkChars) || len(delimiters) == 0 {
		return []string{text}
	}

	delimiter := delimiters[0]
	parts := strings.Split(text, delimiter)
	if len(parts) == 1 {
		return recursiveSplit(text, delimiters[1:])
	}

	var chunks []string
	var currentChunk []string
	currentTokens := 0
	currentChars := 0

	for _, part := range parts {
		partTokens := estimateTokens(part)
		partChars := len(part)
		delimiterChars := 0
		if len(currentChunk) > 0 {
			delimiterChars = len(delimiter)
		}

		if (currentTokens+partTokens > MaxChunkTokens || currentChars+delimiterChars+partChars > MaxChunkChars) && len(currentChunk) > 0 {
			chunks = append(chunks, strings.Join(currentChunk, delimiter))
			currentChunk = nil
			currentTokens = 0
			currentChars = 0
			delimiterChars = 0
		}

		if partTokens > MaxChunkTokens || partChars > MaxChunkChars {
			// Part itself is too large, split it further
			subChunks := recursiveSplit(part, delimiters[1:])
			chunks = append(chunks, subChunks...)
		} else {
			currentChunk = append(currentChunk, part)
			currentTokens += partTokens + 1 // +1 for the delimiter
			currentChars += delimiterChars + partChars
		}
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, strings.Join(currentChunk, delimiter))
	}

	return chunks
}

func estimateTokens(text string) int {
	// Simple word count as token estimate
	return len(strings.Fields(text))
}
