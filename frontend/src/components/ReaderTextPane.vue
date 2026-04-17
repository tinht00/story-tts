<template>
  <div class="reader-text-pane">
    <!-- Chapter Header -->
    <div class="reader-head">
      <div class="reader-head-info">
        <h2 class="chapter-title">{{ chapterContent?.chapterTitle }}</h2>
        <p class="chapter-meta">
          {{ selectedChapterPosition }}/{{ chapterWordCount }} từ
        </p>
      </div>
      <div class="reader-actions">
        <button class="ghost-button" @click="$emit('prev-chapter')" :disabled="!hasPrevChapter">
          ◀ Chương trước
        </button>
        <button class="ghost-button" @click="$emit('next-chapter')" :disabled="!hasNextChapter">
          Chương sau ▶
        </button>
        <button class="ghost-button" @click="$emit('back-to-library')">
          📋 Thư viện
        </button>
      </div>
    </div>

    <!-- Reader Body -->
    <div
      ref="readerBodyRef"
      class="reader-body"
      :style="{ fontSize: readerFontSize + 'px' }"
    >
      <div
        v-for="(block, blockIdx) in renderableBlocks"
        :key="blockIdx"
        class="reader-paragraph"
        :class="{
          'reader-heading': block.isHeading,
          'reader-divider': block.isDivider,
          'reader-spacer': block.isBlank,
        }"
        v-html="block.html"
      ></div>
    </div>

    <!-- Font Controls -->
    <div class="reader-font-controls">
      <span class="font-label">A-</span>
      <input
        type="range"
        class="font-size-slider"
        min="14"
        max="28"
        step="1"
        :value="readerFontSize"
        @input="$emit('update-font-size', Number(($event.target as HTMLInputElement).value))"
      />
      <span class="font-label">A+</span>
      <span class="font-size-value">{{ readerFontSize }}px</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { ChapterContent } from '../types'

interface RenderableBlock {
  html: string
  isHeading: boolean
  isDivider: boolean
  isBlank: boolean
}

interface Props {
  chapterContent: ChapterContent | null
  selectedChapterPosition: number
  chapterWordCount: number
  readerFontSize: number
  renderableBlocks: RenderableBlock[]
  hasPrevChapter: boolean
  hasNextChapter: boolean
}

interface Emits {
  (e: 'update-font-size', size: number): void
  (e: 'prev-chapter'): void
  (e: 'next-chapter'): void
  (e: 'back-to-library'): void
}

defineProps<Props>()
defineEmits<Emits>()

const readerBodyRef = ref<HTMLElement | null>(null)

defineExpose({ readerBodyRef })
</script>

<style scoped>
.reader-text-pane {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.reader-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
  padding: 1.1rem 1.35rem;
  border-bottom: 1px solid var(--color-border, #334155);
  flex-shrink: 0;
}

.reader-head-info {
  flex: 1;
  min-width: 0;
}

.chapter-title {
  margin: 0 0 0.25rem 0;
  font-size: 1.15rem;
  font-weight: 700;
  color: var(--color-text, #e2e8f0);
  line-height: 1.3;
}

.chapter-meta {
  margin: 0;
  font-size: 0.8rem;
  color: var(--color-text-muted, #94a3b8);
}

.reader-actions {
  display: flex;
  gap: 0.5rem;
  flex-shrink: 0;
}

.reader-body {
  flex: 1;
  overflow-y: auto;
  padding: 1.75rem 2rem 2.4rem;
  line-height: 1.8;
}

.reader-body > * {
  width: min(100%, 74ch);
  margin: 0 auto;
}

.reader-paragraph {
  margin-bottom: 1rem;
  color: var(--color-text, #e2e8f0);
}

.reader-heading {
  font-size: 1.2em;
  font-weight: 700;
  text-align: center;
  margin: 1.5rem 0 1rem;
  color: var(--color-accent, #6366f1);
}

.reader-divider {
  text-align: center;
  color: var(--color-text-muted, #94a3b8);
  font-size: 1.2em;
  letter-spacing: 0.3em;
  margin: 1.5rem 0;
  user-select: none;
}

.reader-spacer {
  height: 1em;
}

.reader-font-controls {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem 1.5rem;
  border-top: 1px solid var(--color-border, #334155);
  background: var(--color-surface, #0f172a);
  flex-shrink: 0;
}

.font-label {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--color-text-muted, #94a3b8);
  user-select: none;
}

.font-size-slider {
  flex: 1;
  height: 6px;
  border-radius: 3px;
  background: var(--color-bg-secondary, #1e293b);
  outline: none;
  cursor: pointer;
}

.font-size-value {
  font-size: 0.8rem;
  color: var(--color-text-muted, #94a3b8);
  min-width: 40px;
  text-align: right;
}

/* Responsive adjustments sẽ được xử lý bởi media queries trong App.vue */
</style>
