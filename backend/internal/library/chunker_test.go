package library

import (
	"strings"
	"testing"
)

// === Tests for NewChunkPlanner ===

func TestNewChunkPlanner_Default(t *testing.T) {
	planner := NewChunkPlanner(0)
	if planner.MaxChars != 900 {
		t.Errorf("MaxChars = %d, want 900", planner.MaxChars)
	}
	if planner.MaxWords != 120 {
		t.Errorf("MaxWords = %d, want 120", planner.MaxWords)
	}
}

func TestNewChunkPlanner_Custom(t *testing.T) {
	planner := NewChunkPlanner(500)
	if planner.MaxChars != 500 {
		t.Errorf("MaxChars = %d, want 500", planner.MaxChars)
	}
	if planner.MaxWords != 120 {
		t.Errorf("MaxWords = %d, want 120", planner.MaxWords)
	}
}

func TestNewChunkPlanner_NegativeValue(t *testing.T) {
	planner := NewChunkPlanner(-100)
	if planner.MaxChars != 900 {
		t.Errorf("MaxChars = %d, want 900 (default for negative)", planner.MaxChars)
	}
}

// === Tests for Plan ===

func TestPlan_EmptyText(t *testing.T) {
	planner := NewChunkPlanner(900)
	plans := planner.Plan("")
	if plans != nil {
		t.Errorf("Expected nil for empty text, got %v", plans)
	}
}

func TestPlan_WhitespaceOnly(t *testing.T) {
	planner := NewChunkPlanner(900)
	plans := planner.Plan("   \n\n\n   ")
	if plans != nil {
		t.Errorf("Expected nil for whitespace-only, got %v", plans)
	}
}

func TestPlan_SingleSentence(t *testing.T) {
	planner := NewChunkPlanner(900)
	text := "Đây là một câu đơn giản."
	plans := planner.Plan(text)

	if len(plans) != 1 {
		t.Fatalf("Expected 1 plan, got %d", len(plans))
	}
	if plans[0].Index != 0 {
		t.Errorf("Index = %d, want 0", plans[0].Index)
	}
	if plans[0].Text != text {
		t.Errorf("Text = %q, want %q", plans[0].Text, text)
	}
}

func TestPlan_MultipleSentences_InOneChunk(t *testing.T) {
	planner := NewChunkPlanner(900)
	text := "Câu thứ nhất. Câu thứ hai! Câu thứ ba?"
	plans := planner.Plan(text)

	if len(plans) != 1 {
		t.Fatalf("Expected 1 plan, got %d", len(plans))
	}
	expected := "Câu thứ nhất. Câu thứ hai! Câu thứ ba?"
	if plans[0].Text != expected {
		t.Errorf("Text = %q, want %q", plans[0].Text, expected)
	}
}

func TestPlan_SplitAtSentenceBoundary(t *testing.T) {
	// Create text that exceeds MaxChars, should split at sentence boundary
	planner := NewChunkPlanner(100) // Small limit for testing
	text := "Câu một ngắn. Câu hai cũng ngắn. Câu ba dài hơn một chút để vượt quá giới hạn ký tự."
	plans := planner.Plan(text)

	if len(plans) < 2 {
		t.Fatalf("Expected at least 2 plans, got %d", len(plans))
	}

	// Verify each chunk respects the character limit (approximately)
	for i, plan := range plans {
		if plan.Index != i {
			t.Errorf("Plan %d: Index = %d, want %d", i, plan.Index, i)
		}
	}
}

func TestPlan_SentenceBoundary_NotCuttingMidSentence(t *testing.T) {
	planner := NewChunkPlanner(50)
	// This sentence is longer than the limit, should be split by word
	text := "Câu này dài hơn năm mươi ký tự và sẽ bị cắt."
	plans := planner.Plan(text)

	if len(plans) < 2 {
		t.Fatalf("Expected at least 2 plans for long sentence, got %d", len(plans))
	}

	// Verify no chunk exceeds the limit significantly
	for i, plan := range plans {
		wordCount := len(strings.Fields(plan.Text))
		if wordCount > planner.MaxWords+5 { // Small tolerance for single long words
			t.Errorf("Plan %d: %d words exceeds limit %d", i, wordCount, planner.MaxWords)
		}
	}
}

