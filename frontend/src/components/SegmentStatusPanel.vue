<template>
  <div class="segment-status-card panel">
    <div class="column-head">
      <h4>📋 Tiến trình Realtime</h4>
      <span v-if="cursor" class="eyebrow">
        {{ cursor.chapterTitle }} • {{ cursor.segmentIndex + 1 }}/{{ cursor.totalSegments }}
      </span>
    </div>

    <div v-if="chapterGroups.length === 0" class="empty-segments">
      <p>Chưa có segment nào được render.</p>
    </div>

    <div v-for="group in chapterGroups" :key="group.chapterId" class="segment-chapter-group">
      <div class="segment-chapter-header">
        <span class="chapter-label">{{ group.chapterTitle }}</span>
        <span class="chapter-progress">{{ group.completedSegments }}/{{ group.totalSegments }}</span>
      </div>

      <div class="segment-list">
        <div
          v-for="segment in group.segments"
          :key="segment.index"
          class="segment-status-item"
          :class="{
            'is-reading': isSegmentReading(segment),
            'is-ready': segment.status === 'ready',
            'is-rendering': segment.status === 'rendering',
            'is-retrying': segment.status === 'retrying',
            'is-error': segment.status === 'error',
            'is-current': isSegmentCurrent(segment),
          }"
          @click="$emit('jump-to-segment', segment)"
        >
          <div class="segment-index">{{ segment.index + 1 }}</div>
          <div class="segment-text">{{ truncate(segment.text, 80) }}</div>
          <div class="segment-status-badge">{{ segmentStatusLabel(segment) }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { RealtimeChapterSegmentGroup, RealtimeSegmentItem, RealtimePlaybackCursor } from '../types'

interface Props {
  chapterGroups: RealtimeChapterSegmentGroup[]
  cursor: RealtimePlaybackCursor | null
}

interface Emits {
  (e: 'jump-to-segment', segment: RealtimeSegmentItem): void
}

defineProps<Props>()
defineEmits<Emits>()

function truncate(text: string, maxLen: number): string {
  if (!text) return ''
  return text.length > maxLen ? text.slice(0, maxLen) + '…' : text
}

function segmentStatusLabel(segment: RealtimeSegmentItem): string {
  switch (segment.status) {
    case 'ready': return '✅'
    case 'rendering': return '⏳'
    case 'retrying': return '🔄'
    case 'error': return '❌'
    case 'reading': return '🔊'
    default: return '⏸️'
  }
}

function isSegmentReading(segment: RealtimeSegmentItem): boolean {
  return segment.status === 'reading'
}

function isSegmentCurrent(segment: RealtimeSegmentItem): boolean {
  return segment.status === 'rendering' || segment.status === 'retrying'
}
</script>

<style scoped>
.segment-status-card {
  padding: 1rem;
  max-height: 60vh;
  overflow-y: auto;
}

.column-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
  padding-bottom: 0.75rem;
  border-bottom: 1px solid var(--color-border, #334155);
}

.column-head h4 {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 600;
}

.empty-segments {
  text-align: center;
  padding: 2rem 1rem;
  color: var(--color-text-muted, #94a3b8);
  font-size: 0.85rem;
}

.segment-chapter-group {
  margin-bottom: 1rem;
}

.segment-chapter-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.4rem 0.6rem;
  background: var(--color-bg-secondary, #1e293b);
  border-radius: 6px 6px 0 0;
  font-size: 0.8rem;
}

.chapter-label {
  color: var(--color-text, #e2e8f0);
  font-weight: 500;
}

.chapter-progress {
  color: var(--color-text-muted, #94a3b8);
}

.segment-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.segment-status-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 6px;
  background: var(--color-surface, #0f172a);
  border: 1px solid transparent;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
  font-size: 0.8rem;
}

.segment-status-item:hover {
  background: var(--color-bg-secondary, #1e293b);
  border-color: var(--color-border, #334155);
}

.segment-status-item.is-reading {
  background: var(--color-accent, #6366f1);
  color: white;
  border-color: var(--color-accent, #6366f1);
}

.segment-status-item.is-current {
  border-color: var(--color-accent, #6366f1);
  background: rgba(99, 102, 241, 0.1);
}

.segment-status-item.is-error {
  border-color: #ef4444;
  background: rgba(239, 68, 68, 0.1);
}

.segment-index {
  flex-shrink: 0;
  width: 22px;
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-bg-secondary, #1e293b);
  border-radius: 4px;
  font-size: 0.7rem;
  font-weight: 600;
}

.is-reading .segment-index {
  background: rgba(255, 255, 255, 0.2);
}

.segment-text {
  flex: 1;
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.segment-status-badge {
  flex-shrink: 0;
  font-size: 0.85rem;
}
</style>
