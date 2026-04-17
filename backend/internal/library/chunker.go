package library

import (
	"strings"

	"story-tts/backend/internal/model"
)

type ChunkPlanner struct {
	MaxChars int
	MaxWords int
}

func NewChunkPlanner(maxChars int) ChunkPlanner {
	if maxChars <= 0 {
		maxChars = 900
	}
	return ChunkPlanner{
		MaxChars: maxChars,
		MaxWords: 120,
	}
}

func (p ChunkPlanner) Plan(text string) []model.ChunkPlan {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	paragraphs := splitParagraphs(text)
	var plans []model.ChunkPlan
	var current strings.Builder
	currentWords := 0

	flush := func() {
		content := strings.TrimSpace(current.String())
		if content == "" {
			current.Reset()
			currentWords = 0
			return
		}
		plans = append(plans, model.ChunkPlan{Index: len(plans), Text: content})
		current.Reset()
		currentWords = 0
	}

	for _, paragraph := range paragraphs {
		sentences := splitSentences(paragraph)
		for _, sentence := range sentences {
			trimmed := strings.TrimSpace(sentence)
			if trimmed == "" {
				continue
			}
			sentenceWords := countWords(trimmed)

			if current.Len() > 0 && (current.Len()+len(trimmed)+1 > p.MaxChars || currentWords+sentenceWords > p.MaxWords) {
				flush()
			}

			if len(trimmed) > p.MaxChars || sentenceWords > p.MaxWords {
				for _, part := range splitHard(trimmed, p.MaxChars, p.MaxWords) {
					if current.Len() > 0 {
						flush()
					}
					current.WriteString(part)
					currentWords = countWords(part)
					flush()
				}
				continue
			}

			if current.Len() > 0 {
				current.WriteString(" ")
			}
			current.WriteString(trimmed)
			currentWords += sentenceWords
		}
		if current.Len() > 0 {
			flush()
		}
	}

	return plans
}

func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n\n")
	var out []string
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func splitSentences(paragraph string) []string {
	var out []string
	var current strings.Builder
	for _, r := range paragraph {
		current.WriteRune(r)
		switch r {
		case '.', '!', '?', ';', ':', '…':
			out = append(out, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}

func splitHard(text string, charLimit, wordLimit int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var out []string
	var current strings.Builder
	currentWords := 0
	for _, word := range words {
		if current.Len() > 0 && (current.Len()+len(word)+1 > charLimit || currentWords+1 > wordLimit) {
			out = append(out, current.String())
			current.Reset()
			currentWords = 0
		}
		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(word)
		currentWords++
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}

func countWords(text string) int {
	return len(strings.Fields(text))
}
