package library

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

// === Tests for decodeText ===

func TestDecodeText_UTF8(t *testing.T) {
	input := []byte("Xin chào thế giới")
	result := decodeText(input)
	expected := "Xin chào thế giới"
	if result != expected {
		t.Errorf("decodeText UTF-8 = %q, want %q", result, expected)
	}
}

func TestDecodeText_UTF8WithBOM(t *testing.T) {
	// UTF-8 with BOM (\xef\xbb\xbf) should be handled by NormalizeChapterText
	input := []byte{0xef, 0xbb, 0xbf, 'X', 'i', 'n'}
	result := decodeText(input)
	// UTF-8 BOM bytes are valid UTF-8, so they'll be preserved
	if !utf8ValidString(result) {
		t.Error("decodeText result should be valid UTF-8")
	}
}

func TestDecodeText_Windows1258(t *testing.T) {
	// Windows-1258 encoded "Xin chào" (simplified test)
	// In Windows-1258: 'à' = 0xE0
	input := []byte{0x58, 0x69, 0x6E, 0x20, 0x63, 0x68, 0xE0, 0x6F}
	result := decodeText(input)
	// Should decode to something containing "chào"
	if result == "" {
		t.Error("decodeText Windows-1258 returned empty string")
	}
}

func TestDecodeText_Windows1252(t *testing.T) {
	// Windows-1252 encoded text with special characters
	// "Café" in Windows-1252: 'é' = 0xE9
	input := []byte{0x43, 0x61, 0x66, 0xE9}
	result := decodeText(input)
	if result == "" {
		t.Error("decodeText Windows-1252 returned empty string")
	}
}

func TestDecodeText_InvalidFallback(t *testing.T) {
	// Invalid bytes that can't be decoded by any encoder
	input := []byte{0xFF, 0xFE, 0x00, 0x01}
	result := decodeText(input)
	// Should fallback to string(raw)
	if result == "" {
		t.Error("decodeText fallback returned empty string")
	}
}

// === Tests for NormalizeChapterText ===

func TestNormalizeChapterText_RemoveBOM(t *testing.T) {
	input := "\ufeffXin chào"
	result := NormalizeChapterText(input)
	expected := "Xin chào"
	if result != expected {
		t.Errorf("NormalizeChapterText BOM = %q, want %q", result, expected)
	}
}

func TestNormalizeChapterText_CRLF(t *testing.T) {
	input := "Dòng 1\r\nDòng 2\r\n"
	result := NormalizeChapterText(input)
	expected := "Dòng 1\nDòng 2"
	if result != expected {
		t.Errorf("NormalizeChapterText CRLF = %q, want %q", result, expected)
	}
}

func TestNormalizeChapterText_CR(t *testing.T) {
	input := "Dòng 1\rDòng 2"
	result := NormalizeChapterText(input)
	expected := "Dòng 1\nDòng 2"
	if result != expected {
		t.Errorf("NormalizeChapterText CR = %q, want %q", result, expected)
	}
}

func TestNormalizeChapterText_MultipleNewlines(t *testing.T) {
	input := "Đoạn 1\n\n\n\nĐoạn 2"
	result := NormalizeChapterText(input)
	expected := "Đoạn 1\n\nĐoạn 2"
	if result != expected {
		t.Errorf("NormalizeChapterText multiple newlines = %q, want %q", result, expected)
	}
}

func TestNormalizeChapterText_TrailingDots(t *testing.T) {
	input := "Câu 1.\nCâu 2..\nCâu 3..."
	result := NormalizeChapterText(input)
	expected := "Câu 1\nCâu 2..\nCâu 3..."
	if result != expected {
		t.Errorf("NormalizeChapterText trailing dots = %q, want %q", result, expected)
	}
}

func TestNormalizeChapterText_TrimSpace(t *testing.T) {
	input := "\n\n  Xin chào  \n\n"
	result := NormalizeChapterText(input)
	expected := "Xin chào"
	if result != expected {
		t.Errorf("NormalizeChapterText trim = %q, want %q", result, expected)
	}
}

// === Tests for stripTrailingDotsPerLine ===

func TestStripTrailingDotsPerLine_SingleDot(t *testing.T) {
	input := "Câu 1.\nCâu 2."
	result := stripTrailingDotsPerLine(input)
	expected := "Câu 1\nCâu 2"
	if result != expected {
		t.Errorf("stripTrailingDotsPerLine = %q, want %q", result, expected)
	}
}

func TestStripTrailingDotsPerLine_DoubleDot(t *testing.T) {
	// Double dots should NOT be stripped
	input := "Câu 1..\nCâu 2.."
	result := stripTrailingDotsPerLine(input)
	expected := "Câu 1..\nCâu 2.."
	if result != expected {
		t.Errorf("stripTrailingDotsPerLine double dots = %q, want %q", result, expected)
	}
}

func TestStripTrailingDotsPerLine_Ellipsis(t *testing.T) {
	// Ellipsis (3+ dots) should NOT be stripped
	input := "Câu 1...\nCâu 2...."
	result := stripTrailingDotsPerLine(input)
	expected := "Câu 1...\nCâu 2...."
	if result != expected {
		t.Errorf("stripTrailingDotsPerLine ellipsis = %q, want %q", result, expected)
	}
}

func TestStripTrailingDotsPerLine_NoDots(t *testing.T) {
	input := "Câu 1\nCâu 2"
	result := stripTrailingDotsPerLine(input)
	expected := "Câu 1\nCâu 2"
	if result != expected {
		t.Errorf("stripTrailingDotsPerLine no dots = %q, want %q", result, expected)
	}
}

