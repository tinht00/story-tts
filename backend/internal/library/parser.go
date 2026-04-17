package library

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"story-tts/backend/internal/model"
)

var whitespaceRe = regexp.MustCompile(`\n{3,}`)

type ParsedChapter struct {
	Title          string
	SourceFilePath string
	NormalizedText string
	Checksum       string
}

func ScanLocalTXT(sourceDir string) ([]string, error) {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".txt") {
			files = append(files, filepath.Join(sourceDir, entry.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

func ParseChapterFile(path string) (ParsedChapter, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ParsedChapter{}, err
	}
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return ParseChapterContent(title, raw, path), nil
}

func ParseChapterContent(title string, raw []byte, sourcePath string) ParsedChapter {
	text := decodeText(raw)
	text = NormalizeChapterText(text)

	sum := sha1.Sum([]byte(text))

	return ParsedChapter{
		Title:          title,
		SourceFilePath: sourcePath,
		NormalizedText: text,
		Checksum:       hex.EncodeToString(sum[:]),
	}
}

func NormalizeChapterText(text string) string {
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = stripTrailingDotsPerLine(text)
	text = whitespaceRe.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func stripTrailingDotsPerLine(text string) string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		trimmed := strings.TrimRightFunc(line, unicode.IsSpace)
		if strings.HasSuffix(trimmed, ".") && !strings.HasSuffix(trimmed, "..") {
			trimmed = strings.TrimSuffix(trimmed, ".")
		}
		lines[index] = trimmed
	}
	return strings.Join(lines, "\n")
}

func ToModelChapter(storyID int64, index int, libraryFilePath string, parsed ParsedChapter, preset model.ProsodyPreset) model.Chapter {
	return model.Chapter{
		StoryID:         storyID,
		ChapterIndex:    index,
		Title:           parsed.Title,
		SourceFilePath:  parsed.SourceFilePath,
		LibraryFilePath: libraryFilePath,
		NormalizedText:  parsed.NormalizedText,
		Checksum:        parsed.Checksum,
		Preset:          preset,
	}
}

func decodeText(raw []byte) string {
	if utf8.Valid(raw) {
		return string(raw)
	}
	if decoded, err := charmap.Windows1258.NewDecoder().Bytes(raw); err == nil && utf8.Valid(decoded) {
		return string(decoded)
	}
	if decoded, err := charmap.Windows1252.NewDecoder().Bytes(raw); err == nil && utf8.Valid(decoded) {
		return string(decoded)
	}
	return string(raw)
}
