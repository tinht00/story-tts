package library

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var invalidSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

type StoryPaths struct {
	Root             string
	SourceDir        string
	SourceChapters   string
	ArtifactsDir     string
	ArtifactChapters string
	ArtifactFull     string
	WorkDir          string
	WorkSegments     string
}

func Slugify(input string) string {
	value := strings.ToLower(strings.TrimSpace(input))
	value = invalidSlugChars.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "story"
	}
	return value
}

func ResolveStoryPaths(libraryRoot, storySlug string) StoryPaths {
	root := filepath.Join(libraryRoot, storySlug)
	return StoryPaths{
		Root:             root,
		SourceDir:        filepath.Join(root, "source"),
		SourceChapters:   filepath.Join(root, "source", "chapters"),
		ArtifactsDir:     filepath.Join(root, "artifacts"),
		ArtifactChapters: filepath.Join(root, "artifacts", "chapters"),
		ArtifactFull:     filepath.Join(root, "artifacts", "full"),
		WorkDir:          filepath.Join(root, "work"),
		WorkSegments:     filepath.Join(root, "work", "segments"),
	}
}

func EnsureStoryDirs(paths StoryPaths) error {
	for _, dir := range []string{
		paths.Root,
		paths.SourceDir,
		paths.SourceChapters,
		paths.ArtifactsDir,
		paths.ArtifactChapters,
		paths.ArtifactFull,
		paths.WorkDir,
		paths.WorkSegments,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func ChapterFileName(index int, title string) string {
	base := Slugify(title)
	if base == "" {
		base = "chapter"
	}
	return fmt.Sprintf("%03d-%s.txt", index, base)
}

func ChapterAudioName(index int, title string) string {
	base := Slugify(title)
	if base == "" {
		base = "chapter"
	}
	return fmt.Sprintf("%03d-%s.mp3", index, base)
}