func TestStripTrailingDotsPerLine_TrailingSpace(t *testing.T) {
	input := "Câu 1.  \nCâu 2.   "
	result := stripTrailingDotsPerLine(input)
	expected := "Câu 1\nCâu 2"
	if result != expected {
		t.Errorf("stripTrailingDotsPerLine trailing space = %q, want %q", result, expected)
	}
}

// === Tests for ParseChapterContent ===

func TestParseChapterContent_Basic(t *testing.T) {
	content := "Đây là nội dung chương 1"
	title := "Chương 1"
	sourcePath := "/test/chuong1.txt"

	result := ParseChapterContent(title, []byte(content), sourcePath)

	if result.Title != title {
		t.Errorf("Title = %q, want %q", result.Title, title)
	}
	if result.NormalizedText != content {
		t.Errorf("NormalizedText = %q, want %q", result.NormalizedText, content)
	}
	if result.SourceFilePath != sourcePath {
		t.Errorf("SourceFilePath = %q, want %q", result.SourceFilePath, sourcePath)
	}
	if result.Checksum == "" {
		t.Error("Checksum should not be empty")
	}
}

func TestParseChapterContent_Normalization(t *testing.T) {
	content := "\ufeffDòng 1.\r\n\r\n\r\nDòng 2."
	result := ParseChapterContent("Test", []byte(content), "/test.txt")
	expected := "Dòng 1\n\nDòng 2"
	if result.NormalizedText != expected {
		t.Errorf("ParseChapterContent normalization = %q, want %q", result.NormalizedText, expected)
	}
}

func TestParseChapterContent_ChecksumConsistency(t *testing.T) {
	content := "Nội dung không đổi"
	result1 := ParseChapterContent("Test", []byte(content), "/test.txt")
	result2 := ParseChapterContent("Test", []byte(content), "/test.txt")

	if result1.Checksum != result2.Checksum {
		t.Error("Same content should produce same checksum")
	}
}

func TestParseChapterContent_ChecksumDifferent(t *testing.T) {
	result1 := ParseChapterContent("Test", []byte("Nội dung 1"), "/test.txt")
	result2 := ParseChapterContent("Test", []byte("Nội dung 2"), "/test.txt")

	if result1.Checksum == result2.Checksum {
		t.Error("Different content should produce different checksums")
	}
}

// === Tests for ParseChapterFile ===

func TestParseChapterFile_ExistingFile(t *testing.T) {
	// Create temp file
	dir := t.TempDir()
	filePath := filepath.Join(dir, "chuong1.txt")
	content := "Nội dung chương 1"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	result, err := ParseChapterFile(filePath)
	if err != nil {
		t.Fatalf("ParseChapterFile failed: %v", err)
	}

	expectedTitle := "chuong1"
	if result.Title != expectedTitle {
		t.Errorf("Title = %q, want %q", result.Title, expectedTitle)
	}
	if result.NormalizedText != content {
		t.Errorf("NormalizedText = %q, want %q", result.NormalizedText, content)
	}
}

func TestParseChapterFile_NonExistent(t *testing.T) {
	_, err := ParseChapterFile("/nonexistent/file.txt")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

// === Tests for ScanLocalTXT ===

func TestScanLocalTXT_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	files, err := ScanLocalTXT(dir)
	if err != nil {
		t.Fatalf("ScanLocalTXT failed: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("Expected 0 files, got %d", len(files))
	}
}

func TestScanLocalTXT_WithTXTFiles(t *testing.T) {
	dir := t.TempDir()
	// Create some .txt files
	for _, name := range []string{"file1.txt", "file2.txt", "file3.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	files, err := ScanLocalTXT(dir)
	if err != nil {
		t.Fatalf("ScanLocalTXT failed: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(files))
	}
	// Should be sorted
	if files[0] != filepath.Join(dir, "file1.txt") {
		t.Error("Files should be sorted")
	}
}

func TestScanLocalTXT_IgnoresNonTXTFiles(t *testing.T) {
	dir := t.TempDir()
	// Create mixed files
	for _, name := range []string{"file1.txt", "file2.md", "file3.pdf", "file4.TXT"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	files, err := ScanLocalTXT(dir)
	if err != nil {
		t.Fatalf("ScanLocalTXT failed: %v", err)
	}
	// Should find file1.txt and file4.TXT (case-insensitive)
	if len(files) != 2 {
		t.Errorf("Expected 2 .txt files, got %d", len(files))
	}
}

func TestScanLocalTXT_IgnoresDirectories(t *testing.T) {
	dir := t.TempDir()
	// Create a subdirectory
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}
	// Create a .txt file
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	files, err := ScanLocalTXT(dir)
	if err != nil {
		t.Fatalf("ScanLocalTXT failed: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(files))
	}
}

// === Tests for ToModelChapter ===

func TestToModelChapter_Basic(t *testing.T) {
	parsed := ParsedChapter{
		Title:          "Chương 1",
		SourceFilePath: "/source/chuong1.txt",
		NormalizedText: "Nội dung",
		Checksum:       "abc123",
	}

	chapter := ToModelChapter(1, 1, "/library/chuong1.txt", parsed, "stable")

	if chapter.StoryID != 1 {
		t.Errorf("StoryID = %d, want 1", chapter.StoryID)
	}
	if chapter.ChapterIndex != 1 {
		t.Errorf("ChapterIndex = %d, want 1", chapter.ChapterIndex)
	}
	if chapter.Title != "Chương 1" {
		t.Errorf("Title = %q, want %q", chapter.Title, "Chương 1")
	}
	if chapter.Preset != "stable" {
		t.Errorf("Preset = %q, want %q", chapter.Preset, "stable")
	}
}

// Helper function

func utf8ValidString(s string) bool {
	return utf8.ValidString(s)
}