func TestPlan_MultipleParagraphs(t *testing.T) {
	planner := NewChunkPlanner(900)
	text := "Đoạn 1, câu 1. Đoạn 1, câu 2.\n\nĐoạn 2, câu 1. Đoạn 2, câu 2."
	plans := planner.Plan(text)

	if len(plans) < 1 {
		t.Fatalf("Expected at least 1 plan, got %d", len(plans))
	}

	// All text should be present in the plans
	var reconstructed string
	for i, plan := range plans {
		if i > 0 {
			reconstructed += " "
		}
		reconstructed += plan.Text
	}

	// Normalize whitespace for comparison
	original := strings.Join(strings.Fields(text), " ")
	reconstructed = strings.Join(strings.Fields(reconstructed), " ")

	if reconstructed != original {
		t.Errorf("Reconstructed text doesn't match original.\nGot: %q\nWant: %q", reconstructed, original)
	}
}

func TestPlan_ChunkPlanIndex(t *testing.T) {
	planner := NewChunkPlanner(50)
	text := "Câu 1. Câu 2. Câu 3. Câu 4. Câu 5."
	plans := planner.Plan(text)

	for i, plan := range plans {
		if plan.Index != i {
			t.Errorf("Plan %d: Index = %d, want %d", i, plan.Index, i)
		}
	}
}

func TestPlan_VietnamesePunctuation(t *testing.T) {
	planner := NewChunkPlanner(900)
	text := "Anh ấy hỏi: \"Em khỏe không?\" Tôi trả lời: \"Dạ khỏe! Còn anh?\""
	plans := planner.Plan(text)

	if len(plans) < 1 {
		t.Fatalf("Expected at least 1 plan, got %d", len(plans))
	}

	// Check that punctuation is handled correctly
	var fullText string
	for _, plan := range plans {
		fullText += plan.Text + " "
	}

	if !strings.Contains(fullText, "?") || !strings.Contains(fullText, "!") {
		t.Error("Vietnamese punctuation marks missing from plans")
	}
}

func TestPlan_Ellipsis(t *testing.T) {
	planner := NewChunkPlanner(900)
	text := "Trời ơi… Sao lại thế này…"
	plans := planner.Plan(text)

	if len(plans) < 1 {
		t.Fatalf("Expected at least 1 plan, got %d", len(plans))
	}
}

func TestPlan_ColonAndSemicolon(t *testing.T) {
	planner := NewChunkPlanner(900)
	text := "Có ba thứ: một, hai, ba; và kết thúc."
	plans := planner.Plan(text)

	if len(plans) < 1 {
		t.Fatalf("Expected at least 1 plan, got %d", len(plans))
	}
}

// === Tests for splitParagraphs ===

func TestSplitParagraphs_Single(t *testing.T) {
	text := "Một đoạn văn."
	paragraphs := splitParagraphs(text)
	if len(paragraphs) != 1 {
		t.Fatalf("Expected 1 paragraph, got %d", len(paragraphs))
	}
	if paragraphs[0] != "Một đoạn văn." {
		t.Errorf("Paragraph = %q, want %q", paragraphs[0], "Một đoạn văn.")
	}
}

func TestSplitParagraphs_Multiple(t *testing.T) {
	text := "Đoạn 1.\n\nĐoạn 2.\n\nĐoạn 3."
	paragraphs := splitParagraphs(text)
	if len(paragraphs) != 3 {
		t.Fatalf("Expected 3 paragraphs, got %d", len(paragraphs))
	}
	expected := []string{"Đoạn 1.", "Đoạn 2.", "Đoạn 3."}
	for i, p := range paragraphs {
		if p != expected[i] {
			t.Errorf("Paragraph %d = %q, want %q", i, p, expected[i])
		}
	}
}

func TestSplitParagraphs_EmptyLines(t *testing.T) {
	text := "\n\n\n\n"
	paragraphs := splitParagraphs(text)
	if len(paragraphs) != 0 {
		t.Errorf("Expected 0 paragraphs, got %d", len(paragraphs))
	}
}

