export type ProsodyPreset = "stable" | "gentle" | "tense" | "climax";

export interface Story {
  id: number;
  slug: string;
  title: string;
  sourceType: string;
  sourcePath: string;
  libraryPath: string;
  defaultPreset: ProsodyPreset;
  chapterCount: number;
  lastOpenedAt?: string;
  lastError?: string;
}

export interface Chapter {
  id: number;
  storyId: number;
  chapterIndex: number;
  title: string;
  libraryFilePath: string;
  preset: ProsodyPreset;
  lastError?: string;
}

export interface Artifact {
  id: number;
  storyId: number;
  chapterId?: number;
  kind: string;
  filePath: string;
}

export interface StoryDetail {
  story: Story;
  chapters: Chapter[];
  artifacts: Artifact[];
}

export interface ChapterContent {
  storyId: number;
  chapterId: number;
  chapterIndex: number;
  storyTitle: string;
  chapterTitle: string;
  text: string;
  characterCount: number;
  updatedAt: string;
}

export interface ReaderProgress {
  storyId: number;
  chapterIndex: number;
  scrollPercent: number;
  audioPositionSec: number;
  updatedAt?: string;
}

export interface DirectTTSResponse {
  audioUrl: string;
  cacheHit: boolean;
  preset: ProsodyPreset;
  voice: string;
  durationMs: number;
}

export interface DirectTTSChunk {
  index: number;
  wordCount: number;
  status: string;
  audioUrl?: string;
  cacheHit: boolean;
  error?: string;
}

export interface DirectTTSSession {
  id: string;
  storyId: number;
  chapterId: number;
  chapterTitle: string;
  status: string;
  preset: ProsodyPreset;
  voice: string;
  totalChunks: number;
  readyChunks: number;
  currentChunk: number;
  lastError?: string;
  chunks: DirectTTSChunk[];
  startedAt: string;
  updatedAt: string;
}

export interface AppState {
  config: {
    listenAddr: string;
    libraryDir: string;
    dataDir: string;
    ffmpegPath: string;
    edgeBinary: string;
    edgeVoice: string;
    realtimeTtsBaseUrl: string;
    realtimeDefaultVoice: string;
    realtimeDefaultSpeed: number;
    realtimeDefaultPitch: number;
  };
}

export interface ImportFolderChapter {
  relativePath: string;
  title: string;
  content: string;
}

export interface ImportFolderStory {
  relativePath: string;
  title: string;
  chapters: ImportFolderChapter[];
}

export interface ImportFolderRequest {
  rootName: string;
  stories: ImportFolderStory[];
}

export interface RealtimeVoice {
  id: string;
  name: string;
  locale: string;
  gender: string;
  friendlyName: string;
}

export interface RealtimeChapterPayload {
  chapterId: number;
  chapterIndex: number;
  title: string;
  text: string;
}

export interface RealtimeControlSettings {
  voice: string;
  speed: number;
  pitch: number;
  autoNext: boolean;
}

export interface RealtimeSession {
  id: string;
  storyId: number;
  chapterId: number;
  currentChapterIndex: number;
  status: string;
  voice: string;
  speed: number;
  pitch: number;
  autoNext: boolean;
}

export interface RealtimeSegmentItem {
  index: number;
  text: string;
  wordCount?: number;
  durationEstimate?: number;
  status?: string;
}

export interface RealtimeChapterSegmentGroup {
  chapterId: number;
  chapterIndex: number;
  chapterTitle: string;
  segments: RealtimeSegmentItem[];
  completedSegments: number;
  totalSegments: number;
}

export interface RealtimePlaybackCursor {
  chapterId: number;
  chapterTitle: string;
  chapterIndex: number;
  segmentIndex: number;
  totalSegments: number;
}

export interface RealtimeSessionState {
  type:
    | "session_started"
    | "controls_updated"
    | "audio_format"
    | "chapter_segments"
    | "chunk_started"
    | "chunk_finished"
    | "segment_rendering"
    | "segment_ready"
    | "segment_retry"
    | "segment_started"
    | "segment_finished"
    | "chapter_started"
    | "chapter_finished"
    | "chapter_transition"
    | "story_finished"
    | "stopped"
    | "error"
    | "stream_closed";
  sessionId?: string;
  storyId?: number;
  chapterId?: number;
  chapterIndex?: number;
  chapterTitle?: string;
  fromChapterId?: number;
  toChapterId?: number;
  chunkIndex?: number;
  totalChunks?: number;
  chunkText?: string;
  segmentIndex?: number;
  totalSegments?: number;
  segmentText?: string;
  voice?: string;
  speed?: number;
  pitch?: number;
  autoNext?: boolean;
  mime?: string;
  reason?: string;
  message?: string;
  status?: string;
  attempt?: number;
  segments?: RealtimeSegmentItem[];
  wordCount?: number;
  durationEstimate?: number;
  startSegmentIndex?: number;
}