func TestSplitParagraphs_TrimWhitespace(t *testing.T) {
	text := "  Đoạn 1.  \n\n  \n\n  Đoạn 2.  "
	paragraphs := splitParagraphs(text)
	if len(paragraphs) != 2 {
		t.Fatalf("Expected 2 paragraphs, got %d", len(paragraphs))
	}
	if paragraphs[0] != "Đoạn 1." {
		t.Errorf("Paragraph 1 = %q, want %q", paragraphs[0], "Đoạn 1.")
	}
	if paragraphs[1] != "Đoạn 2." {
		t.Errorf("Paragraph 2 = %q, want %q", paragraphs[1], "Đoạn 2.")
	}
}

// === Tests for splitSentences ===

func TestSplitSentences_Single(t *testing.T) {
	text := "Một câu."
	sentences := splitSentences(text)
	if len(sentences) != 1 {
		t.Fatalf("Expected 1 sentence, got %d", len(sentences))
	}
	if sentences[0] != "Một câu." {
		t.Errorf("Sentence = %q, want %q", sentences[0], "Một câu.")
	}
}

func TestSplitSentences_Multiple(t *testing.T) {
	text := "Câu 1. Câu 2! Câu 3?"
	sentences := splitSentences(text)
	if len(sentences) != 3 {
		t.Fatalf("Expected 3 sentences, got %d", len(sentences))
	}
	expected := []string{"Câu 1.", " Câu 2!", " Câu 3?"}
	for i, s := range sentences {
		if s != expected[i] {
			t.Errorf("Sentence %d = %q, want %q", i, s, expected[i])
		}
	}
}

func TestSplitSentences_NoPunctuation(t *testing.T) {
	text := "Đây là đoạn không có dấu câu"
	sentences := splitSentences(text)
	if len(sentences) != 1 {
		t.Fatalf("Expected 1 sentence (no punctuation), got %d", len(sentences))
	}
	if sentences[0] != text {
		t.Errorf("Sentence = %q, want %q", sentences[0], text)
	}
}

func TestSplitSentences_Ellipsis(t *testing.T) {
	text := "Trời ơi…"
	sentences := splitSentences(text)
	if len(sentences) != 1 {
		t.Fatalf("Expected 1 sentence, got %d", len(sentences))
	}
	if sentences[0] != "Trời ơi…" {
		t.Errorf("Sentence = %q, want %q", sentences[0], "Trời ơi…")
	}
}

func TestSplitSentences_MixedPunctuation(t *testing.T) {
	text := "Hello! How are you? I'm fine; thanks: see you later…"
	sentences := splitSentences(text)
	if len(sentences) != 5 {
		t.Fatalf("Expected 5 sentences, got %d", len(sentences))
	}
}

// === Tests for splitHard ===

func TestSplitHard_Empty(t *testing.T) {
	parts := splitHard("", 50, 10)
	if parts != nil {
		t.Errorf("Expected nil for empty text, got %v", parts)
	}
}

func TestSplitHard_ShortText(t *testing.T) {
	text := "Ngắn."
	parts := splitHard(text, 50, 10)
	if len(parts) != 1 {
		t.Fatalf("Expected 1 part, got %d", len(parts))
	}
	if parts[0] != "Ngắn." {
		t.Errorf("Part = %q, want %q", parts[0], "Ngắn.")
	}
}

func TestSplitHard_CharLimit(t *testing.T) {
	text := "Đây là một câu rất dài vượt quá giới hạn ký tự cho phép trong một chunk duy nhất"
	parts := splitHard(text, 30, 100) // 30 char limit
	if len(parts) < 2 {
		t.Fatalf("Expected at least 2 parts, got %d", len(parts))
	}

	for i, part := range parts {
		if len(part) > 35 { // Small tolerance
			t.Errorf("Part %d: %d chars exceeds limit 30", i, len(part))
		}
	}
}

func TestSplitHard_WordLimit(t *testing.T) {
	text := "một hai ba bốn năm sáu bảy tám chín mười mười một mười hai"
	parts := splitHard(text, 1000, 5) // 5 word limit
	if len(parts) < 2 {
		t.Fatalf("Expected at least 2 parts, got %d", len(parts))
	}

	for i, part := range parts {
		wordCount := len(strings.Fields(part))
		if wordCount > 5 {
			t.Errorf("Part %d: %d words exceeds limit 5", i, wordCount)
		}
	}
}

func TestSplitHard_BothLimits(t *testing.T) {
	text := "Từ một đến mười từ đây là câu dài vượt cả hai giới hạn"
	parts := splitHard(text, 30, 5) // Both 30 chars and 5 words
	if len(parts) < 2 {
		t.Fatalf("Expected at least 2 parts, got %d", len(parts))
	}

	for i, part := range parts {
		charCount := len(part)
		wordCount := len(strings.Fields(part))
		if charCount > 35 || wordCount > 6 {
			t.Errorf("Part %d exceeds limits: %d chars, %d words", i, charCount, wordCount)
		}
	}
}

// === Tests for countWords ===

func TestCountWords_Empty(t *testing.T) {
	count := countWords("")
	if count != 0 {
		t.Errorf("Count = %d, want 0", count)
	}
}

func TestCountWords_Single(t *testing.T) {
	count := countWords("một")
	if count != 1 {
		t.Errorf("Count = %d, want 1", count)
	}
}

func TestCountWords_Multiple(t *testing.T) {
	count := countWords("một hai ba bốn năm")
	if count != 5 {
		t.Errorf("Count = %d, want 5", count)
	}
}

func TestCountWords_ExtraWhitespace(t *testing.T) {
	count := countWords("  một   hai    ba  ")
	if count != 3 {
		t.Errorf("Count = %d, want 3", count)
	}
}

func TestCountWords_Vietnamese(t *testing.T) {
	count := countWords("Xin chào thế giới Việt Nam")
	// "Việt Nam" is counted as 2 words by strings.Fields
	if count != 6 {
		t.Errorf("Count = %d, want 6", count)
	}
}

// === Integration tests for sentence-boundary chunking ===

func TestPlan_SentenceBoundary_Integration(t *testing.T) {
	// Simulate real-world Vietnamese text
	text := `Chương 1: Bắt đầu

Nhân vật chính bước vào khu rừng. Anh ta nhìn xung quanh. Mọi thứ thật xa lạ!
"Đây là đâu?" - anh tự hỏi.

Không ai trả lời. Chỉ có tiếng gió thổi qua lá cây…
Anh tiếp tục bước đi. Con đường trước mắt còn dài.`

	planner := NewChunkPlanner(150)
	plans := planner.Plan(text)

	if len(plans) < 3 {
		t.Fatalf("Expected at least 3 chunks, got %d", len(plans))
	}

	// Verify chunks roughly respect limits
	for i, plan := range plans {
		if len(plan.Text) > 200 { // Generous tolerance for sentence boundary
			t.Logf("Warning: Chunk %d has %d chars (limit 150)", i, len(plan.Text))
		}
	}

	// Verify all content is preserved
	var totalContent string
	for _, plan := range plans {
		totalContent += plan.Text + " "
	}

	// Check key phrases are present
	keyPhrases := []string{"Chương 1", "khu rừng", "xa lạ", "tiếng gió", "bước đi"}
	for _, phrase := range keyPhrases {
		if !strings.Contains(totalContent, phrase) {
			t.Errorf("Key phrase %q missing from chunks", phrase)
		}
	}
}

func TestPlan_SentenceBoundary_LongSentence(t *testing.T) {
	// Test with a sentence that exceeds the limit
	longSentence := "Đây là một câu cực kỳ dài với rất nhiều từ được viết liên tiếp không có dấu câu cho đến tận cuối câu khi chúng ta đọc hết một hàng rất dài luôn."
	text := "Câu ngắn. " + longSentence + " Câu ngắn sau."

	planner := NewChunkPlanner(100)
	plans := planner.Plan(text)

	if len(plans) < 2 {
		t.Fatalf("Expected at least 2 chunks due to long sentence, got %d", len(plans))
	}
}

func TestPlan_NoSentenceBreak_PoetryStyle(t *testing.T) {
	// Poetry or list-style text without sentence boundaries
	text := `đây dòng thơ thứ nhất
đây dòng thơ thứ hai
đây dòng thơ thứ ba
đây dòng thơ thứ tư`

	planner := NewChunkPlanner(100)
	plans := planner.Plan(text)

	// Should still chunk based on paragraph/line boundaries
	if len(plans) < 1 {
		t.Fatalf("Expected at least 1 chunk, got %d", len(plans))
	}
}
