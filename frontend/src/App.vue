<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { api } from "./lib/api";
import { toast } from "./lib/toast";
import ToastContainer from "./components/ToastContainer.vue";
import { ReconnectWebSocket, buildWebSocketUrl } from "./lib/websocket";
import type {
    AppState,
    Chapter,
    ChapterContent,
    ImportFolderRequest,
    ProsodyPreset,
    ReaderProgress,
    RealtimeChapterPayload,
    RealtimeSegmentItem,
    RealtimeSession,
    RealtimeSessionState,
    RealtimeVoice,
    Story,
    StoryDetail,
} from "./types";

type PermissionMode = "read" | "readwrite";
type BrowserPermissionStatus = "granted" | "denied" | "prompt";
type BrowserPermissionDescriptor = { mode?: PermissionMode };
type BrowserFileHandle = FileSystemFileHandle & { getFile(): Promise<File> };
type BrowserDirectoryHandle = FileSystemDirectoryHandle & {
    values(): AsyncIterable<FileSystemHandle>;
    queryPermission(
        descriptor?: BrowserPermissionDescriptor,
    ): Promise<BrowserPermissionStatus>;
    requestPermission(
        descriptor?: BrowserPermissionDescriptor,
    ): Promise<BrowserPermissionStatus>;
};
type DirectoryPickerWindow = Window &
    typeof globalThis & {
        showDirectoryPicker?: (options?: {
            id?: string;
            mode?: "read" | "readwrite";
        }) => Promise<BrowserDirectoryHandle>;
    };

type ImportedChapterDraft = {
    relativePath: string;
    title: string;
    content: string;
};

type ImportedStoryDraft = {
    relativePath: string;
    title: string;
    chapters: ImportedChapterDraft[];
};

type ReaderBlock = {
    kind: "heading" | "body" | "divider" | "spacer";
    text: string;
    normalizedLength: number;
};

type LoadChapterOptions = {
    preservePlayback?: boolean;
    skipPersist?: boolean;
};

type RealtimeStatus =
    | "idle"
    | "connecting"
    | "buffering"
    | "reading"
    | "transitioning"
    | "stopped"
    | "finished"
    | "error";
type RealtimeSegmentStatus =
    | "queued"
    | "rendering"
    | "retrying"
    | "ready"
    | "reading"
    | "played";
type RealtimeSegmentState = {
    index: number;
    text: string;
    wordCount: number;
    status: RealtimeSegmentStatus;
    attempt: number;
    message: string;
    durationEstimate: number;
};
type RealtimeChapterSegmentGroup = {
    chapterId: number;
    chapterIndex: number;
    chapterTitle: string;
    status: "queued" | "rendering" | "reading" | "completed";
    startSegmentIndex: number;
    segments: RealtimeSegmentState[];
};
type RealtimePlaybackCursor = {
    chapterId: number;
    segmentIndex: number;
    audioTimeAtStart: number;
};
type RealtimePlaybackTimelineItem = {
    chapterId: number;
    chapterIndex: number;
    chapterTitle: string;
    segmentIndex: number;
    durationEstimate: number;
};
type ReaderInlineToken = {
    key: string;
    text: string;
    isWord: boolean;
    wordIndex: number | null;
};
type ReaderRenderableBlock = ReaderBlock & {
    tokens: ReaderInlineToken[];
};

const presets: ProsodyPreset[] = ["stable", "gentle", "tense", "climax"];
const handleDbName = "story-tts-reader";
const handleStoreName = "directory-handles";
const handleKey = "library-root";
const realtimePrefsKey = "story-tts.realtime.controls.v1";
const edgeReadAloudPrefsKey = "story-tts.edge-read-aloud.v1";
const readerPrefsKey = "story-tts.reader.preferences.v1";
const segmentInitialRenderWindowSize = 15;
const segmentRenderRefillThreshold = 10;
const segmentRenderRefillSize = 10;

const loading = ref(false);
const error = ref("");
const importProgress = ref<{
    active: boolean;
    phase: "reading" | "sending" | "processing" | "done" | "error";
    currentStory: number;
    totalStories: number;
    currentChapter: number;
    totalChapters: number;
    message: string;
}>({
    active: false,
    phase: "reading",
    currentStory: 0,
    totalStories: 0,
    currentChapter: 0,
    totalChapters: 0,
    message: "",
});
const state = ref<AppState | null>(null);
const stories = ref<Story[]>([]);
const selectedStory = ref<StoryDetail | null>(null);
const selectedChapterId = ref<number | null>(null);
const chapterContent = ref<ChapterContent | null>(null);
const readerProgress = ref<ReaderProgress | null>(null);
const progressByStory = ref<Record<number, ReaderProgress>>({});
const currentPreset = ref<ProsodyPreset>("stable");

const realtimeVoices = ref<RealtimeVoice[]>([]);
const selectedVoice = ref("");
const realtimeSpeed = ref(0);
const realtimePitch = ref(0);
const realtimeStatus = ref<RealtimeStatus>("idle");
const realtimeSession = ref<RealtimeSession | null>(null);
const realtimeBufferedChapterId = ref<number | null>(null);
const realtimeBufferedChapterTitle = ref("");
const realtimeAudibleChapterId = ref<number | null>(null);
const realtimeAudibleChapterTitle = ref("");
const realtimeError = ref("");
const realtimeServiceError = ref("");
const realtimeConnecting = ref(false);
const realtimeChapterGroups = ref<RealtimeChapterSegmentGroup[]>([]);
const realtimePlaybackCursor = ref<RealtimePlaybackCursor | null>(null);
const realtimePlaybackTimeline = ref<RealtimePlaybackTimelineItem[]>([]);
const useEdgeReadAloud = ref(false);
const edgeReadAloudActive = ref(false);
const edgeReadAloudWordsPerMinute = ref(185);

const currentDirectoryHandle = ref<BrowserDirectoryHandle | null>(null);
const currentLibraryRoot = ref("");
const currentStreamMime = ref("audio/mpeg");

const folderInput = ref<HTMLInputElement | null>(null);
const readerBody = ref<HTMLElement | null>(null);
const readerScanContent = ref<HTMLElement | null>(null);
const audioRef = ref<HTMLAudioElement | null>(null);

const contentCache = new Map<number, ChapterContent>();
let progressTimer: number | null = null;
let realtimeControlsSyncTimer: number | null = null;
let realtimeRestartTimer: number | null = null;
let pendingSeekFallbackTimer: number | null = null;
let activeSocket: ReconnectWebSocket | null = null;
let activeMediaSource: MediaSource | null = null;
let activeSourceBuffer: SourceBuffer | null = null;
let isPlaybackActive = false; // Theo dõi xem đang trong phiên playback không
let activeMediaUrl = "";
let queuedAudioChunks: Uint8Array[] = [];
let pendingMediaEnd = false;
let pendingResumePlayback: Promise<void> | null = null;
let userPausedAudio = false;
let edgeReadAloudTimer: number | null = null;
let dropIncomingAudioUntilSeekStart = false;
let resumeBufferThresholdSeconds = 4;
const activeTab = ref<"library" | "reader">("library");
const shouldAutoScroll = ref(false); // Chỉ scroll khi user đã scroll thủ công
let userHasScrolled = false;
const audioCurrentTime = ref(0);
const audioDuration = ref(0);
const audioIsPlaying = ref(false);
const readerFontSize = ref(18);
const readerPaneTab = ref<"text" | "console">("text");
const pendingSeekTarget = ref<{
    chapterId: number;
    segmentIndex: number;
} | null>(null);
const autoSyncingPlaybackChapterId = ref<number | null>(null);
const playbackAutoSyncLockChapterId = ref<number | null>(null);

const selectedChapter = computed<Chapter | null>(() => {
    if (!selectedStory.value || selectedChapterId.value === null) return null;
    return (
        selectedStory.value.chapters.find(
            (chapter) => chapter.id === selectedChapterId.value,
        ) ?? null
    );
});

const sortedStories = computed(() =>
    [...stories.value].sort((left, right) => {
        const leftTime = left.lastOpenedAt
            ? new Date(left.lastOpenedAt).getTime()
            : 0;
        const rightTime = right.lastOpenedAt
            ? new Date(right.lastOpenedAt).getTime()
            : 0;
        return (
            rightTime - leftTime || left.title.localeCompare(right.title, "vi")
        );
    }),
);

const recentStories = computed(() =>
    sortedStories.value.filter((story) => story.lastOpenedAt).slice(0, 4),
);

const selectedChapterPosition = computed(() => {
    if (!selectedStory.value || !selectedChapter.value) return "";
    return `${selectedChapter.value.chapterIndex}/${selectedStory.value.chapters.length}`;
});

const chapterWordCount = computed(() => {
    if (!chapterContent.value?.text) return 0;
    return chapterContent.value.text.split(/\s+/).filter(Boolean).length;
});
const isEdgeBrowser = computed(() => /Edg\//.test(window.navigator.userAgent));

const canRefreshLibrary = computed(
    () => Boolean(currentDirectoryHandle.value) || stories.value.length === 0,
);
const formattedChapterBlocks = computed(() =>
    formatChapterBlocks(
        chapterContent.value?.text ?? "",
        chapterContent.value?.storyTitle ?? "",
        chapterContent.value?.chapterTitle ?? "",
    ),
);

const isRealtimeActive = computed(
    () =>
        realtimeSession.value !== null &&
        ["connecting", "buffering", "reading", "transitioning"].includes(
            realtimeStatus.value,
        ),
);

const realtimeVoiceOptions = computed(() =>
    [...realtimeVoices.value].sort((left, right) =>
        `${left.locale} ${left.friendlyName}`.localeCompare(
            `${right.locale} ${right.friendlyName}`,
            "vi",
        ),
    ),
);

const currentVoiceLabel = computed(() => {
    const matched = realtimeVoiceOptions.value.find(
        (voice) => voice.id === selectedVoice.value,
    );
    return matched?.friendlyName ?? selectedVoice.value ?? "";
});

const sortedRealtimeChapterGroups = computed(() =>
    [...realtimeChapterGroups.value].sort(
        (left, right) => left.chapterIndex - right.chapterIndex,
    ),
);

const selectedRealtimeChapterGroup = computed(() => {
    const chapterId = selectedChapterId.value;
    if (chapterId === null) return null;
    return (
        sortedRealtimeChapterGroups.value.find(
            (group) => group.chapterId === chapterId,
        ) ?? null
    );
});

const currentPlaybackChapterId = computed<number | null>(() => {
    return (
        actualPlaybackLocation.value?.chapterId ??
        realtimeAudibleChapterId.value ??
        realtimePlaybackCursor.value?.chapterId ??
        null
    );
});

const currentPlaybackChapterGroup = computed(() => {
    const chapterId = currentPlaybackChapterId.value;
    if (chapterId === null) return null;
    return (
        sortedRealtimeChapterGroups.value.find(
            (group) => group.chapterId === chapterId,
        ) ?? null
    );
});

const playingChapterGroup = computed(() => {
    const chapterId = currentPlaybackChapterId.value ?? selectedChapterId.value;
    if (chapterId === null) return null;
    return (
        sortedRealtimeChapterGroups.value.find(
            (group) => group.chapterId === chapterId,
        ) ?? null
    );
});

const currentPlayingChapterTitle = computed(() => {
    const group = currentPlaybackChapterGroup.value ?? playingChapterGroup.value;
    if (group?.chapterTitle) {
        return group.chapterTitle;
    }
    if (realtimeAudibleChapterTitle.value) {
        return realtimeAudibleChapterTitle.value;
    }
    if (selectedChapter.value?.title) {
        return selectedChapter.value.title;
    }
    if (chapterContent.value?.chapterTitle) {
        return chapterContent.value.chapterTitle;
    }
    if (typeof group?.chapterIndex === "number" && group.chapterIndex > 0) {
        return `Chương ${group.chapterIndex}`;
    }
    return "Chương hiện tại";
});

const currentPlaybackSegment = computed<RealtimeSegmentState | null>(() => {
    const actual = actualPlaybackLocation.value;
    if (!actual) return null;
    const group = sortedRealtimeChapterGroups.value.find(
        (item) => item.chapterId === actual.chapterId,
    );
    if (!group) return null;
    return (
        group.segments.find(
            (segment) => segment.index === actual.segmentIndex,
        ) ?? null
    );
});

function getApproxSegmentDuration(segment: RealtimeSegmentState) {
    if (segment.durationEstimate > 0) {
        return Math.max(segment.durationEstimate, 0.8);
    }
    if (segment.wordCount > 0) {
        return Math.max(segment.wordCount / 3.2, 0.8);
    }
    return 1;
}

function resolvePlaybackTimelineDuration(entry: RealtimePlaybackTimelineItem) {
    const group = sortedRealtimeChapterGroups.value.find(
        (item) => item.chapterId === entry.chapterId,
    );
    const segment = group?.segments.find(
        (item) => item.index === entry.segmentIndex,
    );
    if (segment) {
        return getApproxSegmentDuration(segment);
    }
    if (entry.durationEstimate > 0) {
        return Math.max(entry.durationEstimate, 0.8);
    }
    return 1;
}

const actualPlaybackLocation = computed<{
    chapterId: number;
    segmentIndex: number;
    progress: number;
} | null>(() => {
    const timeline = realtimePlaybackTimeline.value;
    if (timeline.length === 0) {
        const cursor = realtimePlaybackCursor.value;
        if (!cursor) return null;
        return {
            chapterId: cursor.chapterId,
            segmentIndex: cursor.segmentIndex,
            progress: 0,
        };
    }

    let elapsed = Math.max(0, audioCurrentTime.value);

    for (let position = 0; position < timeline.length; position++) {
        const item = timeline[position];
        const duration = resolvePlaybackTimelineDuration(item);
        if (elapsed <= duration || position === timeline.length - 1) {
            return {
                chapterId: item.chapterId,
                segmentIndex: item.segmentIndex,
                progress: Math.min(1, duration > 0 ? elapsed / duration : 0),
            };
        }
        elapsed -= duration;
    }

    const lastSegment = timeline[timeline.length - 1];
    return {
        chapterId: lastSegment.chapterId,
        segmentIndex: lastSegment.segmentIndex,
        progress: 1,
    };
});

const activePlayingSegment = computed<RealtimeSegmentState | null>(() => {
    const group = playingChapterGroup.value;
    if (!group) return null;

    const actual = actualPlaybackLocation.value;
    if (actual && actual.chapterId === group.chapterId) {
        const actualSegment = group.segments.find(
            (segment) => segment.index === actual.segmentIndex,
        );
        if (actualSegment) {
            return actualSegment;
        }
    }

    const lastTimelineSegment = [...realtimePlaybackTimeline.value]
        .reverse()
        .find((entry) => entry.chapterId === group.chapterId);
    if (lastTimelineSegment) {
        return (
            group.segments.find(
                (segment) => segment.index === lastTimelineSegment.segmentIndex,
            ) ?? null
        );
    }

    return null;
});

const currentSegmentElapsedSeconds = computed(() => {
    const segment = activePlayingSegment.value;
    const actual = actualPlaybackLocation.value;
    if (!segment || !actual) return 0;
    return getApproxSegmentDuration(segment) * actual.progress;
});

const currentSegmentDurationSeconds = computed(() => {
    const estimatedDuration = activePlayingSegment.value?.durationEstimate ?? 0;
    if (estimatedDuration > 0) {
        return estimatedDuration;
    }
    if (!isRealtimeActive.value && Number.isFinite(audioDuration.value)) {
        return Math.max(audioDuration.value, 0);
    }
    return 0;
});

const currentSegmentProgressPercent = computed(() => {
    const actual = actualPlaybackLocation.value;
    if (!actual) return 0;
    return Math.min(100, actual.progress * 100);
});

const currentSegmentProgressLabel = computed(() => {
    const group = playingChapterGroup.value;
    const segment = activePlayingSegment.value;
    if (!group || !segment) {
        return "Player đang chờ xác định segment audio hiện tại";
    }
    return `${currentPlayingChapterTitle.value} • segment ${segment.index + 1}/${group.segments.length}`;
});

const currentSegmentPreviewText = computed(() => {
    const segment = activePlayingSegment.value;
    if (!segment?.text) {
        return "Chưa xác định được segment audio đang phát.";
    }
    return segment.text;
});

function getVisibleSegmentWindowForGroup(group: RealtimeChapterSegmentGroup | null) {
    if (!group) {
        return {
            start: 0,
            end: 0,
            items: [] as RealtimeSegmentState[],
        };
    }

    const start = Math.max(0, group.startSegmentIndex);
    const renderFrontierIndex = group.segments.reduce((maxIndex, segment) => {
        if (segment.status === "queued") {
            return maxIndex;
        }
        return Math.max(maxIndex, segment.index);
    }, Math.max(start, (actualPlaybackLocation.value?.chapterId === group.chapterId
        ? actualPlaybackLocation.value.segmentIndex
        : start)));
    const end = Math.min(group.segments.length, renderFrontierIndex + 1);

    return {
        start,
        end,
        items: group.segments.slice(start, end),
    };
}

const visiblePlayingSegmentWindow = computed(() =>
    getVisibleSegmentWindowForGroup(playingChapterGroup.value),
);

function getVisibleSegmentRangeForGroup(group: RealtimeChapterSegmentGroup | null) {
    const window = getVisibleSegmentWindowForGroup(group);
    if (!group || window.items.length === 0) return "";
    return `Hiển thị segment ${window.start + 1}-${window.end}/${group.segments.length}; khởi tạo ${segmentInitialRenderWindowSize} segment và khi còn ${segmentRenderRefillThreshold} segment phía trước sẽ nạp thêm ${segmentRenderRefillSize} segment tiếp theo.`;
}

const visiblePlayingSegmentRange = computed(() => {
    return getVisibleSegmentRangeForGroup(playingChapterGroup.value);
});

const renderChapterGroups = computed(() =>
    sortedRealtimeChapterGroups.value
        .map((group) => ({
            group,
            window: getVisibleSegmentWindowForGroup(group),
            note: getVisibleSegmentRangeForGroup(group),
            displayState: getRealtimeChapterDisplayState(group),
            isPlaybackChapter: currentPlaybackChapterId.value === group.chapterId,
        }))
        .filter((entry) => entry.window.items.length > 0),
);

const bufferedAheadSummary = computed(() => {
    if (
        realtimeBufferedChapterId.value === null ||
        realtimeBufferedChapterId.value === currentPlaybackChapterId.value
    ) {
        return "";
    }

    const bufferedGroup =
        sortedRealtimeChapterGroups.value.find(
            (group) => group.chapterId === realtimeBufferedChapterId.value,
        ) ?? null;
    const bufferedTitle =
        bufferedGroup?.chapterTitle ?? realtimeBufferedChapterTitle.value;
    if (!bufferedTitle) {
        return "";
    }

    return `Backend dang nap ahead toi ${bufferedTitle}; audio van bam theo chapter dang phat thuc te.`;
});

const showReaderConsole = computed(
    () => !useEdgeReadAloud.value && readerPaneTab.value === "console",
);

const showReaderText = computed(
    () => useEdgeReadAloud.value || readerPaneTab.value === "text",
);

const textHighlightLeadSeconds = 0.2;

const readerTargetsDifferentRealtimeChapter = computed(() => {
    if (useEdgeReadAloud.value) return false;
    if (!selectedChapterId.value || !currentPlaybackChapterId.value) return false;
    return selectedChapterId.value !== currentPlaybackChapterId.value;
});

const readerPlayButtonIsPlaying = computed(() => {
    if (useEdgeReadAloud.value) {
        return edgeReadAloudActive.value;
    }
    return !readerTargetsDifferentRealtimeChapter.value && audioIsPlaying.value;
});

const readerPlayButtonLabel = computed(() => {
    if (useEdgeReadAloud.value) {
        return edgeReadAloudActive.value ? "Tạm dừng Edge" : "Phát Edge";
    }
    if (readerTargetsDifferentRealtimeChapter.value) {
        return "Phát chương này";
    }
    return audioIsPlaying.value ? "Tạm dừng" : "Phát";
});

// Calculate dynamic segment status based on actual playback position
// Returns: "reading" | "played" | "queued"
function getDynamicSegmentStatus(
    segmentIndex: number,
    groupChapterId: number,
): RealtimeSegmentStatus {
    const group = sortedRealtimeChapterGroups.value.find(
        (item) => item.chapterId === groupChapterId,
    );
    const segment = group?.segments.find((item) => item.index === segmentIndex);

    const actual = actualPlaybackLocation.value;
    if (actual && actual.chapterId === groupChapterId) {
        if (segmentIndex < actual.segmentIndex) {
            return "played";
        }
        if (segmentIndex === actual.segmentIndex) {
            return "reading";
        }
    }
    if (segment?.status === "rendering" || segment?.status === "retrying") {
        return segment.status;
    }
    if (segment?.status === "ready") {
        return "ready";
    }
    return "queued";
}

function chapterGroupHasBufferedAudio(group: RealtimeChapterSegmentGroup) {
    return group.segments.some(
        (segment) =>
            segment.durationEstimate > 0 ||
            ["ready", "played", "reading", "rendering", "retrying"].includes(
                segment.status,
            ),
    );
}

function getRealtimeChapterDisplayState(group: RealtimeChapterSegmentGroup) {
    if (actualPlaybackLocation.value?.chapterId === group.chapterId) {
        return {
            tone: "reading",
            label: "Dang phat",
        } as const;
    }

    const hasRenderingWork = group.segments.some((segment) =>
        ["rendering", "retrying"].includes(segment.status),
    );
    if (hasRenderingWork || realtimeBufferedChapterId.value === group.chapterId) {
        return {
            tone: "rendering",
            label:
                realtimeBufferedChapterId.value === group.chapterId
                    ? "Dang nap truoc"
                    : "Dang tao tiep",
        } as const;
    }

    const hasBufferedAudio = chapterGroupHasBufferedAudio(group);
    if (!hasBufferedAudio) {
        return {
            tone: "queued",
            label: "Da tach",
        } as const;
    }

    if (
        currentPlaybackChapterGroup.value &&
        group.chapterIndex < currentPlaybackChapterGroup.value.chapterIndex
    ) {
        return {
            tone: "played",
            label: "Da phat qua",
        } as const;
    }

    return {
        tone: "ready",
        label: "Da nap truoc",
    } as const;
}

function isSegmentNowPlaying(
    segment: RealtimeSegmentState,
    groupChapterId: number,
) {
    return getDynamicSegmentStatus(segment.index, groupChapterId) === "reading";
}

const realtimeSegmentMetrics = computed(() => {
    const segmentEntries = sortedRealtimeChapterGroups.value.flatMap((group) =>
        group.segments.map((segment) => ({
            group,
            segment,
            dynamicStatus: getDynamicSegmentStatus(segment.index, group.chapterId),
        })),
    );
    const total = segmentEntries.length;
    const played = segmentEntries.filter(
        (entry) => entry.dynamicStatus === "played",
    ).length;
    const ready = segmentEntries.filter(
        (entry) => entry.dynamicStatus === "ready",
    ).length;
    const rendering = segmentEntries.filter((entry) =>
        ["rendering", "retrying"].includes(entry.dynamicStatus),
    ).length;
    const reading = segmentEntries.filter(
        (entry) => entry.dynamicStatus === "reading",
    ).length;
    return { total, played, ready, rendering, reading };
});

const renderableChapterBlocks = computed<ReaderRenderableBlock[]>(() => {
    let wordIndex = 0;
    return formattedChapterBlocks.value.map((block, blockIndex) => {
        if (block.kind === "spacer") {
            return { ...block, tokens: [] };
        }

        const parts = block.text.match(/\S+|\s+/g) ?? [];
        const tokens = parts.map((part, tokenIndex) => {
            const isWord = /\S/.test(part);
            const token: ReaderInlineToken = {
                key: `${blockIndex}-${tokenIndex}-${isWord ? wordIndex : "space"}`,
                text: part,
                isWord,
                wordIndex: isWord ? wordIndex : null,
            };
            if (isWord) {
                wordIndex += 1;
            }
            return token;
        });
        return { ...block, tokens };
    });
});

const activeWordGlobalIndex = computed<number | null>(() => {
    const playback = actualPlaybackLocation.value;
    const group = selectedRealtimeChapterGroup.value;
    if (!playback || !group || playback.chapterId !== group.chapterId)
        return null;

    const targetSegment = group.segments.find(
        (segment) => segment.index === playback.segmentIndex,
    );
    if (!targetSegment || targetSegment.wordCount <= 0) return null;

    let startWord = 0;
    for (const segment of group.segments) {
        if (segment.index === playback.segmentIndex) break;
        startWord += segment.wordCount;
    }

    const segmentDuration = Math.max(
        0.8,
        getApproxSegmentDuration(targetSegment),
    );
    // Đẩy highlight đi sớm hơn một nhịp nhỏ để mắt bám sát tiếng đọc hơn.
    const progress = Math.min(
        0.999,
        Math.max(
            0,
            playback.progress + textHighlightLeadSeconds / segmentDuration,
        ),
    );
    const offset = Math.min(
        targetSegment.wordCount - 1,
        Math.floor(progress * targetSegment.wordCount),
    );
    return startWord + offset;
});

// Sentence-level highlight: word range of current segment being read
// === FIX: Only highlight when the segment's chapter matches the displayed chapter ===
// Prevents highlight from jumping to pre-rendered chapters
const activeSegmentWordRange = computed<{ start: number; end: number } | null>(
    () => {
        const playback = actualPlaybackLocation.value;
        const group = selectedRealtimeChapterGroup.value;

        // Only highlight if playback chapter matches displayed chapter
        if (!playback || !group) return null;
        if (playback.chapterId !== group.chapterId) return null;
        if (playback.chapterId !== selectedChapterId.value) return null;

        const targetSegment = group.segments.find(
            (segment) => segment.index === playback.segmentIndex,
        );
        if (!targetSegment || targetSegment.wordCount <= 0) return null;

        let startWord = 0;
        for (const segment of group.segments) {
            if (segment.index === playback.segmentIndex) break;
            startWord += segment.wordCount;
        }

        return { start: startWord, end: startWord + targetSegment.wordCount };
    },
);

function isWordInActiveSegment(wordIndex: number | null): boolean {
    if (wordIndex === null || !activeSegmentWordRange.value) return false;
    return (
        wordIndex >= activeSegmentWordRange.value.start &&
        wordIndex < activeSegmentWordRange.value.end
    );
}

const realtimeStatusLabel = computed(() => {
    if (pendingSeekTarget.value) {
        return "Đang nhảy đoạn";
    }
    switch (realtimeStatus.value) {
        case "connecting":
            return "Đang kết nối";
        case "buffering":
            return "Đang tải audio";
        case "reading":
            return "Đang đọc";
        case "transitioning":
            return "Đổi chương";
        case "stopped":
            return "Đã dừng";
        case "finished":
            return "Hoàn tất";
        case "error":
            return "Có lỗi";
        default:
            return "Sẵn sàng";
    }
});

const realtimeStatusText = computed(() => {
    if (realtimeError.value) return realtimeError.value;
    if (realtimeServiceError.value) return realtimeServiceError.value;
    if (pendingSeekTarget.value) {
        return `Đang chuyển tới segment ${pendingSeekTarget.value.segmentIndex + 1} đã chọn và tận dụng cache audio hiện có nếu đã render.`;
    }

    // === FIX: Use displayed chapter title instead of TTS cursor chapter ===
    const displayChapterTitle =
        selectedChapter.value?.title ||
        chapterContent.value?.chapterTitle ||
        "chương hiện tại";

    switch (realtimeStatus.value) {
        case "connecting":
            return "Đang mở phiên realtime TTS và kết nối stream audio...";
        case "buffering":
            return `Đang nhận audio realtime cho ${displayChapterTitle}.`;
        case "reading":
            return `Đang đọc ${displayChapterTitle} và sẽ tự chuyển chương sau khi đọc xong.`;
        case "transitioning":
            return "Đang chuyển sang chương kế tiếp...";
        case "stopped":
            return "Đã dừng phiên đọc realtime.";
        case "finished":
            return "Đã đọc hết truyện hiện tại.";
        case "error":
            return "Luồng realtime TTS gặp lỗi.";
        default:
            return "Realtime TTS sẽ stream trực tiếp, không tạo file audio trung gian.";
    }
});

const currentStoryProgress = computed(() => {
    const storyId = selectedStory.value?.story.id;
    if (!storyId) return null;
    return progressByStory.value[storyId] ?? null;
});

const progressPercentage = computed(() => {
    const p = importProgress.value;
    if (!p.active || p.totalChapters === 0) return 0;

    switch (p.phase) {
        case "reading":
            return Math.min(
                30,
                (p.currentChapter / Math.max(1, p.totalChapters)) * 30,
            );
        case "sending":
            return 30 + (p.currentStory / Math.max(1, p.totalStories)) * 30;
        case "processing":
            return 60 + (p.currentStory / Math.max(1, p.totalStories)) * 30;
        case "done":
            return 100;
        case "error":
            return 0;
        default:
            return 0;
    }
});

const progressTitle = computed(() => {
    switch (importProgress.value.phase) {
        case "reading":
            return "📖 Đang đọc file";
        case "sending":
            return "📤 Đang gửi lên server";
        case "processing":
            return "⚙️ Đang xử lý";
        case "done":
            return "✅ Hoàn thành";
        case "error":
            return "❌ Lỗi import";
        default:
            return "Import...";
    }
});

function isPhaseDone(
    phase: "reading" | "sending" | "processing" | "done" | "error",
): boolean {
    const phases = ["reading", "sending", "processing", "done"];
    const currentIndex = phases.indexOf(
        importProgress.value.phase === "error"
            ? "done"
            : importProgress.value.phase,
    );
    const targetIndex = phases.indexOf(phase);
    return currentIndex > targetIndex;
}

function formatDateTime(value?: string) {
    if (!value) return "";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "";
    return new Intl.DateTimeFormat("vi-VN", {
        hour: "2-digit",
        minute: "2-digit",
        day: "2-digit",
        month: "2-digit",
    }).format(date);
}

function formatDuration(seconds: number) {
    if (isNaN(seconds) || seconds === Infinity) return "0:00";
    const mins = Math.floor(seconds / 60);
    const secs = Math.floor(seconds % 60);
    return `${mins}:${secs.toString().padStart(2, "0")}`;
}

function resetRealtimeSegments() {
    realtimeChapterGroups.value = [];
    realtimePlaybackCursor.value = null;
    realtimePlaybackTimeline.value = [];
    pendingSeekTarget.value = null;
    playbackAutoSyncLockChapterId.value = null;
    dropIncomingAudioUntilSeekStart = false;
}

function resetPlaybackTimeline() {
    realtimePlaybackTimeline.value = [];
    realtimePlaybackCursor.value = null;
}

function appendPlaybackTimelineEntry(payload: {
    chapterId: number;
    chapterIndex?: number;
    chapterTitle?: string;
    segmentIndex: number;
    durationEstimate?: number;
}) {
    const lastEntry =
        realtimePlaybackTimeline.value[realtimePlaybackTimeline.value.length - 1];
    if (
        lastEntry &&
        lastEntry.chapterId === payload.chapterId &&
        lastEntry.segmentIndex === payload.segmentIndex
    ) {
        return;
    }

    realtimePlaybackTimeline.value = [
        ...realtimePlaybackTimeline.value,
        {
            chapterId: payload.chapterId,
            chapterIndex: payload.chapterIndex ?? 0,
            chapterTitle:
                payload.chapterTitle ??
                `Chương ${payload.chapterIndex ?? ""}`.trim(),
            segmentIndex: payload.segmentIndex,
            durationEstimate: payload.durationEstimate ?? 0,
        },
    ];

    if (realtimePlaybackCursor.value === null) {
        realtimePlaybackCursor.value = {
            chapterId: payload.chapterId,
            segmentIndex: payload.segmentIndex,
            audioTimeAtStart: 0,
        };
    }
}

function buildRealtimeSegmentState(
    item: RealtimeSegmentItem,
): RealtimeSegmentState {
    return {
        index: item.index,
        text: item.text,
        wordCount:
            item.wordCount ?? item.text.split(/\s+/).filter(Boolean).length,
        status: "queued",
        attempt: 1,
        message: "",
        durationEstimate: item.durationEstimate ?? 0,
    };
}

function ensureRealtimeChapterGroup(
    chapterId: number,
    chapterIndex?: number,
    chapterTitle?: string,
) {
    const found = realtimeChapterGroups.value.find(
        (group) => group.chapterId === chapterId,
    );
    if (found) return found;

    const fallback: RealtimeChapterSegmentGroup = {
        chapterId,
        chapterIndex: chapterIndex ?? 0,
        chapterTitle: chapterTitle ?? `Chương ${chapterIndex ?? ""}`.trim(),
        status: "queued",
        startSegmentIndex: 0,
        segments: [],
    };
    realtimeChapterGroups.value = [...realtimeChapterGroups.value, fallback];
    return (
        realtimeChapterGroups.value.find(
            (group) => group.chapterId === chapterId,
        ) ?? fallback
    );
}

function ensureRealtimeSegment(
    chapterId: number,
    index: number,
    totalSegments?: number,
) {
    const group = ensureRealtimeChapterGroup(chapterId);
    const found = group.segments.find((segment) => segment.index === index);
    if (found) return found;

    const fallback: RealtimeSegmentState = {
        index,
        text: totalSegments
            ? `Segment ${index + 1}/${totalSegments}`
            : `Segment ${index + 1}`,
        wordCount: 0,
        status: "queued",
        attempt: 1,
        message: "",
        durationEstimate: 0,
    };
    realtimeChapterGroups.value = realtimeChapterGroups.value.map((item) =>
        item.chapterId === chapterId
            ? {
                  ...item,
                  segments: [...item.segments, fallback].sort(
                      (left, right) => left.index - right.index,
                  ),
              }
            : item,
    );
    return (
        ensureRealtimeChapterGroup(chapterId).segments.find(
            (segment) => segment.index === index,
        ) ?? fallback
    );
}

function patchRealtimeChapterGroup(
    chapterId: number,
    patch: Partial<RealtimeChapterSegmentGroup>,
) {
    const safePatch = Object.fromEntries(
        Object.entries(patch).filter(([, value]) => value !== undefined),
    ) as Partial<RealtimeChapterSegmentGroup>;
    const current = ensureRealtimeChapterGroup(
        chapterId,
        safePatch.chapterIndex,
        safePatch.chapterTitle,
    );
    realtimeChapterGroups.value = realtimeChapterGroups.value.map((group) =>
        group.chapterId === chapterId ? { ...current, ...safePatch } : group,
    );
}

function rewindRealtimeChapterFromSegment(
    chapterId: number,
    segmentIndex: number,
) {
    realtimeChapterGroups.value = realtimeChapterGroups.value.map((group) =>
        group.chapterId === chapterId
            ? {
                  ...group,
                  startSegmentIndex: segmentIndex,
                  segments: group.segments.map((segment) =>
                      segment.index >= segmentIndex &&
                      segment.status === "played"
                          ? { ...segment, status: "ready", message: "" }
                          : segment,
                  ),
              }
            : group,
    );
}

function patchRealtimeSegment(
    chapterId: number,
    index: number,
    patch: Partial<RealtimeSegmentState>,
    totalSegments?: number,
) {
    const current = ensureRealtimeSegment(chapterId, index, totalSegments);
    realtimeChapterGroups.value = realtimeChapterGroups.value.map((group) =>
        group.chapterId === chapterId
            ? {
                  ...group,
                  segments: group.segments
                      .map((segment) =>
                          segment.index === index
                              ? { ...current, ...patch }
                              : segment,
                      )
                      .sort((left, right) => left.index - right.index),
              }
            : group,
    );
}

function syncRealtimeSegments(
    chapterId: number,
    chapterIndex: number,
    chapterTitle: string,
    items: RealtimeSegmentItem[],
    startSegmentIndex = 0,
) {
    const current = ensureRealtimeChapterGroup(
        chapterId,
        chapterIndex,
        chapterTitle,
    );
    const existingByIndex = new Map(
        current.segments.map((segment) => [segment.index, segment]),
    );
    realtimeChapterGroups.value = realtimeChapterGroups.value.map((group) =>
        group.chapterId === chapterId
            ? {
                  ...current,
                  chapterIndex,
                  chapterTitle,
                  startSegmentIndex,
                  status:
                      group.status === "completed"
                          ? "completed"
                          : current.status,
                  segments: items
                      .map((item) => {
                          const existing = existingByIndex.get(item.index);
                          return existing
                              ? {
                                    ...existing,
                                    text: item.text,
                                    wordCount:
                                        item.wordCount ?? existing.wordCount,
                                    durationEstimate:
                                        item.durationEstimate ??
                                        existing.durationEstimate,
                                }
                              : buildRealtimeSegmentState(item);
                      })
                      .sort((left, right) => left.index - right.index),
              }
            : group,
    );
}

function realtimeSegmentStatusLabel(status: RealtimeSegmentStatus) {
    switch (status) {
        case "rendering":
            return "Đang tạo";
        case "retrying":
            return "Đang retry";
        case "ready":
            return "Sẵn sàng";
        case "reading":
            return "Đang đọc";
        case "played":
            return "Đã đọc";
        default:
            return "Đã tách";
    }
}

function getSegmentRenderProgress(status: RealtimeSegmentStatus) {
    switch (status) {
        case "ready":
            return 100;
        case "retrying":
            return 72;
        case "rendering":
            return 38;
        default:
            return 0;
    }
}

function getSegmentRenderLabel(segment: RealtimeSegmentState) {
    switch (segment.status) {
        case "ready":
            return "100%";
        case "retrying":
            return `Retry lần ${segment.attempt}`;
        case "rendering":
            return "Đang tạo";
        default:
            return "Chưa tạo";
    }
}

function getSegmentPlaybackProgress(
    segment: RealtimeSegmentState,
    groupChapterId: number,
) {
    const dynamicStatus = getDynamicSegmentStatus(segment.index, groupChapterId);
    if (dynamicStatus === "played") return 100;
    if (dynamicStatus !== "reading") return 0;

    const actual = actualPlaybackLocation.value;
    if (!actual) return 0;
    if (
        actual.chapterId !== groupChapterId ||
        actual.segmentIndex !== segment.index
    ) {
        return 0;
    }

    return Math.min(100, actual.progress * 100);
}

function collateNatural(left: string, right: string) {
    return left.localeCompare(right, "vi", {
        numeric: true,
        sensitivity: "base",
    });
}

function formatChapterBlocks(
    text: string,
    storyTitle: string,
    chapterTitle: string,
): ReaderBlock[] {
    const rawNormalized = text
        .replace(/\u00a0/g, " ")
        .replace(/\r\n/g, "\n")
        .replace(/\r/g, "\n")
        .trim();
    if (!rawNormalized) return [];

    const blocks: ReaderBlock[] = [];
    const lines = rawNormalized.split("\n");

    for (const line of lines) {
        const trimmedLine = line.trim();

        if (!trimmedLine) {
            if (blocks.at(-1)?.kind !== "spacer") {
                blocks.push({ kind: "spacer", text: "", normalizedLength: 0 });
            }
            continue;
        }

        if (isDividerLine(trimmedLine)) {
            blocks.push({
                kind: "divider",
                text: trimmedLine,
                normalizedLength: 0,
            });
            continue;
        }

        if (isHeadingLine(trimmedLine)) {
            blocks.push({
                kind: "heading",
                text: trimmedLine,
                normalizedLength: countNormalizedLength(trimmedLine),
            });
            continue;
        }

        blocks.push({
            kind: "body",
            text: line,
            normalizedLength: countNormalizedLength(trimmedLine),
        });
    }

    return blocks;
}

function isDividerLine(value: string) {
    return /^[-=_*~.]{5,}$/.test(value.replace(/\s+/g, ""));
}

function isHeadingLine(value: string) {
    return /^(chuong|chương|quyen|quyển|phan|phần|tap|tập)\b/i.test(value);
}

function normalizeTtsText(value: string) {
    return value
        .replace(/\r\n/g, "\n")
        .replace(/\r/g, "\n")
        .replace(/[ \t]+/g, " ")
        .replace(/\n{3,}/g, "\n\n")
        .replace(/[-=_*~]{5,}/g, " ")
        .replace(/\s+([,.;:!?])/g, "$1")
        .replace(/\s+/g, " ")
        .trim();
}

function countNormalizedLength(value: string) {
    const normalized = normalizeTtsText(value);
    return normalized ? normalized.length : 0;
}

function openLegacyFolderPicker() {
    folderInput.value?.click();
}

async function refresh(
    preferredStoryId?: number,
    preferredStorySlug?: string,
    lastRead?: LastReadPosition,
) {
    loading.value = true;
    error.value = "";
    try {
        const [nextState, nextStories] = await Promise.all([
            api.state(),
            api.stories(),
        ]);
        state.value = nextState;
        applyRealtimeDefaults(nextState);
        await ensureRealtimeVoices(nextState.config.realtimeTtsBaseUrl);

        stories.value = nextStories.items ?? [];
        await hydrateStoryProgress(stories.value);
        if (stories.value.length === 0) {
            selectedStory.value = null;
            selectedChapterId.value = null;
            chapterContent.value = null;
            readerProgress.value = null;
            return;
        }

        const targetStoryId = resolveStoryId(
            preferredStoryId,
            preferredStorySlug,
        );
        if (targetStoryId) {
            await loadStory(targetStoryId);

            // Resume last-read position
            if (lastRead && lastRead.chapterIndex > 0) {
                await resumeLastRead(lastRead);
            }
        } else if (lastRead && lastRead.storyId) {
            // No preferred story, try to resume from last-read
            const storyExists = stories.value.some(
                (s) => s.id === lastRead.storyId,
            );
            if (storyExists) {
                await loadStory(lastRead.storyId);
                if (lastRead.chapterIndex > 0) {
                    await resumeLastRead(lastRead);
                }
            }
        }
    } catch (err) {
        error.value = err instanceof Error ? err.message : String(err);
    } finally {
        loading.value = false;
    }
}

async function resumeLastRead(lastRead: LastReadPosition) {
    if (!selectedStory.value) return;

    const chapter = selectedStory.value.chapters.find(
        (c) => c.chapterIndex === lastRead.chapterIndex,
    );

    if (chapter) {
        // Create a ReaderProgress object to restore position
        const progress: ReaderProgress = {
            storyId: lastRead.storyId ?? selectedStory.value.story.id,
            chapterIndex: lastRead.chapterIndex,
            scrollPercent: lastRead.scrollPercent,
            audioPositionSec: lastRead.audioPositionSec,
        };

        // Load the last-read chapter and restore position
        await loadChapter(chapter.id, progress);

        toast.info(
            `Tiếp tục đọc: Chương ${chapter.chapterIndex} - ${chapter.title}`,
        );
    }
}

async function hydrateStoryProgress(items: Story[]) {
    const entries = await Promise.all(
        items.map(async (story) => {
            try {
                const progress = await api.progress(story.id);
                return [
                    story.id,
                    progress.storyId
                        ? progress
                        : {
                              storyId: story.id,
                              chapterIndex: 0,
                              scrollPercent: 0,
                              audioPositionSec: 0,
                          },
                ] as const;
            } catch {
                return [
                    story.id,
                    {
                        storyId: story.id,
                        chapterIndex: 0,
                        scrollPercent: 0,
                        audioPositionSec: 0,
                    },
                ] as const;
            }
        }),
    );

    progressByStory.value = Object.fromEntries(entries);
}

function applyRealtimeDefaults(appState: AppState) {
    const persisted = readRealtimePreferences();
    if (!selectedVoice.value) {
        selectedVoice.value =
            persisted.voice ||
            appState.config.realtimeDefaultVoice ||
            appState.config.edgeVoice;
    }
    realtimeSpeed.value =
        persisted.speed ?? appState.config.realtimeDefaultSpeed;
    realtimePitch.value =
        persisted.pitch ?? appState.config.realtimeDefaultPitch;
}

function clampRealtimeSpeed(value: number) {
    return Math.max(-50, Math.min(50, Math.round(value)));
}

function clampRealtimePitch(value: number) {
    return Math.max(-80, Math.min(80, Math.round(value)));
}

function readRealtimePreferences(): {
    voice: string;
    speed?: number;
    pitch?: number;
} {
    try {
        const raw = window.localStorage.getItem(realtimePrefsKey);
        if (!raw) return { voice: "" };
        const parsed = JSON.parse(raw) as {
            voice?: string;
            speed?: number;
            pitch?: number;
        };
        return {
            voice: typeof parsed.voice === "string" ? parsed.voice : "",
            speed:
                typeof parsed.speed === "number"
                    ? clampRealtimeSpeed(parsed.speed)
                    : undefined,
            pitch:
                typeof parsed.pitch === "number"
                    ? clampRealtimePitch(parsed.pitch)
                    : undefined,
        };
    } catch {
        return { voice: "" };
    }
}

function persistRealtimePreferences() {
    try {
        window.localStorage.setItem(
            realtimePrefsKey,
            JSON.stringify({
                voice: selectedVoice.value,
                speed: clampRealtimeSpeed(realtimeSpeed.value),
                pitch: clampRealtimePitch(realtimePitch.value),
            }),
        );
    } catch {
        // Bỏ qua lỗi localStorage (quota/private mode), không chặn luồng đọc.
    }
}

function clampReaderFontSize(value: number) {
    return Math.min(28, Math.max(14, Math.round(value || 18)));
}

function persistReaderPreferences() {
    try {
        window.localStorage.setItem(
            readerPrefsKey,
            JSON.stringify({
                fontSize: clampReaderFontSize(readerFontSize.value),
            }),
        );
    } catch {
        // Bỏ qua lỗi localStorage để không chặn màn hình đọc.
    }
}

// ===== Last-Read Position Persistence =====

const lastReadKey = "story-tts.reader.last-read.v1";

interface LastReadPosition {
    storyId: number | null;
    chapterIndex: number;
    scrollPercent: number;
    audioPositionSec: number;
    storySlug?: string;
}

function saveLastReadPosition(progress: ReaderProgress) {
    try {
        const position: LastReadPosition = {
            storyId: progress.storyId,
            chapterIndex: progress.chapterIndex,
            scrollPercent: progress.scrollPercent,
            audioPositionSec: progress.audioPositionSec,
            storySlug: selectedStory.value?.story.slug,
        };
        window.localStorage.setItem(lastReadKey, JSON.stringify(position));
    } catch {
        // Ignore localStorage errors
    }
}

function readLastReadPosition(): LastReadPosition {
    try {
        const raw = window.localStorage.getItem(lastReadKey);
        if (!raw) {
            return {
                storyId: null,
                chapterIndex: 0,
                scrollPercent: 0,
                audioPositionSec: 0,
            };
        }
        const parsed = JSON.parse(raw) as LastReadPosition;
        return {
            storyId: typeof parsed.storyId === "number" ? parsed.storyId : null,
            chapterIndex:
                typeof parsed.chapterIndex === "number"
                    ? parsed.chapterIndex
                    : 0,
            scrollPercent:
                typeof parsed.scrollPercent === "number"
                    ? parsed.scrollPercent
                    : 0,
            audioPositionSec:
                typeof parsed.audioPositionSec === "number"
                    ? parsed.audioPositionSec
                    : 0,
            storySlug: parsed.storySlug,
        };
    } catch {
        return {
            storyId: null,
            chapterIndex: 0,
            scrollPercent: 0,
            audioPositionSec: 0,
        };
    }
}

function clearLastReadPosition() {
    try {
        window.localStorage.removeItem(lastReadKey);
    } catch {
        // Ignore
    }
}

function adjustReaderFontSize(offset: number) {
    readerFontSize.value = clampReaderFontSize(readerFontSize.value + offset);
}

async function ensureRealtimeVoices(baseUrl?: string) {
    const targetBaseUrl = baseUrl || state.value?.config.realtimeTtsBaseUrl;
    if (!targetBaseUrl) return;

    try {
        const payload = await api.realtimeVoices(targetBaseUrl);
        realtimeVoices.value = payload.items ?? [];
        realtimeServiceError.value = "";

        const preferredVoice =
            selectedVoice.value ||
            payload.defaultVoice ||
            state.value?.config.realtimeDefaultVoice ||
            "";
        const matched =
            realtimeVoices.value.find((voice) => voice.id === preferredVoice) ??
            realtimeVoices.value[0];
        selectedVoice.value = matched?.id ?? preferredVoice;
    } catch (err) {
        realtimeServiceError.value =
            err instanceof Error ? err.message : String(err);
    }
}

function resolveStoryId(
    preferredStoryId?: number,
    preferredStorySlug?: string,
) {
    if (preferredStoryId) return preferredStoryId;
    if (preferredStorySlug) {
        const matched = stories.value.find(
            (story) => story.slug === preferredStorySlug,
        );
        if (matched) return matched.id;
    }
    if (selectedStory.value) {
        const byId = stories.value.find(
            (story) => story.id === selectedStory.value?.story.id,
        );
        if (byId) return byId.id;
        const bySlug = stories.value.find(
            (story) => story.slug === selectedStory.value?.story.slug,
        );
        if (bySlug) return bySlug.id;
    }
    return sortedStories.value[0]?.id;
}

async function importFromFolder(event: Event) {
    const target = event.target as HTMLInputElement | null;
    const files = Array.from(target?.files ?? []);
    if (files.length === 0) return;

    try {
        const payload = await buildImportPayloadFromFiles(files);
        currentDirectoryHandle.value = null;
        currentLibraryRoot.value = payload.rootName;
        await importPayload(payload);
    } catch (err) {
        error.value = err instanceof Error ? err.message : String(err);
    } finally {
        if (target) {
            target.value = "";
        }
        loading.value = false;
    }
}

async function buildImportPayloadFromFiles(
    files: File[],
): Promise<ImportFolderRequest> {
    const txtFiles = files.filter(
        (file) =>
            file.name.toLowerCase().endsWith(".txt") && file.webkitRelativePath,
    );
    if (txtFiles.length === 0) {
        throw new Error(
            "Không tìm thấy file .txt hợp lệ trong thư mục đã chọn",
        );
    }

    importProgress.value.totalStories = 0;
    importProgress.value.totalChapters = 0;
    importProgress.value.phase = "reading";
    importProgress.value.message = `Đang đọc ${txtFiles.length} file...`;

    const rootName = txtFiles[0].webkitRelativePath.split("/")[0] || "library";
    const storyMap = new Map<
        string,
        {
            title: string;
            chapters: Array<{
                relativePath: string;
                title: string;
                file: File;
            }>;
        }
    >();

    for (const file of txtFiles) {
        const segments = file.webkitRelativePath.split("/").filter(Boolean);
        if (segments.length !== 3) continue;
        const [, storyDir, chapterFile] = segments;
        const storyBucket = storyMap.get(storyDir) ?? {
            title: storyDir,
            chapters: [],
        };
        storyBucket.chapters.push({
            relativePath: `${storyDir}/${chapterFile}`,
            title: chapterFile.replace(/\.txt$/i, ""),
            file,
        });
        storyMap.set(storyDir, storyBucket);
    }

    if (storyMap.size === 0) {
        throw new Error(
            "Cần chọn thư mục gốc có cấu trúc: thư_mục_gốc/truyện/chương.txt",
        );
    }

    const storiesPayload: ImportedStoryDraft[] = [];
    for (const [relativePath, storyEntry] of [...storyMap.entries()].sort(
        (left, right) => collateNatural(left[0], right[0]),
    )) {
        const chapters = await Promise.all(
            storyEntry.chapters
                .sort((left, right) =>
                    collateNatural(left.relativePath, right.relativePath),
                )
                .map(async (chapter) => ({
                    relativePath: chapter.relativePath,
                    title: chapter.title,
                    content: await chapter.file.text(),
                })),
        );

        storiesPayload.push({
            relativePath,
            title: storyEntry.title,
            chapters,
        });
    }

    return { rootName, stories: storiesPayload };
}

async function chooseLibraryFolder() {
    const pickerWindow = window as DirectoryPickerWindow;
    if (!pickerWindow.showDirectoryPicker) {
        openLegacyFolderPicker();
        return;
    }

    try {
        const handle = await pickerWindow.showDirectoryPicker({
            id: "story-tts-library",
            mode: "read",
        });
        if (!(await ensureDirectoryPermission(handle, true))) {
            throw new Error("Chưa có quyền đọc thư mục đã chọn");
        }
        currentDirectoryHandle.value = handle;
        currentLibraryRoot.value = handle.name;
        await saveDirectoryHandle(handle);
        await importFromDirectoryHandle(handle);
    } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") {
            return;
        }
        error.value = err instanceof Error ? err.message : String(err);
    }
}

async function importFromDirectoryHandle(handle: BrowserDirectoryHandle) {
    const storiesPayload: ImportedStoryDraft[] = [];

    for await (const entry of handle.values()) {
        if (entry.kind !== "directory") continue;
        const storyHandle = entry as BrowserDirectoryHandle;
        const chapters: ImportedChapterDraft[] = [];

        for await (const child of storyHandle.values()) {
            if (
                child.kind !== "file" ||
                !child.name.toLowerCase().endsWith(".txt")
            )
                continue;
            const file = await (child as BrowserFileHandle).getFile();
            chapters.push({
                relativePath: `${storyHandle.name}/${child.name}`,
                title: child.name.replace(/\.txt$/i, ""),
                content: await file.text(),
            });
        }

        if (chapters.length === 0) continue;
        storiesPayload.push({
            relativePath: storyHandle.name,
            title: storyHandle.name,
            chapters: chapters.sort((left, right) =>
                collateNatural(left.relativePath, right.relativePath),
            ),
        });
    }

    if (storiesPayload.length === 0) {
        throw new Error(
            "Không tìm thấy chương .txt trực tiếp bên trong các thư mục truyện",
        );
    }

    storiesPayload.sort((left, right) =>
        collateNatural(left.relativePath, right.relativePath),
    );
    currentLibraryRoot.value = handle.name;
    await importPayload({ rootName: handle.name, stories: storiesPayload });
}

async function importPayload(payload: ImportFolderRequest) {
    loading.value = true;
    error.value = "";

    // Calculate totals for progress tracking
    const totalStories = payload.stories.length;
    const totalChapters = payload.stories.reduce(
        (sum, story) => sum + story.chapters.length,
        0,
    );

    importProgress.value = {
        active: true,
        phase: "reading",
        currentStory: 0,
        totalStories,
        currentChapter: 0,
        totalChapters,
        message: `Đang chuẩn bị import ${totalStories} truyện, ${totalChapters} chương...`,
    };

    try {
        const previousStorySlug = selectedStory.value?.story.slug;

        // Phase: sending
        importProgress.value.phase = "sending";
        importProgress.value.message = "Đang gửi dữ liệu lên server...";

        const snapshot = await api.importFolder(payload);

        // Phase: processing
        importProgress.value.phase = "processing";
        importProgress.value.message = "Server đang xử lý và lưu trữ...";

        stories.value = snapshot.stories ?? [];
        const preferredStoryId = resolveStoryId(undefined, previousStorySlug);
        await refresh(preferredStoryId, previousStorySlug);

        // Done
        importProgress.value.phase = "done";
        importProgress.value.message = `Import thành công! ${totalStories} truyện, ${totalChapters} chương.`;

        toast.success(
            `Import thành công! ${totalStories} truyện, ${totalChapters} chương.`,
        );

        // Auto-hide progress after 3 seconds
        setTimeout(() => {
            importProgress.value.active = false;
        }, 3000);
    } catch (err) {
        importProgress.value.phase = "error";
        importProgress.value.message =
            err instanceof Error ? err.message : "Lỗi không xác định";
        error.value = importProgress.value.message;
        toast.error(`Import thất bại: ${importProgress.value.message}`);
        throw err;
    } finally {
        loading.value = false;
    }
}

async function refreshLibrary() {
    error.value = "";
    try {
        if (currentDirectoryHandle.value) {
            const granted = await ensureDirectoryPermission(
                currentDirectoryHandle.value,
                true,
            );
            if (!granted) {
                error.value =
                    "Không còn quyền truy cập thư mục đã chọn. Hãy chọn lại thư mục truyện.";
                return;
            }
            await importFromDirectoryHandle(currentDirectoryHandle.value);
            return;
        }

        if (stories.value.length > 0) {
            error.value =
                'Trình duyệt chưa giữ quyền thư mục. Hãy bấm "Chọn thư mục truyện" lại một lần để app có thể làm mới tự động.';
            return;
        }

        await refresh();
    } catch (err) {
        error.value = err instanceof Error ? err.message : String(err);
    }
}

async function ensureDirectoryPermission(
    handle: BrowserDirectoryHandle,
    ask: boolean,
) {
    const options = { mode: "read" as const };
    if ((await handle.queryPermission(options)) === "granted") {
        return true;
    }
    if (!ask) return false;
    return (await handle.requestPermission(options)) === "granted";
}

async function restoreDirectoryHandle() {
    const handle = await loadDirectoryHandle();
    if (!handle) return;
    const granted = await ensureDirectoryPermission(handle, false);
    if (!granted) return;
    currentDirectoryHandle.value = handle;
    currentLibraryRoot.value = handle.name;
}

function openHandleDb() {
    return new Promise<IDBDatabase>((resolve, reject) => {
        const request = window.indexedDB.open(handleDbName, 1);
        request.onerror = () =>
            reject(
                request.error ??
                    new Error("Không mở được IndexedDB cho handle thư mục"),
            );
        request.onupgradeneeded = () => {
            request.result.createObjectStore(handleStoreName);
        };
        request.onsuccess = () => resolve(request.result);
    });
}

async function saveDirectoryHandle(handle: BrowserDirectoryHandle) {
    const db = await openHandleDb();
    await new Promise<void>((resolve, reject) => {
        const tx = db.transaction(handleStoreName, "readwrite");
        tx.objectStore(handleStoreName).put(handle, handleKey);
        tx.oncomplete = () => resolve();
        tx.onerror = () =>
            reject(tx.error ?? new Error("Không lưu được handle thư mục"));
        tx.onabort = () =>
            reject(tx.error ?? new Error("Không lưu được handle thư mục"));
    });
    db.close();
}

async function loadDirectoryHandle() {
    if (!window.indexedDB) return null;
    const db = await openHandleDb();
    const result = await new Promise<BrowserDirectoryHandle | null>(
        (resolve, reject) => {
            const tx = db.transaction(handleStoreName, "readonly");
            const request = tx.objectStore(handleStoreName).get(handleKey);
            request.onsuccess = () =>
                resolve(
                    (request.result as BrowserDirectoryHandle | undefined) ??
                        null,
                );
            request.onerror = () =>
                reject(
                    request.error ??
                        new Error("Không đọc được handle thư mục đã lưu"),
                );
        },
    );
    db.close();
    return result;
}

async function loadStory(storyId: number) {
    console.log("loadStory starting with storyId:", storyId);
    await stopRealtimePlayback({ quiet: true, clearSession: true });
    loading.value = true;
    error.value = "";
    try {
        const [detail, progress] = await Promise.all([
            api.story(storyId),
            api.progress(storyId),
        ]);
        console.log(
            "loadStory loaded detail for:",
            detail.story.id,
            detail.story.title,
        );
        selectedStory.value = detail;
        readerProgress.value = progress.storyId ? progress : null;
        progressByStory.value = {
            ...progressByStory.value,
            [storyId]: progress.storyId
                ? progress
                : {
                      storyId,
                      chapterIndex: 0,
                      scrollPercent: 0,
                      audioPositionSec: 0,
                  },
        };
        currentPreset.value = detail.story.defaultPreset || "stable";
        contentCache.clear();

        const preferredChapter =
            detail.chapters.find(
                (chapter) => chapter.chapterIndex === progress.chapterIndex,
            ) ?? detail.chapters[0];
        if (preferredChapter) {
            await loadChapter(
                preferredChapter.id,
                progress.chapterIndex === preferredChapter.chapterIndex
                    ? progress
                    : null,
            );
        } else {
            selectedChapterId.value = null;
            chapterContent.value = null;
        }
    } catch (err) {
        error.value = err instanceof Error ? err.message : String(err);
    } finally {
        loading.value = false;
    }
}

function storyProgress(storyId: number) {
    return progressByStory.value[storyId] ?? null;
}

function storyProgressText(story: Story) {
    const progress = storyProgress(story.id);
    if (!progress?.chapterIndex) {
        return "Chưa có tiến độ đọc";
    }
    return `Đang đọc tới chương ${progress.chapterIndex}/${story.chapterCount}`;
}

function storyContinueText(story: Story) {
    const progress = storyProgress(story.id);
    if (!progress?.chapterIndex) {
        return "Bắt đầu đọc";
    }
    const nextChapter = Math.min(progress.chapterIndex, story.chapterCount);
    return `Đọc tiếp chương ${nextChapter}`;
}

async function selectStory(storyId: number) {
    console.log("selectStory called with storyId:", storyId);
    await loadStory(storyId);
}

async function openStoryReader(storyId: number) {
    console.log("openStoryReader called with storyId:", storyId);
    activeTab.value = "reader";
    await loadStory(storyId);
}

async function continueStory(storyId: number, event?: Event) {
    event?.stopPropagation();
    console.log("continueStory called with storyId:", storyId);
    activeTab.value = "reader";
    await loadStory(storyId);
}

async function selectChapter(chapterId: number, event?: Event) {
    event?.stopPropagation();
    console.log("selectChapter called with chapterId:", chapterId);
    activeTab.value = "reader";
    await loadChapter(chapterId, null, { switchTab: true, skipPersist: false });

    // Lưu progress để đánh dấu chương đã đọc
    if (selectedStory.value) {
        const chapter = selectedStory.value.chapters.find(
            (c) => c.id === chapterId,
        );
        if (chapter) {
            await markChapterRead(
                selectedStory.value.story.id,
                chapter.chapterIndex,
            );
        }
    }
}

async function markChapterRead(storyId: number, chapterIndex: number) {
    const progress = progressByStory.value[storyId];
    if (progress && chapterIndex > progress.chapterIndex) {
        const payload: ReaderProgress = {
            ...progress,
            chapterIndex,
            scrollPercent: 0,
            audioPositionSec: 0,
        };
        const saved = await api.saveProgress(payload);
        progressByStory.value = {
            ...progressByStory.value,
            [storyId]: saved,
        };
        if (selectedStory.value?.story.id === storyId) {
            readerProgress.value = saved;
        }
        saveLastReadPosition(saved);
    }
}

function isChapterRead(chapter: Chapter): boolean {
    if (!selectedStory.value) return false;
    const progress = progressByStory.value[selectedStory.value.story.id];
    if (!progress) return false;
    return chapter.chapterIndex <= progress.chapterIndex;
}

async function loadChapter(
    chapterId: number,
    progress?: ReaderProgress | null,
    options: LoadChapterOptions & { switchTab?: boolean } = {},
) {
    if (options.switchTab) {
        activeTab.value = "reader";
    }
    if (!options.preservePlayback) {
        if (edgeReadAloudActive.value) {
            await stopEdgeReadAloudPlayback();
        } else {
            clearEdgeReadAloudTimer();
        }
        await stopRealtimePlayback({ quiet: true, clearSession: true });
    }

    loading.value = true;
    error.value = "";
    try {
        const content = await getChapterContentCached(chapterId);
        selectedChapterId.value = chapterId;
        chapterContent.value = content;
        await nextTick();
        restoreReaderPosition(progress);
        if (!options.skipPersist) {
            await persistProgress(0);
        }
    } catch (err) {
        error.value = err instanceof Error ? err.message : String(err);
    } finally {
        loading.value = false;
    }
}

async function getChapterContentCached(chapterId: number) {
    const cached = contentCache.get(chapterId);
    if (cached) return cached;
    const content = await api.chapterContent(chapterId);
    contentCache.set(chapterId, content);
    return content;
}

function restoreReaderPosition(progress?: ReaderProgress | null) {
    const chapter = selectedChapter.value;
    const container = readerBody.value;
    if (!chapter || !container) return;

    if (progress && progress.chapterIndex === chapter.chapterIndex) {
        const maxScroll = Math.max(
            container.scrollHeight - container.clientHeight,
            0,
        );
        container.scrollTop = maxScroll * progress.scrollPercent;
    } else {
        container.scrollTop = 0;
    }
}

function currentScrollPercent() {
    const container = readerBody.value;
    if (!container) return 0;
    const maxScroll = container.scrollHeight - container.clientHeight;
    if (maxScroll <= 0) return 0;
    return container.scrollTop / maxScroll;
}

async function persistProgress(audioPositionSec = 0) {
    const story = selectedStory.value?.story;
    const targetChapterId =
        isRealtimeActive.value && currentPlaybackChapterId.value !== null
            ? currentPlaybackChapterId.value
            : selectedChapterId.value;
    const chapter =
        selectedStory.value?.chapters.find(
            (item) => item.id === targetChapterId,
        ) ?? selectedChapter.value;
    if (!story || !chapter) return;

    const payload: ReaderProgress = {
        storyId: story.id,
        chapterIndex: chapter.chapterIndex,
        scrollPercent:
            selectedChapterId.value === chapter.id ? currentScrollPercent() : 0,
        audioPositionSec,
    };

    const saved = await api.saveProgress(payload);
    readerProgress.value = saved;
    progressByStory.value = {
        ...progressByStory.value,
        [story.id]: saved,
    };
    stories.value = stories.value.map((item) =>
        item.id === story.id
            ? {
                  ...item,
                  lastOpenedAt: saved.updatedAt ?? new Date().toISOString(),
              }
            : item,
    );

    // Also save to localStorage for resume on reload
    saveLastReadPosition(saved);
}

async function persistPlaybackChapterProgress(
    chapterId: number,
    audioPositionSec = 0,
) {
    const story = selectedStory.value?.story;
    const chapter = selectedStory.value?.chapters.find(
        (item) => item.id === chapterId,
    );
    if (!story || !chapter) return;

    const current = progressByStory.value[story.id];
    const payload: ReaderProgress = {
        storyId: story.id,
        chapterIndex: chapter.chapterIndex,
        scrollPercent:
            selectedChapterId.value === chapterId ? currentScrollPercent() : 0,
        audioPositionSec:
            selectedChapterId.value === chapterId ? audioPositionSec : 0,
    };

    const saved = await api.saveProgress(payload);
    progressByStory.value = {
        ...progressByStory.value,
        [story.id]: saved,
    };
    readerProgress.value = saved;
    saveLastReadPosition(saved);
}

function scheduleProgressSave() {
    if (progressTimer !== null) {
        window.clearTimeout(progressTimer);
    }
    progressTimer = window.setTimeout(() => {
        void persistProgress(audioRef.value?.currentTime ?? 0);
    }, 350);
}

function clearEdgeReadAloudTimer() {
    if (edgeReadAloudTimer !== null) {
        window.clearTimeout(edgeReadAloudTimer);
        edgeReadAloudTimer = null;
    }
}

function persistEdgeReadAloudPreferences() {
    window.localStorage.setItem(
        edgeReadAloudPrefsKey,
        JSON.stringify({
            enabled: useEdgeReadAloud.value,
            wordsPerMinute: edgeReadAloudWordsPerMinute.value,
        }),
    );
}

function collectReadableTextNodes(root: HTMLElement) {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
        acceptNode(node) {
            return node.textContent?.trim()
                ? NodeFilter.FILTER_ACCEPT
                : NodeFilter.FILTER_SKIP;
        },
    });
    const textNodes: Text[] = [];
    let currentNode = walker.nextNode();
    while (currentNode) {
        textNodes.push(currentNode as Text);
        currentNode = walker.nextNode();
    }
    return textNodes;
}

function focusReaderForEdgeReadAloud(
    options: { preserveScroll?: boolean } = {},
) {
    if (!readerBody.value || !readerScanContent.value) return false;
    const preserveScroll = options.preserveScroll ?? false;
    const previousScrollTop = readerBody.value.scrollTop;
    if (!preserveScroll) {
        readerBody.value.scrollTop = 0;
    }
    readerBody.value.focus({ preventScroll: preserveScroll });

    const textNodes = collectReadableTextNodes(readerScanContent.value);
    if (textNodes.length === 0) return false;
    const selection = window.getSelection();
    if (!selection) return false;

    const range = document.createRange();
    range.setStart(textNodes[0], 0);
    const lastTextNode = textNodes[textNodes.length - 1];
    range.setEnd(lastTextNode, lastTextNode.textContent?.length ?? 0);
    selection.removeAllRanges();
    selection.addRange(range);
    if (preserveScroll) {
        readerBody.value.scrollTop = previousScrollTop;
    }
    return true;
}

async function prepareEdgeReadAloudSelection(
    options: { preserveScroll?: boolean } = {},
) {
    await nextTick();
    const prepared = focusReaderForEdgeReadAloud(options);
    if (!prepared) {
        error.value =
            "Không tìm thấy khối văn bản để chuyển sang Edge Read Aloud.";
        return false;
    }
    error.value = "";
    return true;
}

async function sendEdgeReadAloudHotkey() {
    await new Promise((resolve) => window.setTimeout(resolve, 180));
    await api.toggleEdgeReadAloud();
}

async function rescanReaderTextForEdgeReadAloud() {
    if (!chapterContent.value) return;
    const wasActive = edgeReadAloudActive.value;
    clearEdgeReadAloudTimer();

    if (wasActive) {
        edgeReadAloudActive.value = false;
        await sendEdgeReadAloudHotkey();
        await new Promise((resolve) => window.setTimeout(resolve, 220));
    }

    const prepared = await prepareEdgeReadAloudSelection({
        preserveScroll: true,
    });
    if (!prepared) return;

    if (wasActive) {
        edgeReadAloudActive.value = true;
        await sendEdgeReadAloudHotkey();
        scheduleEdgeReadAloudAutoNext();
    }
}

function scheduleEdgeReadAloudAutoNext() {
    clearEdgeReadAloudTimer();
    if (!edgeReadAloudActive.value || !selectedChapter.value) return;

    const estimatedMs = Math.max(
        12000,
        Math.round(
            (chapterWordCount.value /
                Math.max(edgeReadAloudWordsPerMinute.value, 120)) *
                60_000 +
                4000,
        ),
    );
    edgeReadAloudTimer = window.setTimeout(async () => {
        if (!edgeReadAloudActive.value) return;
        const target = chapterAt(1);
        if (!target) {
            edgeReadAloudActive.value = false;
            return;
        }
        await loadChapter(target.id, null, { preservePlayback: true });
        const prepared = await prepareEdgeReadAloudSelection();
        if (!prepared) return;
        await sendEdgeReadAloudHotkey();
        scheduleEdgeReadAloudAutoNext();
    }, estimatedMs);
}

async function startEdgeReadAloudPlayback() {
    if (!isEdgeBrowser.value) {
        error.value =
            "Edge Read Aloud mode chỉ dùng được trong Microsoft Edge.";
        return;
    }
    if (!chapterContent.value) return;

    await stopRealtimePlayback({ quiet: true, clearSession: true });
    const prepared = await prepareEdgeReadAloudSelection();
    if (!prepared) return;
    error.value = "";
    edgeReadAloudActive.value = true;
    await sendEdgeReadAloudHotkey();
    scheduleEdgeReadAloudAutoNext();
}

async function stopEdgeReadAloudPlayback() {
    clearEdgeReadAloudTimer();
    if (!edgeReadAloudActive.value) return;
    edgeReadAloudActive.value = false;
    await sendEdgeReadAloudHotkey();
}

function handleTimeUpdate() {
    if (audioRef.value) {
        audioCurrentTime.value = audioRef.value.currentTime;
        audioDuration.value = audioRef.value.duration;
    }
    scheduleProgressSave();
}

function handleAudioPlay() {
    userPausedAudio = false;
    audioIsPlaying.value = true;
}

function handleAudioPause() {
    audioIsPlaying.value = false;
    scheduleProgressSave();
}

function handleAudioEnded() {
    audioIsPlaying.value = false;
}

function resetAudioTimelineState() {
    audioCurrentTime.value = 0;
    audioDuration.value = 0;
}

function setResumeBufferThreshold(seconds: number) {
    resumeBufferThresholdSeconds = Math.max(0, seconds);
}

function togglePlayback() {
    if (useEdgeReadAloud.value) {
        if (edgeReadAloudActive.value) {
            void stopEdgeReadAloudPlayback();
        } else {
            void startEdgeReadAloudPlayback();
        }
        return;
    }

    if (!audioRef.value) return;

    if (!realtimeSession.value && !realtimeConnecting.value) {
        void startRealtimePlayback();
        return;
    }

    if (audioRef.value.paused) {
        userPausedAudio = false;
        audioRef.value.play();
    } else {
        userPausedAudio = true;
        audioRef.value.pause();
    }
}

async function handleReaderPlayAction() {
    if (useEdgeReadAloud.value) {
        togglePlayback();
        return;
    }

    const targetChapterId = selectedChapterId.value;
    if (
        readerTargetsDifferentRealtimeChapter.value &&
        targetChapterId !== null
    ) {
        const targetGroup =
            sortedRealtimeChapterGroups.value.find(
                (group) => group.chapterId === targetChapterId,
            ) ?? null;
        const startSegmentIndex = targetGroup?.startSegmentIndex ?? 0;
        toast.info("Chuyển sang đọc chương đang mở.");
        await jumpToRealtimeSegment(targetChapterId, startSegmentIndex);
        return;
    }

    togglePlayback();
}

function seekAudio(event: MouseEvent) {
    const container = event.currentTarget as HTMLElement;
    if (!container || !audioRef.value) return;
    const rect = container.getBoundingClientRect();
    const percent = (event.clientX - rect.left) / rect.width;
    const seekTime = percent * (audioRef.value.duration || 0);
    audioRef.value.currentTime = seekTime;
}

function scrollActiveWordIntoView() {
    if (!readerBody.value || activeWordGlobalIndex.value === null) return;
    const target = readerBody.value.querySelector<HTMLElement>(
        `[data-word-index="${activeWordGlobalIndex.value}"]`,
    );
    if (!target) return;

    const rect = target.getBoundingClientRect();
    const containerRect = readerBody.value.getBoundingClientRect();
    const isVisible =
        rect.top >= containerRect.top + 24 &&
        rect.bottom <= containerRect.bottom - 24;
    if (!isVisible) {
        target.scrollIntoView({
            behavior: "smooth",
            block: "center",
            inline: "nearest",
        });
    }
}

function findSegmentIndexByWordOffset(
    group: RealtimeChapterSegmentGroup,
    wordOffset: number,
) {
    let accumulated = 0;
    for (const segment of group.segments) {
        const start = accumulated;
        const end = accumulated + segment.wordCount;
        if (wordOffset < end) {
            return segment.index;
        }
        accumulated = end;
    }
    return group.startSegmentIndex;
}

function getSelectionWordOffset() {
    if (!readerScanContent.value) {
        console.warn("[Selection] readerScanContent is null");
        return null;
    }
    const selection = window.getSelection();
    if (!selection || selection.rangeCount === 0 || selection.isCollapsed) {
        console.warn(
            "[Selection] No valid selection:",
            selection?.toString()?.slice(0, 50),
        );
        return null;
    }

    const range = selection.getRangeAt(0);
    if (!readerScanContent.value.contains(range.startContainer)) {
        console.warn("[Selection] Selection not inside readerScanContent");
        return null;
    }

    const prefix = document.createRange();
    prefix.selectNodeContents(readerScanContent.value);
    prefix.setEnd(range.startContainer, range.startOffset);
    const textBefore = normalizeTtsText(prefix.toString());
    if (!textBefore) return 0;
    const words = textBefore.split(/\s+/).filter(Boolean);
    console.log(
        `[Selection] Word offset=${words.length}, text before: "${textBefore.slice(0, 100)}..."`,
    );
    return words.length;
}

async function jumpToRealtimeSegment(chapterId: number, segmentIndex: number) {
    if (!selectedStory.value) return;
    const baseUrl = realtimeBaseUrl();
    if (!baseUrl) return;
    playbackAutoSyncLockChapterId.value = chapterId;

    if (selectedChapterId.value !== chapterId) {
        await loadChapter(chapterId, null, {
            preservePlayback: true,
            skipPersist: true,
        });
    }

    patchRealtimeChapterGroup(chapterId, { startSegmentIndex: segmentIndex });
    rewindRealtimeChapterFromSegment(chapterId, segmentIndex);

    const currentSessionId = realtimeSession.value?.id;
    const targetChapterGroup =
        sortedRealtimeChapterGroups.value.find(
            (group) => group.chapterId === chapterId,
        ) ?? null;
    const targetSegment =
        targetChapterGroup?.segments.find(
            (segment) => segment.index === segmentIndex,
        ) ?? null;
    const targetSegmentDynamicStatus = targetSegment
        ? getDynamicSegmentStatus(targetSegment.index, chapterId)
        : null;
    const hasCurrentRealtimeSession =
        Boolean(currentSessionId) &&
        Boolean(targetChapterGroup) &&
        targetSegment !== null;
    const canFastResumeCurrentSession =
        targetSegmentDynamicStatus !== null &&
        ["ready", "played", "reading"].includes(targetSegmentDynamicStatus);

    if (hasCurrentRealtimeSession && currentSessionId) {
        console.log(
            `[Seek] Reusing current session for chapter ${chapterId}, segment ${segmentIndex} (status=${targetSegmentDynamicStatus ?? "unknown"})`,
        );
        try {
            pendingSeekTarget.value = { chapterId, segmentIndex };
            dropIncomingAudioUntilSeekStart = true;
            realtimeStatus.value = "transitioning";
            resetPlaybackTimeline();
            if (audioRef.value && !audioRef.value.paused) {
                audioRef.value.pause();
            }
            await prepareRealtimeMediaStream({
                fastResume: canFastResumeCurrentSession,
            });
            const updatedSession = await api.seekRealtimeSession(
                baseUrl,
                currentSessionId,
                {
                    chapterId,
                    segmentIndex,
                },
            );
            realtimeSession.value = updatedSession;
            schedulePendingSeekFallback(
                currentSessionId,
                chapterId,
                segmentIndex,
            );
            if (canFastResumeCurrentSession) {
                toast.info(
                    `Đọc ngay đoạn ${segmentIndex + 1} từ audio đã render.`,
                );
            } else {
                toast.info(
                    `Chuyển sang đoạn ${segmentIndex + 1} trong session hiện tại để tiếp tục render từ cache đang có.`,
                );
            }
            return;
        } catch (err) {
            clearPendingSeekFallbackTimer();
            pendingSeekTarget.value = null;
            dropIncomingAudioUntilSeekStart = false;
            playbackAutoSyncLockChapterId.value = null;
            console.warn(
                "[Seek] Seek current session thất bại, fallback sang restart:",
                err,
            );
        }
    }

    // Stop current playback and clear all state
    if (isRealtimeActive.value) {
        console.log(
            `[Seek] Stopping current playback to jump to chapter ${chapterId}, segment ${segmentIndex}`,
        );
        await stopRealtimePlayback({ quiet: true, clearSession: true });
    }

    // Small delay to ensure cleanup is complete
    await new Promise((resolve) => setTimeout(resolve, 300));

    // Start new playback from the target segment
    console.log(
        `[Seek] Starting new playback: chapterId=${chapterId}, segmentIndex=${segmentIndex}`,
    );
    toast.info(`Đọc từ đoạn ${segmentIndex + 1}...`);
    await startRealtimePlayback({
        startChapterId: chapterId,
        startSegmentIndex: segmentIndex,
    });
}

async function startRealtimeFromSelection() {
    const chapterId = selectedChapterId.value;
    const group = selectedRealtimeChapterGroup.value;
    console.log(
        `[Selection] chapterId=${chapterId}, group exists=${!!group}, segments count=${group?.segments?.length ?? 0}`,
    );
    if (chapterId === null || !group) {
        error.value = "Chưa có segment realtime cho chương đang mở.";
        toast.error(
            "Chưa có segment realtime. Hãy bắt đầu đọc từ đầu chương trước.",
        );
        return;
    }

    const wordOffset = getSelectionWordOffset();
    if (wordOffset === null) {
        error.value =
            "Hãy bôi chọn một đoạn trong khung đọc trước khi yêu cầu đọc tiếp từ đó.";
        toast.warning(
            "Không tìm thấy vị trí trong text. Hãy thử click vào segment bên dưới.",
        );
        return;
    }

    const targetSegmentIndex = findSegmentIndexByWordOffset(group, wordOffset);
    console.log(
        `[Selection] wordOffset=${wordOffset} → targetSegmentIndex=${targetSegmentIndex}`,
    );
    toast.info(`Đọc từ đoạn ${targetSegmentIndex + 1}...`);
    await jumpToRealtimeSegment(chapterId, targetSegmentIndex);
}

async function buildRealtimePayload(
    options: { startChapterId?: number; startSegmentIndex?: number } = {},
) {
    if (!selectedStory.value) {
        throw new Error("Chưa có chương để phát realtime");
    }

    const targetChapterId = options.startChapterId ?? selectedChapter.value?.id;
    if (!targetChapterId) {
        throw new Error("Chưa có chương để phát realtime");
    }

    // === FIX: Only send chapters starting from targetChapterId ===
    // Sending ALL chapters is slow and unnecessary
    const startIndex = selectedStory.value.chapters.findIndex(
        (c) => c.id === targetChapterId,
    );
    const chaptersToBuild =
        startIndex >= 0
            ? selectedStory.value.chapters.slice(startIndex)
            : selectedStory.value.chapters;

    const chapters = await Promise.all(
        chaptersToBuild.map(async (chapter) => {
            const content = await getChapterContentCached(chapter.id);
            return {
                chapterId: content.chapterId,
                chapterIndex: content.chapterIndex,
                title: content.chapterTitle,
                text: content.text,
            } satisfies RealtimeChapterPayload;
        }),
    );

    console.log(
        `[buildRealtimePayload] targetChapterId=${targetChapterId}, startIndex=${startIndex}, sending ${chapters.length}/${selectedStory.value.chapters.length} chapters, startSegmentIndex=${options.startSegmentIndex ?? 0}`,
    );

    return {
        storyId: selectedStory.value.story.id,
        chapterId: targetChapterId,
        chapters,
        voice: selectedVoice.value,
        speed: realtimeSpeed.value,
        pitch: realtimePitch.value,
        autoNext: true,
        startSegmentIndex: options.startSegmentIndex ?? 0,
    };
}

function realtimeBaseUrl() {
    return state.value?.config.realtimeTtsBaseUrl ?? "";
}

async function startRealtimePlayback(
    options: { startChapterId?: number; startSegmentIndex?: number } = {},
) {
    if (!selectedStory.value || !selectedChapter.value) return;
    const baseUrl = realtimeBaseUrl();
    if (!baseUrl) {
        error.value = "Thiếu cấu hình realtime TTS base URL từ backend Go.";
        return;
    }

    if (realtimeVoices.value.length === 0) {
        await ensureRealtimeVoices(baseUrl);
    }
    if (!selectedVoice.value) {
        error.value = "Chưa có voice realtime hợp lệ.";
        return;
    }

    realtimeError.value = "";
    realtimeConnecting.value = true;
    userPausedAudio = false;
    error.value = "";

    try {
        await stopRealtimePlayback({ quiet: true, clearSession: true });
        resetRealtimeSegments();
        realtimeStatus.value = "connecting";
        await prepareRealtimeMediaStream();
        const payload = await buildRealtimePayload(options);
        console.log(
            `[startRealtimePlayback] Creating session with chapterId=${payload.chapterId}, startSegmentIndex=${payload.startSegmentIndex}`,
        );
        const session = await api.createRealtimeSession(baseUrl, payload);
        realtimeSession.value = session;
        const initialChapterId = options.startChapterId ?? session.chapterId;
        const initialChapterTitle =
            selectedStory.value.chapters.find(
                (chapter) => chapter.id === initialChapterId,
            )?.title ?? selectedChapter.value?.title ?? "";
        realtimeBufferedChapterId.value = initialChapterId;
        realtimeBufferedChapterTitle.value = initialChapterTitle;
        realtimeAudibleChapterId.value = initialChapterId;
        realtimeAudibleChapterTitle.value = initialChapterTitle;
        isPlaybackActive = true; // Đánh dấu bắt đầu phiên playback
        connectRealtimeSocket(baseUrl, session.id);
    } catch (err) {
        realtimeStatus.value = "error";
        realtimeError.value = err instanceof Error ? err.message : String(err);
        error.value = realtimeError.value;
        teardownRealtimeSocket();
        resetRealtimeMediaStream();
    } finally {
        realtimeConnecting.value = false;
    }
}

async function restartRealtimePlayback() {
    await startRealtimePlayback();
}

function clearRealtimeControlSyncTimer() {
    if (realtimeControlsSyncTimer !== null) {
        window.clearTimeout(realtimeControlsSyncTimer);
        realtimeControlsSyncTimer = null;
    }
}

function clearRealtimeRestartTimer() {
    if (realtimeRestartTimer !== null) {
        window.clearTimeout(realtimeRestartTimer);
        realtimeRestartTimer = null;
    }
}

function clearPendingSeekFallbackTimer() {
    if (pendingSeekFallbackTimer !== null) {
        window.clearTimeout(pendingSeekFallbackTimer);
        pendingSeekFallbackTimer = null;
    }
}

function schedulePendingSeekFallback(
    sessionId: string,
    chapterId: number,
    segmentIndex: number,
) {
    clearPendingSeekFallbackTimer();
    pendingSeekFallbackTimer = window.setTimeout(() => {
        const stillPending =
            pendingSeekTarget.value?.chapterId === chapterId &&
            pendingSeekTarget.value?.segmentIndex === segmentIndex;
        const sameSession = realtimeSession.value?.id === sessionId;
        if (!stillPending || !sameSession) {
            return;
        }

        console.warn(
            `[Seek] Timeout waiting for ${chapterId}:${segmentIndex}. Falling back to restart playback.`,
        );
        pendingSeekTarget.value = null;
        dropIncomingAudioUntilSeekStart = false;
        playbackAutoSyncLockChapterId.value = null;

        void (async () => {
            try {
                await stopRealtimePlayback({ quiet: true, clearSession: true });
                await new Promise((resolve) => window.setTimeout(resolve, 300));
                toast.info(
                    `Seek đang bị kẹt, mở lại đọc từ đoạn ${segmentIndex + 1}...`,
                );
                await startRealtimePlayback({
                    startChapterId: chapterId,
                    startSegmentIndex: segmentIndex,
                });
            } catch (err) {
                const message =
                    err instanceof Error ? err.message : String(err);
                realtimeStatus.value = "error";
                realtimeError.value = message;
                error.value = message;
                toast.error(`Không thể đọc từ đoạn đã chọn: ${message}`);
            }
        })();
    }, 2500);
}

async function syncRealtimeControls() {
    const baseUrl = realtimeBaseUrl();
    const sessionId = realtimeSession.value?.id;
    if (!baseUrl || !sessionId) return;

    try {
        const updated = await api.updateRealtimeControls(baseUrl, sessionId, {
            voice: selectedVoice.value,
            speed: realtimeSpeed.value,
            pitch: realtimePitch.value,
            autoNext: realtimeSession.value?.autoNext ?? true,
        });
        realtimeSession.value = updated;
        realtimeServiceError.value = "";
    } catch (err) {
        realtimeServiceError.value =
            err instanceof Error ? err.message : String(err);
    }
}

function scheduleRealtimeControlsSync() {
    if (!realtimeSession.value || realtimeConnecting.value) return;
    clearRealtimeControlSyncTimer();
    // Debounce để không spam API khi người dùng kéo slider liên tục.
    realtimeControlsSyncTimer = window.setTimeout(() => {
        void syncRealtimeControls();
    }, 180);
}

function scheduleRealtimeRestart() {
    if (!realtimeSession.value || realtimeConnecting.value) return;
    clearRealtimeRestartTimer();
    // Khi đang đọc, restart phiên realtime để chương/đoạn tiếp theo áp dụng tốc độ/cao độ mới ngay.
    realtimeRestartTimer = window.setTimeout(() => {
        if (isRealtimeActive.value) {
            void restartRealtimePlayback();
            return;
        }
        void syncRealtimeControls();
    }, 260);
}

async function stopRealtimePlayback(
    options: { quiet?: boolean; clearSession?: boolean } = {},
) {
    clearRealtimeControlSyncTimer();
    clearRealtimeRestartTimer();
    clearPendingSeekFallbackTimer();
    const baseUrl = realtimeBaseUrl();
    const sessionId = realtimeSession.value?.id;

    if (sessionId && baseUrl) {
        try {
            await api.stopRealtimeSession(baseUrl, sessionId);
        } catch (err) {
            if (!options.quiet) {
                error.value = err instanceof Error ? err.message : String(err);
            }
        }
    }

    teardownRealtimeSocket();
    finishRealtimeStream();
    resetRealtimeSegments();
    if (options.clearSession) {
        realtimeSession.value = null;
        realtimeBufferedChapterId.value = null;
        realtimeBufferedChapterTitle.value = "";
        realtimeAudibleChapterId.value = null;
        realtimeAudibleChapterTitle.value = "";
        realtimeStatus.value = "stopped";
    }
    await persistProgress(0).catch(() => undefined);
}

async function prepareRealtimeMediaStream(
    options: { fastResume?: boolean } = {},
) {
    console.log("[MediaStream] Preparing realtime media stream...");
    resetRealtimeMediaStream(true);
    resetAudioTimelineState();
    setResumeBufferThreshold(options.fastResume ? 0.25 : 4);

    if (!window.MediaSource || !MediaSource.isTypeSupported("audio/mpeg")) {
        throw new Error(
            "Trình duyệt hiện tại không hỗ trợ MediaSource cho audio/mpeg realtime.",
        );
    }

    if (!audioRef.value) {
        await nextTick();
    }
    if (!audioRef.value) {
        throw new Error("Không khởi tạo được audio player realtime.");
    }

    activeMediaSource = new MediaSource();
    activeMediaUrl = URL.createObjectURL(activeMediaSource);
    currentStreamMime.value = "audio/mpeg";
    pendingMediaEnd = false;
    queuedAudioChunks = [];
    isPlaybackActive = true;

    console.log("[MediaStream] MediaSource created, waiting for sourceopen...");

    // Đợi sourceopen event với timeout dài hơn (30s) để tránh timeout khi backend retry
    await new Promise<void>((resolve, reject) => {
        const timeout = setTimeout(() => {
            console.warn(
                "[MediaStream] Timeout waiting for sourceopen after 30s, proceeding anyway...",
            );
            resolve(); // Không reject, tiếp tục để tránh reset
        }, 30000);

        if (activeMediaSource!.readyState === "open") {
            clearTimeout(timeout);
            console.log("[MediaStream] MediaSource already open");
            resolve();
        } else {
            activeMediaSource!.addEventListener(
                "sourceopen",
                () => {
                    clearTimeout(timeout);
                    console.log(
                        "[MediaStream] MediaSource sourceopen received, ready for audio",
                    );
                    resolve();
                },
                { once: true },
            );
        }
    });

    // Setup source buffer
    if (activeMediaSource && activeMediaSource.readyState === "open") {
        try {
            activeSourceBuffer = activeMediaSource.addSourceBuffer(
                currentStreamMime.value,
            );
            activeSourceBuffer.mode = "sequence";
            activeSourceBuffer.addEventListener(
                "updateend",
                flushAudioChunkQueue,
            );
            console.log("[MediaStream] SourceBuffer created and ready");
        } catch (err) {
            console.error("[MediaStream] Failed to create SourceBuffer:", err);
            throw err;
        }
    } else {
        console.warn(
            "[MediaStream] MediaSource not open yet, will setup SourceBuffer when ready",
        );
        activeMediaSource.addEventListener(
            "sourceopen",
            () => {
                if (
                    activeMediaSource &&
                    activeMediaSource.readyState === "open"
                ) {
                    try {
                        activeSourceBuffer = activeMediaSource.addSourceBuffer(
                            currentStreamMime.value,
                        );
                        activeSourceBuffer.mode = "sequence";
                        activeSourceBuffer.addEventListener(
                            "updateend",
                            flushAudioChunkQueue,
                        );
                        console.log(
                            "[MediaStream] SourceBuffer created on sourceopen event",
                        );
                    } catch (err) {
                        console.error(
                            "[MediaStream] Failed to create SourceBuffer on sourceopen:",
                            err,
                        );
                    }
                }
            },
            { once: true },
        );
    }

    audioRef.value.src = activeMediaUrl;
    audioRef.value.autoplay = false;
    audioRef.value.load();

    console.log(
        "[MediaStream] Audio player setup complete, waiting for backend audio chunks...",
    );
}

function handleSourceOpen() {
    if (!activeMediaSource || activeMediaSource.readyState !== "open") return;
    activeSourceBuffer = activeMediaSource.addSourceBuffer(
        currentStreamMime.value,
    );
    activeSourceBuffer.mode = "sequence";
    activeSourceBuffer.addEventListener("updateend", flushAudioChunkQueue);
    flushAudioChunkQueue();
}

function appendRealtimeChunk(data: ArrayBuffer) {
    if (dropIncomingAudioUntilSeekStart) {
        return;
    }
    queuedAudioChunks.push(new Uint8Array(data.slice(0)));
    if (
        realtimeStatus.value === "connecting" ||
        realtimeStatus.value === "transitioning"
    ) {
        realtimeStatus.value = "buffering";
    }
    flushAudioChunkQueue();
}

function isSourceBufferAttached(sourceBuffer: SourceBuffer) {
    if (!activeMediaSource || activeMediaSource.readyState !== "open")
        return false;
    try {
        return Array.from(activeMediaSource.sourceBuffers).includes(
            sourceBuffer,
        );
    } catch {
        return false;
    }
}

function resumeRealtimeAudioPlayback() {
    const audio = audioRef.value;
    if (
        !audio ||
        userPausedAudio ||
        !audio.paused ||
        !audio.src ||
        pendingResumePlayback
    ) {
        return;
    }

    const playPromise = audio.play();
    if (!playPromise || typeof playPromise.then !== "function") {
        return;
    }

    pendingResumePlayback = playPromise
        .catch((err) => {
            if (err instanceof DOMException && err.name === "AbortError") {
                console.debug(
                    "[Audio] Resume playback bị ngắt do pause hoặc teardown.",
                );
                return;
            }
            console.error("[Audio] Failed to resume playback:", err);
        })
        .finally(() => {
            pendingResumePlayback = null;
        });
}

function flushAudioChunkQueue() {
    const sourceBuffer = activeSourceBuffer;
    if (!sourceBuffer || !isSourceBufferAttached(sourceBuffer)) return;

    try {
        if (sourceBuffer.updating) return;
    } catch {
        return;
    }

    // Kiểm tra buffered time để quản lý playback
    let bufferedSeconds = 0;
    let currentTime = 0;
    try {
        if (sourceBuffer.buffered.length > 0) {
            bufferedSeconds = sourceBuffer.buffered.end(
                sourceBuffer.buffered.length - 1,
            );
        }
        if (audioRef.value && !isNaN(audioRef.value.currentTime)) {
            currentTime = audioRef.value.currentTime;
        }
    } catch {
        bufferedSeconds = 0;
    }

    const remainingBuffer = bufferedSeconds - currentTime;

    // === FIX: SourceBuffer overflow prevention ===
    // Khi buffer > 30s, xóa các range cũ để giải phóng bộ nhớ
    // Giữ lại 15s buffer phía trước vị trí hiện tại
    const MAX_BUFFER_SECONDS = 30;
    const KEEP_BUFFER_SECONDS = 15;

    if (
        remainingBuffer > MAX_BUFFER_SECONDS &&
        sourceBuffer.buffered.length > 0
    ) {
        try {
            const removeEnd = Math.max(0, currentTime - KEEP_BUFFER_SECONDS);
            for (let i = 0; i < sourceBuffer.buffered.length; i++) {
                const rangeEnd = sourceBuffer.buffered.end(i);
                const rangeStart = sourceBuffer.buffered.start(i);
                if (rangeEnd <= removeEnd) {
                    console.log(
                        `[Audio] Removing old buffer range [${rangeStart.toFixed(1)}s - ${rangeEnd.toFixed(1)}s] to prevent overflow`,
                    );
                    sourceBuffer.remove(rangeStart, rangeEnd);
                    // Chờ updateend rồi mới append chunk mới
                    return;
                }
            }
        } catch (err) {
            console.warn("[Audio] Failed to remove old buffer:", err);
        }
    }

    // Nếu buffer sắp hết (< 2s) và đang play, pause để tránh lỗi
    // Nhưng KHÔNG reset audio element - giữ nguyên currentTime
    if (
        remainingBuffer < 2 &&
        audioRef.value &&
        !audioRef.value.paused &&
        !userPausedAudio
    ) {
        console.log(
            `[Audio] Buffer low (${remainingBuffer.toFixed(1)}s at ${currentTime.toFixed(1)}s), pausing to wait for backend retry...`,
        );
        audioRef.value.pause();
    }

    if (audioRef.value?.paused && !userPausedAudio) {
        // Seek vào segment đã render sẵn chỉ cần buffer rất ngắn để phát ngay.
        if (
            bufferedSeconds >= resumeBufferThresholdSeconds ||
            pendingMediaEnd
        ) {
            console.log(
                `[Audio] Buffer sufficient (${bufferedSeconds.toFixed(1)}s / threshold ${resumeBufferThresholdSeconds.toFixed(2)}s), resuming from ${currentTime.toFixed(1)}s...`,
            );
            void resumeRealtimeAudioPlayback();
        }
    }

    if (queuedAudioChunks.length > 0) {
        const chunk = queuedAudioChunks.shift();
        if (!chunk) return;
        const buffer = chunk.buffer.slice(
            chunk.byteOffset,
            chunk.byteOffset + chunk.byteLength,
        ) as ArrayBuffer;
        try {
            sourceBuffer.appendBuffer(buffer);
            console.log(
                `[Audio] Appended chunk: ${buffer.byteLength} bytes, total buffered: ${bufferedSeconds.toFixed(1)}s`,
            );
        } catch (err: unknown) {
            const error = err as Error;
            // === FIX: Handle QuotaExceededError gracefully ===
            if (error.name === "QuotaExceededError") {
                console.warn(
                    `[Audio] SourceBuffer full (buffered=${bufferedSeconds.toFixed(1)}s). Forcing cleanup and retry...`,
                );
                try {
                    // Xóa buffer cũ nhất để giải phóng
                    if (sourceBuffer.buffered.length > 0) {
                        const removeEnd = Math.max(0, currentTime - 5);
                        const rangeEnd = sourceBuffer.buffered.end(0);
                        if (rangeEnd <= removeEnd) {
                            sourceBuffer.remove(
                                sourceBuffer.buffered.start(0),
                                rangeEnd,
                            );
                            console.log(
                                "[Audio] Emergency buffer cleanup done. Retrying append...",
                            );
                            // Lưu chunk lại để retry sau khi updateend
                            queuedAudioChunks.unshift(chunk);
                            return;
                        }
                    }
                } catch (cleanupErr) {
                    console.error(
                        "[Audio] Emergency cleanup failed:",
                        cleanupErr,
                    );
                }
            } else {
                console.error("[Audio] Failed to append chunk:", error);
            }
            // Có thể bị teardown đúng lúc append; bỏ qua để tránh lỗi runtime.
        }
        return;
    }

    if (
        pendingMediaEnd &&
        activeMediaSource &&
        activeMediaSource.readyState === "open"
    ) {
        try {
            activeMediaSource.endOfStream();
        } catch {
            // Bỏ qua nếu MediaSource đã tự kết thúc.
        }
    }
}

function finishRealtimeStream() {
    pendingMediaEnd = true;
    flushAudioChunkQueue();
    // Reset flags khi kết thúc stream
    shouldAutoScroll.value = false;
    isPlaybackActive = false; // Đánh dấu kết thúc phiên playback
}

function resetRealtimeMediaStream(force = false) {
    // KHÔNG reset nếu đang trong phiên playback active
    // Điều này tránh audio bị restart khi backend đang retry chunk
    if (
        !force &&
        isPlaybackActive &&
        activeMediaSource &&
        activeMediaSource.readyState === "open"
    ) {
        console.log(
            "[MediaStream] Skipping reset during active playback to avoid audio restart",
        );
        return;
    }

    pendingMediaEnd = false;
    pendingResumePlayback = null;
    queuedAudioChunks = [];
    userPausedAudio = false;
    isPlaybackActive = false;
    setResumeBufferThreshold(4);
    resetAudioTimelineState();

    if (activeSourceBuffer) {
        activeSourceBuffer.removeEventListener(
            "updateend",
            flushAudioChunkQueue,
        );
        try {
            if (activeMediaSource?.readyState === "open") {
                activeSourceBuffer.abort();
            }
        } catch {
            // Ignore abort errors while tearing down the stream.
        }
    }

    if (audioRef.value) {
        audioRef.value.pause();
        audioRef.value.removeAttribute("src");
        audioRef.value.load();
    }

    if (activeMediaUrl) {
        URL.revokeObjectURL(activeMediaUrl);
    }

    activeSourceBuffer = null;
    activeMediaSource = null;
    activeMediaUrl = "";
}

function connectRealtimeSocket(baseUrl: string, sessionId: string) {
    teardownRealtimeSocket();

    const wsUrl = buildWebSocketUrl(baseUrl, sessionId);
    console.log(`[RealtimeSocket] Connecting to ${wsUrl}`);

    activeSocket = new ReconnectWebSocket(wsUrl, {
        maxRetries: 10,
        baseDelayMs: 1000,
        maxDelayMs: 30000,
        onMessage: (data) => {
            handleRealtimeEvent(JSON.parse(data) as RealtimeSessionState);
        },
        onBinary: (buffer) => {
            appendRealtimeChunk(buffer);
        },
        onStateChange: (state) => {
            if (state === "open") {
                // Connection restored, update status if it was error
                if (realtimeStatus.value === "error") {
                    realtimeStatus.value = "reading";
                    realtimeError.value = "";
                }
            } else if (state === "error") {
                realtimeStatus.value = "error";
                realtimeError.value =
                    "Kết nối WebSocket realtime TTS bị lỗi sau khi đã thử kết nối lại.";
            }
        },
        onError: (error) => {
            console.error("[RealtimeSocket] Error:", error);
            realtimeError.value =
                "Kết nối WebSocket realtime TTS bị lỗi. Đang thử kết nối lại...";
            toast.warning("Mất kết nối TTS. Đang thử kết nối lại...");
        },
    });

    activeSocket.connect();
}

function teardownRealtimeSocket() {
    if (!activeSocket) return;
    activeSocket.close();
    activeSocket = null;
}

function handleRealtimeEvent(event: RealtimeSessionState) {
    if (event.type === "audio_format" && event.mime) {
        currentStreamMime.value = event.mime;
        return;
    }

    switch (event.type) {
        case "controls_updated":
            if (realtimeSession.value) {
                realtimeSession.value = {
                    ...realtimeSession.value,
                    voice: event.voice ?? realtimeSession.value.voice,
                    speed: event.speed ?? realtimeSession.value.speed,
                    pitch: event.pitch ?? realtimeSession.value.pitch,
                    autoNext: event.autoNext ?? realtimeSession.value.autoNext,
                };
            }
            break;
        case "session_started":
            realtimeStatus.value = "buffering";
            realtimeError.value = "";
            resetRealtimeSegments();
            break;
        case "chapter_segments":
            if (typeof event.chapterId === "number") {
                syncRealtimeSegments(
                    event.chapterId,
                    event.chapterIndex ?? 0,
                    event.chapterTitle ??
                        `Chương ${event.chapterIndex ?? ""}`.trim(),
                    event.segments ?? [],
                    event.startSegmentIndex ?? 0,
                );
            }
            break;
        case "segment_rendering":
            if (
                typeof event.chapterId === "number" &&
                typeof event.segmentIndex === "number"
            ) {
                patchRealtimeChapterGroup(event.chapterId, {
                    chapterIndex: event.chapterIndex,
                    chapterTitle: event.chapterTitle,
                    status: "rendering",
                });
                patchRealtimeSegment(
                    event.chapterId,
                    event.segmentIndex,
                    {
                        status: "rendering",
                        attempt: event.attempt ?? 1,
                        message: "",
                    },
                    event.totalSegments,
                );
            }
            break;
        case "segment_ready":
            if (
                typeof event.chapterId === "number" &&
                typeof event.segmentIndex === "number"
            ) {
                patchRealtimeSegment(
                    event.chapterId,
                    event.segmentIndex,
                    {
                        status: "ready",
                        attempt:
                            event.attempt ??
                            ensureRealtimeSegment(
                                event.chapterId,
                                event.segmentIndex,
                                event.totalSegments,
                            ).attempt,
                        message: "",
                        durationEstimate:
                            event.durationEstimate ??
                            ensureRealtimeSegment(
                                event.chapterId,
                                event.segmentIndex,
                                event.totalSegments,
                            ).durationEstimate,
                    },
                    event.totalSegments,
                );
            }
            break;
        case "segment_retry":
            if (
                typeof event.chapterId === "number" &&
                typeof event.segmentIndex === "number"
            ) {
                patchRealtimeChapterGroup(event.chapterId, {
                    chapterIndex: event.chapterIndex,
                    chapterTitle: event.chapterTitle,
                    status: "rendering",
                });
                patchRealtimeSegment(
                    event.chapterId,
                    event.segmentIndex,
                    {
                        status: "retrying",
                        attempt:
                            event.attempt ??
                            ensureRealtimeSegment(
                                event.chapterId,
                                event.segmentIndex,
                                event.totalSegments,
                            ).attempt + 1,
                        message: event.message ?? "Render lỗi, đang thử lại.",
                    },
                    event.totalSegments,
                );
            }
            break;
        case "segment_started":
            if (
                typeof event.chapterId === "number" &&
                typeof event.segmentIndex === "number"
            ) {
                const hasPendingSeek = pendingSeekTarget.value !== null;
                const isPendingSeekStart =
                    pendingSeekTarget.value?.chapterId === event.chapterId &&
                    pendingSeekTarget.value?.segmentIndex ===
                        event.segmentIndex;
                if (hasPendingSeek && !isPendingSeekStart) {
                    console.log(
                        `[Seek] Ignoring stale segment_started ${event.chapterId}:${event.segmentIndex} while waiting for ${pendingSeekTarget.value?.chapterId}:${pendingSeekTarget.value?.segmentIndex}`,
                    );
                    break;
                }
                if (isPendingSeekStart) {
                    clearPendingSeekFallbackTimer();
                    pendingSeekTarget.value = null;
                    dropIncomingAudioUntilSeekStart = false;
                    playbackAutoSyncLockChapterId.value = null;
                }
                appendPlaybackTimelineEntry({
                    chapterId: event.chapterId,
                    chapterIndex: event.chapterIndex,
                    chapterTitle: event.chapterTitle,
                    segmentIndex: event.segmentIndex,
                    durationEstimate: event.durationEstimate,
                });
                if (isPendingSeekStart) {
                    realtimeStatus.value = "buffering";
                }
                patchRealtimeChapterGroup(event.chapterId, {
                    chapterIndex: event.chapterIndex,
                    chapterTitle: event.chapterTitle,
                    status: "reading",
                });
                patchRealtimeSegment(
                    event.chapterId,
                    event.segmentIndex,
                    {
                        attempt:
                            event.attempt ??
                            ensureRealtimeSegment(
                                event.chapterId,
                                event.segmentIndex,
                                event.totalSegments,
                            ).attempt,
                        message: "",
                        durationEstimate:
                            event.durationEstimate ??
                            ensureRealtimeSegment(
                                event.chapterId,
                                event.segmentIndex,
                                event.totalSegments,
                            ).durationEstimate,
                    },
                    event.totalSegments,
                );
            }
            break;
        case "segment_finished":
            if (
                typeof event.chapterId === "number" &&
                typeof event.segmentIndex === "number"
            ) {
                patchRealtimeSegment(
                    event.chapterId,
                    event.segmentIndex,
                    {
                        message: "",
                    },
                    event.totalSegments,
                );
            }
            break;
        case "chunk_started":
        case "chunk_finished":
            break;
        case "chapter_started":
            realtimeStatus.value = "reading";
            realtimeBufferedChapterId.value = event.chapterId ?? null;
            realtimeBufferedChapterTitle.value = event.chapterTitle ?? "";
            if (typeof event.chapterId === "number") {
                patchRealtimeChapterGroup(event.chapterId, {
                    chapterIndex: event.chapterIndex,
                    chapterTitle: event.chapterTitle,
                    status: "rendering",
                });
            }
            break;
        case "chapter_finished":
            realtimeStatus.value = pendingSeekTarget.value
                ? "buffering"
                : "transitioning";
            if (typeof event.chapterId === "number") {
                patchRealtimeChapterGroup(event.chapterId, {
                    status: "completed",
                });
            }
            void persistProgress(0);
            break;
        case "chapter_transition":
            realtimeStatus.value =
                pendingSeekTarget.value || event.reason === "seek"
                    ? "buffering"
                    : "transitioning";
            break;
        case "story_finished":
            realtimeStatus.value = "finished";
            realtimePlaybackCursor.value = null;
            clearPendingSeekFallbackTimer();
            pendingSeekTarget.value = null;
            dropIncomingAudioUntilSeekStart = false;
            finishRealtimeStream();
            // Reset scroll flags khi kết thúc truyện
            shouldAutoScroll.value = false;
            userHasScrolled = false;
            break;
        case "stopped":
            realtimeStatus.value = "stopped";
            realtimePlaybackCursor.value = null;
            clearPendingSeekFallbackTimer();
            pendingSeekTarget.value = null;
            dropIncomingAudioUntilSeekStart = false;
            finishRealtimeStream();
            // Reset scroll flags khi dừng
            shouldAutoScroll.value = false;
            userHasScrolled = false;
            break;
        case "stream_closed":
            if (
                !["finished", "stopped", "error"].includes(realtimeStatus.value)
            ) {
                realtimeStatus.value =
                    event.status === "completed" ? "finished" : "stopped";
            }
            realtimePlaybackCursor.value = null;
            clearPendingSeekFallbackTimer();
            pendingSeekTarget.value = null;
            dropIncomingAudioUntilSeekStart = false;
            finishRealtimeStream();
            break;
        case "error":
            realtimeStatus.value = "error";
            realtimePlaybackCursor.value = null;
            clearPendingSeekFallbackTimer();
            pendingSeekTarget.value = null;
            dropIncomingAudioUntilSeekStart = false;
            realtimeError.value = event.message || "Realtime TTS gặp lỗi.";
            error.value = realtimeError.value;
            toast.error(`Realtime TTS lỗi: ${realtimeError.value}`);
            finishRealtimeStream();
            break;
    }
}

function chapterAt(offset: number) {
    if (!selectedStory.value || !selectedChapter.value) return null;
    const currentIndex = selectedStory.value.chapters.findIndex(
        (chapter) => chapter.id === selectedChapter.value?.id,
    );
    return selectedStory.value.chapters[currentIndex + offset] ?? null;
}

async function goToSiblingChapter(offset: number) {
    const target = chapterAt(offset);
    if (!target) return;

    const shouldResumeRealtime = isRealtimeActive.value;
    const shouldResumeEdgeReadAloud = edgeReadAloudActive.value;
    if (shouldResumeEdgeReadAloud) {
        await stopEdgeReadAloudPlayback();
    }
    await loadChapter(target.id, null, {
        preservePlayback: shouldResumeRealtime,
        skipPersist: shouldResumeRealtime,
    });

    // Chỉ đánh dấu chương đọc tay; khi realtime đang chạy thì progress bám theo chapter audio thực.
    if (selectedStory.value && !shouldResumeRealtime) {
        const chapter = selectedStory.value.chapters.find(
            (c) => c.id === target.id,
        );
        if (chapter) {
            await markChapterRead(
                selectedStory.value.story.id,
                chapter.chapterIndex,
            );
        }
    }

    if (shouldResumeEdgeReadAloud) {
        await startEdgeReadAloudPlayback();
    }
}

watch(selectedVoice, () => {
    persistRealtimePreferences();
    scheduleRealtimeControlsSync();
});

watch(realtimeSpeed, () => {
    realtimeSpeed.value = clampRealtimeSpeed(realtimeSpeed.value);
    persistRealtimePreferences();
    scheduleRealtimeRestart();
});

watch(realtimePitch, () => {
    realtimePitch.value = clampRealtimePitch(realtimePitch.value);
    persistRealtimePreferences();
    scheduleRealtimeRestart();
});

watch(useEdgeReadAloud, () => {
    if (!isEdgeBrowser.value && useEdgeReadAloud.value) {
        useEdgeReadAloud.value = false;
        error.value =
            "Edge Read Aloud mode chỉ dùng được trong Microsoft Edge.";
    }
    persistEdgeReadAloudPreferences();
});

watch(edgeReadAloudWordsPerMinute, () => {
    edgeReadAloudWordsPerMinute.value = Math.min(
        260,
        Math.max(120, Math.round(edgeReadAloudWordsPerMinute.value || 185)),
    );
    persistEdgeReadAloudPreferences();
    if (edgeReadAloudActive.value) {
        scheduleEdgeReadAloudAutoNext();
    }
});

watch(readerFontSize, () => {
    readerFontSize.value = clampReaderFontSize(readerFontSize.value);
    persistReaderPreferences();
});

watch(activeWordGlobalIndex, (nextWord, prevWord) => {
    if (nextWord === null || nextWord === prevWord) return;
    if (!userHasScrolled) return;
    setTimeout(() => {
        scrollActiveWordIntoView();
    }, 40);
});

watch(
    () => actualPlaybackLocation.value?.chapterId ?? null,
    (chapterId) => {
        if (chapterId) {
            realtimeAudibleChapterId.value = chapterId;
            const chapterTitle =
                sortedRealtimeChapterGroups.value.find(
                    (group) => group.chapterId === chapterId,
                )?.chapterTitle ??
                selectedStory.value?.chapters.find(
                    (chapter) => chapter.id === chapterId,
                )?.title ??
                "";
            if (chapterTitle) {
                realtimeAudibleChapterTitle.value = chapterTitle;
            }
        }

        if (playbackAutoSyncLockChapterId.value !== null) {
            if (chapterId === playbackAutoSyncLockChapterId.value) {
                playbackAutoSyncLockChapterId.value = null;
            } else {
                return;
            }
        }

        if (
            !chapterId ||
            !isRealtimeActive.value ||
            pendingSeekTarget.value !== null ||
            selectedChapterId.value === chapterId ||
            autoSyncingPlaybackChapterId.value === chapterId
        ) {
            return;
        }

        autoSyncingPlaybackChapterId.value = chapterId;
        void loadChapter(chapterId, null, {
            preservePlayback: true,
            skipPersist: true,
        }).finally(() => {
            if (autoSyncingPlaybackChapterId.value === chapterId) {
                autoSyncingPlaybackChapterId.value = null;
            }
        });
    },
);

onMounted(() => {
    try {
        const raw = window.localStorage.getItem(edgeReadAloudPrefsKey);
        if (raw) {
            const parsed = JSON.parse(raw) as {
                enabled?: boolean;
                wordsPerMinute?: number;
            };
            useEdgeReadAloud.value = Boolean(parsed.enabled);
            edgeReadAloudWordsPerMinute.value = Math.min(
                260,
                Math.max(120, Math.round(parsed.wordsPerMinute ?? 185)),
            );
        }
    } catch {
        useEdgeReadAloud.value = false;
        edgeReadAloudWordsPerMinute.value = 185;
    }
    try {
        const raw = window.localStorage.getItem(readerPrefsKey);
        if (raw) {
            const parsed = JSON.parse(raw) as { fontSize?: number };
            readerFontSize.value = clampReaderFontSize(parsed.fontSize ?? 18);
        }
    } catch {
        readerFontSize.value = 18;
    }

    // Lắng nghe scroll event để bật auto-scroll khi user scroll thủ công
    const setupScrollListener = () => {
        if (readerBody.value) {
            readerBody.value.addEventListener(
                "scroll",
                () => {
                    userHasScrolled = true;
                    shouldAutoScroll.value = true;
                },
                { passive: true },
            );
        }
    };

    // Đợi reader render xong rồi attach listener
    nextTick(() => {
        setupScrollListener();
    });

    void restoreDirectoryHandle();

    // Restore last-read position and resume
    const lastRead = readLastReadPosition();
    void refresh(lastRead.storyId ?? undefined, undefined, lastRead);
});

onUnmounted(() => {
    clearEdgeReadAloudTimer();
    clearRealtimeControlSyncTimer();
    clearRealtimeRestartTimer();
    if (progressTimer !== null) {
        window.clearTimeout(progressTimer);
    }
    void stopRealtimePlayback({ quiet: true, clearSession: true });
});
</script>

<template>
    <!-- Toast Notifications -->
    <ToastContainer />

    <main
        class="app-shell"
        :class="{
            'edge-read-aloud-shell': activeTab === 'reader' && useEdgeReadAloud,
        }"
    >
        <input
            ref="folderInput"
            class="hidden-input"
            type="file"
            webkitdirectory
            directory
            multiple
            @change="importFromFolder"
        />

        <header
            class="app-header panel"
            :class="{
                'edge-read-aloud-hidden':
                    activeTab === 'reader' && useEdgeReadAloud,
            }"
        >
            <div class="branding">
                <div class="logo-icon">📚</div>
                <div>
                    <h1 class="app-title">Story-TTS Reader</h1>
                    <p class="app-subtitle">
                        Thư viện truyện local, đọc tiếp nhanh và nghe realtime
                        TTS
                    </p>
                </div>
            </div>

            <div class="header-actions">
                <button
                    class="ghost-button"
                    @click="refreshLibrary"
                    :disabled="loading || !canRefreshLibrary"
                >
                    Làm mới thư viện
                </button>
                <button @click="chooseLibraryFolder" :disabled="loading">
                    Chọn thư mục truyện
                </button>
            </div>
        </header>

        <p v-if="error" class="error-banner">{{ error }}</p>

        <!-- Import Progress Overlay -->
        <div v-if="importProgress.active" class="import-progress-overlay">
            <div class="import-progress-card">
                <div class="progress-header">
                    <div
                        class="spinner"
                        :class="{
                            'spinner-done': importProgress.phase === 'done',
                        }"
                    ></div>
                    <h3>{{ progressTitle }}</h3>
                </div>

                <div class="progress-phases">
                    <div
                        class="phase-indicator"
                        :class="{
                            active: importProgress.phase === 'reading',
                            done: isPhaseDone('reading'),
                        }"
                    >
                        📖 Đọc file
                    </div>
                    <div
                        class="phase-indicator"
                        :class="{
                            active: importProgress.phase === 'sending',
                            done: isPhaseDone('sending'),
                        }"
                    >
                        📤 Gửi lên server
                    </div>
                    <div
                        class="phase-indicator"
                        :class="{
                            active: importProgress.phase === 'processing',
                            done: isPhaseDone('processing'),
                        }"
                    >
                        ⚙️ Xử lý
                    </div>
                </div>

                <div class="progress-bar-container">
                    <div
                        class="progress-bar"
                        :style="{ width: progressPercentage + '%' }"
                    ></div>
                </div>

                <p class="progress-message">{{ importProgress.message }}</p>

                <p
                    v-if="importProgress.phase === 'error'"
                    class="progress-error"
                >
                    <button
                        @click="importProgress.active = false"
                        class="btn-dismiss"
                    >
                        Đóng
                    </button>
                </p>
            </div>
        </div>

        <nav
            class="app-tabs"
            :class="{
                'edge-read-aloud-hidden':
                    activeTab === 'reader' && useEdgeReadAloud,
            }"
        >
            <button
                :class="{ active: activeTab === 'library' }"
                @click="activeTab = 'library'"
            >
                📋 Quản lý Thư viện
            </button>
            <button
                :class="{ active: activeTab === 'reader' }"
                @click="activeTab = 'reader'"
                :disabled="!selectedStory"
            >
                📖 Trình phát & Đọc chữ
            </button>
        </nav>

        <section
            v-if="activeTab === 'library'"
            class="workspace library-workspace"
        >
            <div class="library-main">
                <section class="library-summary panel">
                    <div class="sidebar-head">
                        <div>
                            <p class="eyebrow">Thư viện</p>
                            <h2>{{ stories.length }} truyện</h2>
                        </div>
                        <p v-if="state" class="meta">
                            Voice mặc định:
                            {{ state.config.realtimeDefaultVoice }}
                        </p>
                    </div>

                    <p v-if="currentLibraryRoot" class="meta library-root">
                        Đang theo dõi: {{ currentLibraryRoot }}
                    </p>

                    <div v-if="recentStories.length" class="recent-strip">
                        <button
                            v-for="story in recentStories"
                            :key="`recent-${story.id}`"
                            class="recent-chip"
                            @click="selectStory(story.id)"
                        >
                            {{ story.title }}
                        </button>
                    </div>
                </section>

                <section class="story-gallery">
                    <article
                        v-for="story in sortedStories"
                        :key="story.id"
                        class="story-card compact-story-card panel gallery-story-card"
                        :class="{
                            active:
                                selectedStory != null &&
                                selectedStory.story.id === story.id,
                        }"
                        @click.stop="selectStory(story.id)"
                    >
                        <div class="story-card-top">
                            <strong>{{ story.title }}</strong>
                            <span class="story-chapter-total"
                                >{{ story.chapterCount }} chương</span
                            >
                        </div>

                        <p class="story-progress-line">
                            {{ storyProgressText(story) }}
                        </p>
                        <span class="story-path">{{ story.sourcePath }}</span>

                        <div class="story-card-actions">
                            <span v-if="story.lastOpenedAt" class="story-time"
                                >Mở gần nhất:
                                {{ formatDateTime(story.lastOpenedAt) }}</span
                            >
                            <button
                                class="continue-button"
                                @click="continueStory(story.id, $event)"
                            >
                                {{ storyContinueText(story) }}
                            </button>
                        </div>
                    </article>
                </section>
            </div>

            <div v-if="selectedStory" class="chapter-column panel">
                <div class="column-head">
                    <div>
                        <p class="eyebrow">Chương</p>
                        <h2>{{ selectedStory.story.title }}</h2>
                    </div>

                    <label class="preset-select">
                        <span>Preset reader</span>
                        <select v-model="currentPreset">
                            <option
                                v-for="preset in presets"
                                :key="preset"
                                :value="preset"
                            >
                                {{ preset }}
                            </option>
                        </select>
                    </label>
                </div>

                <div class="chapter-list">
                    <button
                        v-for="chapter in selectedStory.chapters"
                        :key="chapter.id"
                        class="chapter-card"
                        :class="{
                            active:
                                selectedChapterId === chapter.id ||
                                currentPlaybackChapterId === chapter.id,
                            'chapter-read': isChapterRead(chapter),
                        }"
                        @click="selectChapter(chapter.id, $event)"
                    >
                        <strong>Chương {{ chapter.chapterIndex }}</strong>
                        <span>{{ chapter.title }}</span>
                    </button>
                </div>
            </div>
        </section>

        <section
            v-if="activeTab === 'reader'"
            class="workspace reader-workspace"
        >
            <section class="reader-shell">
                <article
                    v-if="selectedStory && chapterContent"
                    class="reader-column panel centered-reader"
                    :class="{ 'edge-read-aloud-mode': useEdgeReadAloud }"
                >
                    <nav
                        v-if="!useEdgeReadAloud"
                        class="reader-pane-tabs"
                    >
                        <button
                            :class="{ active: readerPaneTab === 'text' }"
                            @click="readerPaneTab = 'text'"
                        >
                            Đọc chữ
                        </button>
                        <button
                            :class="{ active: readerPaneTab === 'console' }"
                            @click="readerPaneTab = 'console'"
                        >
                            Player + Segment
                        </button>
                    </nav>
                    <div
                        class="reader-layout"
                        :class="{
                            'edge-read-aloud-layout': useEdgeReadAloud,
                            'is-console-only': showReaderConsole,
                            'is-text-only': showReaderText && !useEdgeReadAloud,
                        }"
                    >
                        <aside
                            v-if="showReaderConsole"
                            class="premium-player-column"
                        >
                            <div class="player-console-head">
                                <p class="eyebrow">Realtime Console</p>
                                <h3>
                                    {{
                                        currentPlayingChapterTitle
                                    }}
                                </h3>
                                <p>{{ chapterContent.storyTitle }}</p>
                                <p v-if="bufferedAheadSummary">
                                    {{ bufferedAheadSummary }}
                                </p>
                            </div>

                            <section class="console-media-strip">
                                <div class="console-media-strip__meta">
                                    <p class="console-media-strip__eyebrow">
                                        Đang phát
                                    </p>
                                    <h4>{{ currentSegmentProgressLabel }}</h4>
                                    <p class="console-media-strip__text">
                                        {{ currentSegmentPreviewText }}
                                    </p>
                                </div>
                                <div class="console-media-strip__controls">
                                    <span
                                        class="player-status-pill"
                                        :class="`is-${realtimeStatus}`"
                                        >{{ realtimeStatusLabel }}</span
                                    >
                                    <span class="player-voice-label">{{
                                        currentVoiceLabel
                                    }}</span>
                                    <button
                                        class="neume-btn play-pause-btn console-play-btn"
                                        @click="togglePlayback"
                                        :class="{
                                            'is-playing':
                                                audioIsPlaying ||
                                                edgeReadAloudActive,
                                        }"
                                    >
                                        <span
                                            class="center-glyph"
                                            :class="{
                                                'is-playing':
                                                    audioIsPlaying ||
                                                    edgeReadAloudActive,
                                            }"
                                        ></span>
                                    </button>
                                </div>
                                <div class="console-media-strip__progress">
                                    <div
                                        class="progress-bar-container progress-bar-container--readonly progress-bar-container--hero"
                                    >
                                        <div
                                            class="progress-filled progress-filled--hero"
                                            :style="{
                                                width:
                                                    currentSegmentProgressPercent +
                                                    '%',
                                            }"
                                        ></div>
                                    </div>
                                    <div class="time-labels time-labels--hero">
                                        <span>{{
                                            formatDuration(
                                                currentSegmentElapsedSeconds,
                                            )
                                        }}</span>
                                        <span>{{
                                            formatDuration(
                                                currentSegmentDurationSeconds,
                                            )
                                        }}</span>
                                    </div>
                                </div>
                            </section>

                            <div class="console-split-layout">

                            <div class="premium-player-card">
                                <div class="card-head">
                                    <button
                                        class="card-head-icon nav-icon"
                                        @click="goToSiblingChapter(-1)"
                                        :disabled="!chapterAt(-1)"
                                    >
                                        ‹
                                    </button>
                                    <p class="card-head-title">NOW PLAYING</p>
                                    <div class="card-head-actions">
                                        <button
                                            class="card-head-icon tune-trigger"
                                            title="Tinh chỉnh"
                                        >
                                            ⋮
                                        </button>
                                        <div class="player-settings-hover">
                                            <div class="hover-voice-row">
                                                <span class="setting-label"
                                                    >Giọng đọc</span
                                                >
                                                <select
                                                    class="hover-voice-select"
                                                    v-model="selectedVoice"
                                                    :disabled="
                                                        realtimeConnecting ||
                                                        realtimeVoiceOptions.length ===
                                                            0
                                                    "
                                                >
                                                    <option
                                                        v-for="voice in realtimeVoiceOptions"
                                                        :key="voice.id"
                                                        :value="voice.id"
                                                    >
                                                        {{ voice.friendlyName }}
                                                    </option>
                                                </select>
                                            </div>
                                            <div class="hover-sliders">
                                                <div class="hover-slider-col">
                                                    <span class="setting-label"
                                                        >Tốc độ</span
                                                    >
                                                    <div
                                                        class="vertical-slider-track"
                                                    >
                                                        <input
                                                            class="vertical-range"
                                                            v-model.number="
                                                                realtimeSpeed
                                                            "
                                                            type="range"
                                                            min="-50"
                                                            max="50"
                                                            step="5"
                                                        />
                                                    </div>
                                                    <strong
                                                        >{{
                                                            realtimeSpeed
                                                        }}%</strong
                                                    >
                                                </div>
                                                <div class="hover-slider-col">
                                                    <span class="setting-label"
                                                        >Cao độ</span
                                                    >
                                                    <div
                                                        class="vertical-slider-track"
                                                    >
                                                        <input
                                                            class="vertical-range"
                                                            v-model.number="
                                                                realtimePitch
                                                            "
                                                            type="range"
                                                            min="-80"
                                                            max="80"
                                                            step="5"
                                                        />
                                                    </div>
                                                    <strong
                                                        >{{
                                                            realtimePitch
                                                        }}Hz</strong
                                                    >
                                                </div>
                                            </div>
                                        </div>
                                    </div>
                                </div>

                                <div class="player-artwork">
                                    <div class="artwork-inner">
                                        <span class="artwork-emoji">📖</span>
                                    </div>
                                    <div class="artwork-glow"></div>
                                </div>

                                <div class="player-info-block">
                                    <h3 class="playing-title">
                                        <!-- === FIX: Show the chapter currently displayed in reader, not TTS cursor chapter === -->
                                        {{
                                            currentPlayingChapterTitle
                                        }}
                                    </h3>
                                    <p class="playing-subtitle">
                                        {{ chapterContent.storyTitle }}
                                    </p>
                                    <div class="player-status-row">
                                        <span
                                            class="player-status-pill"
                                            :class="`is-${realtimeStatus}`"
                                            >{{ realtimeStatusLabel }}</span
                                        >
                                        <span class="player-voice-label">{{
                                            currentVoiceLabel
                                        }}</span>
                                    </div>
                                    <p class="player-status-text">
                                        {{ realtimeStatusText }}
                                    </p>
                                </div>

                                <div class="player-main-controls">
                                    <button
                                        class="neume-btn heart-btn"
                                        title="Yêu thích"
                                    >
                                        <span class="heart-glyph">❤</span>
                                    </button>
                                    <button
                                        class="neume-btn play-pause-btn"
                                        @click="togglePlayback"
                                        :class="{
                                            'is-playing':
                                                audioIsPlaying ||
                                                edgeReadAloudActive,
                                        }"
                                    >
                                        <span
                                            class="center-glyph"
                                            :class="{
                                                'is-playing':
                                                    audioIsPlaying ||
                                                    edgeReadAloudActive,
                                            }"
                                        ></span>
                                    </button>
                                </div>

                                <div class="player-progress-section">
                                    <p class="player-progress-caption">
                                        {{ currentSegmentProgressLabel }}
                                    </p>
                                    <div
                                        class="progress-bar-container progress-bar-container--readonly"
                                    >
                                        <div
                                            class="progress-filled"
                                            :style="{
                                                width:
                                                    currentSegmentProgressPercent +
                                                    '%',
                                            }"
                                        ></div>
                                        <div
                                            class="progress-knob"
                                            :style="{
                                                left:
                                                    currentSegmentProgressPercent +
                                                    '%',
                                            }"
                                        ></div>
                                    </div>
                                    <div class="time-labels">
                                        <span>{{
                                            formatDuration(
                                                currentSegmentElapsedSeconds,
                                            )
                                        }}</span>
                                        <span>{{
                                            formatDuration(
                                                currentSegmentDurationSeconds,
                                            )
                                        }}</span>
                                    </div>
                                </div>

                                <div class="lyrics-hint">
                                    <span class="lyrics-arrow">⌃</span>
                                    <span>{{
                                        visiblePlayingSegmentRange ||
                                        'Khung theo dõi segment hiển thị toàn bộ phần đã nạp/render'
                                    }}</span>
                                </div>
                            </div>

                            <section
                                v-if="sortedRealtimeChapterGroups.length"
                                class="segment-status-card segment-status-card--main"
                            >
                                <div class="segment-status-head">
                                    <div>
                                        <p class="segment-status-kicker">
                                            Tiến trình segment
                                        </p>
                                        <strong
                                            >{{
                                                realtimeSegmentMetrics.played
                                            }}/{{
                                                realtimeSegmentMetrics.total
                                            }}
                                            đã đọc</strong
                                        >
                                        <p class="segment-window-note">
                                            {{
                                                visiblePlayingSegmentRange ||
                                                'Khung hiển thị được nhóm theo chapter và bám theo phần đã nạp/render'
                                            }}
                                        </p>
                                    </div>
                                    <div class="segment-status-metrics">
                                        <span
                                            >{{
                                                realtimeSegmentMetrics.rendering
                                            }}
                                            đang tạo</span
                                        >
                                        <span
                                            >{{
                                                realtimeSegmentMetrics.ready
                                            }}
                                            sẵn sàng</span
                                        >
                                        <span
                                            >{{
                                                realtimeSegmentMetrics.reading
                                            }}
                                            đang đọc</span
                                        >
                                    </div>
                                </div>

                                <div class="segment-status-list">
                                    <section
                                        v-for="entry in renderChapterGroups"
                                        :key="`realtime-group-${entry.group.chapterId}`"
                                        class="segment-chapter-group"
                                        :class="{
                                            'is-reading': entry.isPlaybackChapter,
                                        }"
                                    >
                                        <div class="segment-chapter-head">
                                            <div>
                                                <strong
                                                    >Chương
                                                    {{
                                                        entry.group.chapterIndex
                                                    }}</strong
                                                >
                                                <p>
                                                    {{
                                                        entry.group.chapterTitle
                                                    }}
                                                </p>
                                                <p class="segment-chapter-note">
                                                    {{
                                                        entry.note ||
                                                        'Chưa có segment nào sẵn sàng hiển thị.'
                                                    }}
                                                </p>
                                            </div>
                                            <span
                                                class="segment-status-badge"
                                                :class="`is-${entry.group.status === 'completed' ? 'played' : entry.group.status}`"
                                            >
                                                {{
                                                    entry.group.status ===
                                                    "reading"
                                                        ? "Đang đọc"
                                                        : entry.group.status ===
                                                            "rendering"
                                                          ? "Đang tạo tiếp"
                                                          : entry.group.status ===
                                                              "completed"
                                                            ? "Đã xong"
                                                            : "Đã tách"
                                                }}
                                            </span>
                                        </div>

                                        <div class="segment-tiles">
                                            <article
                                                v-for="segment in entry.window.items"
                                                :key="`${entry.group.chapterId}-segment-${segment.index}`"
                                                class="segment-status-item"
                                                :class="[
                                                    `is-${segment.status}`,
                                                    {
                                                        'is-active':
                                                            isSegmentNowPlaying(
                                                                segment,
                                                                entry.group.chapterId,
                                                            ),
                                                        'is-now-playing':
                                                            isSegmentNowPlaying(
                                                                segment,
                                                                entry.group.chapterId,
                                                            ),
                                                    },
                                                ]"
                                                @click="
                                                    jumpToRealtimeSegment(
                                                        entry.group.chapterId,
                                                        segment.index,
                                                    )
                                                "
                                            >
                                                <div
                                                    class="segment-status-top"
                                                >
                                                    <strong
                                                        >Segment
                                                        {{
                                                            segment.index + 1
                                                        }}</strong
                                                    >
                                                    <span
                                                        v-if="
                                                            isSegmentNowPlaying(
                                                                segment,
                                                                entry.group.chapterId,
                                                            )
                                                        "
                                                        class="segment-now-pill"
                                                    >
                                                        Dang phat
                                                    </span>
                                                    <span
                                                        class="segment-status-badge"
                                                        :class="`is-${getDynamicSegmentStatus(segment.index, entry.group.chapterId)}`"
                                                    >
                                                        {{
                                                            realtimeSegmentStatusLabel(
                                                                getDynamicSegmentStatus(
                                                                    segment.index,
                                                                    entry.group.chapterId,
                                                                ),
                                                            )
                                                        }}
                                                    </span>
                                                </div>
                                                <p
                                                    class="segment-status-meta"
                                                >
                                                    {{ segment.wordCount }} từ
                                                    <span
                                                        v-if="
                                                            segment.attempt > 1
                                                        "
                                                    >
                                                        · lần
                                                        {{
                                                            segment.attempt
                                                        }}</span
                                                    >
                                                    <span
                                                        v-if="
                                                            entry.group.startSegmentIndex ===
                                                            segment.index
                                                        "
                                                    >
                                                        · điểm bắt đầu</span
                                                    >
                                                </p>
                                                <p class="segment-status-text">
                                                    {{ segment.text }}
                                                </p>
                                                <div
                                                    class="segment-progress-stack"
                                                >
                                                    <div
                                                        class="segment-progress-row"
                                                    >
                                                        <span>Tạo audio</span>
                                                        <strong>{{
                                                            getSegmentRenderLabel(
                                                                segment,
                                                            )
                                                        }}</strong>
                                                    </div>
                                                    <div
                                                        class="segment-mini-progress"
                                                    >
                                                        <div
                                                            class="segment-mini-progress-fill is-render"
                                                            :class="{
                                                                'is-live':
                                                                    segment.status ===
                                                                        'rendering' ||
                                                                    segment.status ===
                                                                        'retrying',
                                                            }"
                                                            :style="{
                                                                width:
                                                                    getSegmentRenderProgress(
                                                                        segment.status,
                                                                    ) + '%',
                                                            }"
                                                        ></div>
                                                    </div>
                                                    <div
                                                        class="segment-progress-row"
                                                    >
                                                        <span>Đang đọc</span>
                                                        <strong
                                                            >{{
                                                                Math.round(
                                                                    getSegmentPlaybackProgress(
                                                                        segment,
                                                                        entry.group.chapterId,
                                                                    ),
                                                                )
                                                            }}%</strong
                                                        >
                                                    </div>
                                                    <div
                                                        class="segment-mini-progress"
                                                    >
                                                        <div
                                                            class="segment-mini-progress-fill is-play"
                                                            :class="{
                                                                'is-live':
                                                                    getDynamicSegmentStatus(
                                                                        segment.index,
                                                                        entry.group.chapterId,
                                                                    ) ===
                                                                    'reading',
                                                            }"
                                                            :style="{
                                                                width:
                                                                    getSegmentPlaybackProgress(
                                                                        segment,
                                                                        entry.group.chapterId,
                                                                    ) + '%',
                                                            }"
                                                        ></div>
                                                    </div>
                                                </div>
                                                <p
                                                    v-if="segment.message"
                                                    class="segment-status-message"
                                                >
                                                    {{ segment.message }}
                                                </p>
                                            </article>
                                        </div>
                                    </section>
                                </div>
                            </section>
                            </div>
                        </aside>

                        <section
                            v-if="showReaderText"
                            class="reader-text-pane"
                            :class="{
                                'edge-read-aloud-text-pane': useEdgeReadAloud,
                            }"
                        >
                            <header
                                class="reader-head"
                                :class="{
                                    'edge-read-aloud-hidden': useEdgeReadAloud,
                                }"
                            >
                                <div>
                                    <p class="eyebrow">Đang đọc</p>
                                    <h2>{{ chapterContent.chapterTitle }}</h2>
                                    <p class="meta">
                                        {{ chapterContent.storyTitle }} · chương
                                        {{ selectedChapterPosition }} ·
                                        {{ chapterContent.characterCount }} ký
                                        tự · {{ chapterWordCount }} từ
                                    </p>
                                </div>

                                <div class="reader-actions">
                                    <button
                                        class="ghost-button reader-play-button"
                                        :class="{
                                            'is-playing':
                                                readerPlayButtonIsPlaying,
                                        }"
                                        @click="handleReaderPlayAction"
                                    >
                                        {{ readerPlayButtonLabel }}
                                    </button>
                                    <div class="reader-font-controls">
                                        <span>Cỡ chữ</span>
                                        <button
                                            class="ghost-button font-size-button"
                                            @click="adjustReaderFontSize(-1)"
                                            :disabled="readerFontSize <= 14"
                                        >
                                            A-
                                        </button>
                                        <input
                                            v-model.number="readerFontSize"
                                            class="font-size-slider"
                                            type="range"
                                            min="14"
                                            max="28"
                                            step="1"
                                        />
                                        <button
                                            class="ghost-button font-size-button"
                                            @click="adjustReaderFontSize(1)"
                                            :disabled="readerFontSize >= 28"
                                        >
                                            A+
                                        </button>
                                        <strong>{{ readerFontSize }}px</strong>
                                    </div>
                                    <button
                                        class="ghost-button"
                                        @click="startRealtimeFromSelection"
                                        :disabled="
                                            !selectedRealtimeChapterGroup
                                        "
                                        :title="
                                            selectedRealtimeChapterGroup
                                                ? 'Đọc từ đoạn văn đã bôi chọn'
                                                : 'Hãy bắt đầu Đọc realtime trước khi dùng tính năng này'
                                        "
                                    >
                                        Đọc từ bôi chọn
                                        <span
                                            v-if="!selectedRealtimeChapterGroup"
                                            class="eyebrow"
                                            style="
                                                font-size: 0.7rem;
                                                opacity: 0.6;
                                            "
                                        >
                                            (cần đọc realtime trước)
                                        </span>
                                    </button>
                                    <button
                                        class="ghost-button"
                                        @click="goToSiblingChapter(-1)"
                                        :disabled="!chapterAt(-1)"
                                    >
                                        Chương trước
                                    </button>
                                    <button
                                        class="ghost-button"
                                        @click="goToSiblingChapter(1)"
                                        :disabled="!chapterAt(1)"
                                    >
                                        Chương sau
                                    </button>
                                </div>
                            </header>

                            <section
                                ref="readerBody"
                                class="reader-body"
                                :style="{
                                    '--reader-font-size': `${readerFontSize}px`,
                                }"
                                tabindex="0"
                                @scroll="scheduleProgressSave"
                            >
                                <div
                                    ref="readerScanContent"
                                    class="reader-scan-content"
                                >
                                    <template
                                        v-for="(
                                            block, index
                                        ) in renderableChapterBlocks"
                                        :key="`${chapterContent.chapterId}-${index}`"
                                    >
                                        <h3
                                            v-if="block.kind === 'heading'"
                                            class="reader-heading"
                                        >
                                            <template
                                                v-for="token in block.tokens"
                                                :key="token.key"
                                            >
                                                <span
                                                    v-if="token.isWord"
                                                    class="reader-word"
                                                    :class="{
                                                        'is-active-word':
                                                            token.wordIndex ===
                                                            activeWordGlobalIndex,
                                                        'is-active-segment':
                                                            isWordInActiveSegment(
                                                                token.wordIndex,
                                                            ),
                                                    }"
                                                    :data-word-index="
                                                        token.wordIndex ??
                                                        undefined
                                                    "
                                                    >{{ token.text }}</span
                                                >
                                                <span v-else>{{
                                                    token.text
                                                }}</span>
                                            </template>
                                        </h3>
                                        <div
                                            v-else-if="block.kind === 'divider'"
                                            class="reader-divider"
                                        >
                                            {{ block.text }}
                                        </div>
                                        <div
                                            v-else-if="block.kind === 'spacer'"
                                            class="reader-spacer"
                                            aria-hidden="true"
                                        ></div>
                                        <p v-else class="reader-paragraph">
                                            <template
                                                v-for="token in block.tokens"
                                                :key="token.key"
                                            >
                                                <span
                                                    v-if="token.isWord"
                                                    class="reader-word"
                                                    :class="{
                                                        'is-active-word':
                                                            token.wordIndex ===
                                                            activeWordGlobalIndex,
                                                        'is-active-segment':
                                                            isWordInActiveSegment(
                                                                token.wordIndex,
                                                            ),
                                                    }"
                                                    :data-word-index="
                                                        token.wordIndex ??
                                                        undefined
                                                    "
                                                    >{{ token.text }}</span
                                                >
                                                <span v-else>{{
                                                    token.text
                                                }}</span>
                                            </template>
                                        </p>
                                    </template>
                                </div>
                            </section>

                            <footer
                                v-if="isEdgeBrowser"
                                class="edge-read-aloud-dock"
                                :class="{ active: edgeReadAloudActive }"
                            >
                                <div class="edge-dock-meta">
                                    <label class="edge-read-aloud-toggle">
                                        <input
                                            v-model="useEdgeReadAloud"
                                            type="checkbox"
                                        />
                                        <span>Dùng Read Aloud của Edge</span>
                                    </label>
                                    <label class="edge-wpm-control">
                                        <span>WPM</span>
                                        <input
                                            v-model.number="
                                                edgeReadAloudWordsPerMinute
                                            "
                                            type="number"
                                            min="120"
                                            max="260"
                                            step="5"
                                        />
                                    </label>
                                    <div
                                        v-if="useEdgeReadAloud"
                                        class="reader-font-controls reader-font-controls--compact"
                                    >
                                        <span>Cỡ chữ</span>
                                        <button
                                            class="ghost-button font-size-button"
                                            @click="adjustReaderFontSize(-1)"
                                            :disabled="readerFontSize <= 14"
                                        >
                                            A-
                                        </button>
                                        <input
                                            v-model.number="readerFontSize"
                                            class="font-size-slider"
                                            type="range"
                                            min="14"
                                            max="28"
                                            step="1"
                                        />
                                        <button
                                            class="ghost-button font-size-button"
                                            @click="adjustReaderFontSize(1)"
                                            :disabled="readerFontSize >= 28"
                                        >
                                            A+
                                        </button>
                                        <strong>{{ readerFontSize }}px</strong>
                                    </div>
                                    <button
                                        v-if="useEdgeReadAloud"
                                        class="ghost-button edge-dock-scan"
                                        @click="
                                            rescanReaderTextForEdgeReadAloud
                                        "
                                        :disabled="!chapterContent"
                                    >
                                        {{
                                            edgeReadAloudActive
                                                ? "Quét lại khối chữ"
                                                : "Quét khối chữ"
                                        }}
                                    </button>
                                </div>

                                <div class="edge-dock-actions">
                                    <button
                                        class="ghost-button edge-dock-button"
                                        @click="activeTab = 'library'"
                                    >
                                        Về thư viện
                                    </button>
                                    <button
                                        class="ghost-button edge-dock-button"
                                        @click="goToSiblingChapter(-1)"
                                        :disabled="!chapterAt(-1)"
                                    >
                                        Trước
                                    </button>
                                    <button
                                        class="ghost-button edge-dock-button edge-dock-play"
                                        @click="togglePlayback"
                                        :disabled="!useEdgeReadAloud"
                                    >
                                        {{
                                            edgeReadAloudActive
                                                ? "Pause Edge"
                                                : "Play Edge"
                                        }}
                                    </button>
                                    <button
                                        class="ghost-button edge-dock-button"
                                        @click="goToSiblingChapter(1)"
                                        :disabled="!chapterAt(1)"
                                    >
                                        Sau
                                    </button>
                                </div>
                            </footer>
                        </section>
                    </div>
                    <audio
                        ref="audioRef"
                        class="audio-player-hidden"
                        @timeupdate="handleTimeUpdate"
                        @pause="handleAudioPause"
                        @play="handleAudioPlay"
                        @ended="handleAudioEnded"
                    />
                </article>

                <article v-else class="empty-state panel">
                    <p class="eyebrow">Bắt đầu</p>
                    <h2>Chọn thư mục gốc để tạo thư viện truyện local</h2>
                    <p class="lead compact">
                        Cấu trúc v1: một thư mục gốc, mỗi thư mục con là một
                        truyện, mỗi file <code>.txt</code> bên trong là một
                        chương.
                    </p>
                </article>
            </section>
        </section>
    </main>
</template>

<style scoped>
.hidden-input {
    display: none;
}

.error-banner {
    margin: 0;
    padding: 0.9rem 1.2rem;
    border-radius: var(--radius-md);
    background: rgba(239, 68, 68, 0.1);
    color: #b91c1c;
    border: 1px solid rgba(239, 68, 68, 0.18);
}

.eyebrow {
    margin: 0 0 0.35rem;
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--accent);
    font-weight: 700;
}

.app-shell {
    max-width: 1600px;
    margin: 0 auto;
    padding: 1rem 1.25rem 1.25rem;
    height: 100vh;
    display: flex;
    flex-direction: column;
    gap: 1rem;
    overflow: hidden;
}

.app-shell.edge-read-aloud-shell {
    padding-top: 0.5rem;
}

.app-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.9rem 1.25rem;
    background: var(--bg-panel);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-sm);
    border: 1px solid var(--border);
    flex-shrink: 0;
}

.edge-read-aloud-hidden {
    display: none !important;
}

.branding {
    display: flex;
    align-items: center;
    gap: 0.8rem;
}

.logo-icon {
    font-size: 1.6rem;
    line-height: 1;
}

.app-title {
    font-size: 1.05rem;
    font-weight: 700;
    color: var(--text-primary);
    margin: 0;
}

.app-subtitle {
    font-size: 0.82rem;
    color: var(--text-secondary);
    margin: 0;
}

.header-actions {
    display: flex;
    align-items: center;
    gap: 1rem;
}

button {
    background: var(--bg-active);
    color: var(--text-active);
    border: none;
    padding: 0.6rem 1.2rem;
    border-radius: var(--radius-sm);
    font-weight: 500;
    font-size: 0.95rem;
    cursor: pointer;
    transition: all 0.2s ease;
    box-shadow: var(--shadow-sm);
}

button:hover:not(:disabled) {
    background: var(--bg-active-hover);
    transform: translateY(-1px);
    box-shadow: var(--shadow-md);
}

button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
    transform: none;
    box-shadow: none;
}

.ghost-button {
    background: var(--bg-panel);
    color: var(--text-primary);
    border: 1px solid var(--border);
    box-shadow: var(--shadow-sm);
}

.ghost-button:hover:not(:disabled) {
    background: var(--bg-panel-hover);
    color: var(--accent);
    border-color: var(--accent);
}

.app-tabs {
    display: flex;
    flex-wrap: wrap;
    gap: 0.45rem;
    padding: 0 0.25rem 0.35rem;
    border-bottom: 1px solid var(--border);
    margin-bottom: 0.35rem;
}

.app-tabs button {
    background: rgba(59, 130, 246, 0.06);
    color: var(--text-secondary);
    border: 1px solid transparent;
    font-size: 0.8rem;
    font-weight: 600;
    padding: 0.42rem 0.72rem;
    box-shadow: none;
    border-radius: 999px;
    cursor: pointer;
    min-height: 0;
}

.app-tabs button:hover:not(:disabled) {
    color: var(--text-primary);
    background: transparent;
    box-shadow: none;
    transform: none;
}

.app-tabs button.active {
    color: var(--accent);
    border-color: rgba(59, 130, 246, 0.25);
    background: rgba(59, 130, 246, 0.12);
}

.workspace {
    gap: 1.5rem;
    flex: 1;
    min-height: 0;
    overflow: hidden;
    padding: 0 0.5rem 1rem;
}

.library-workspace {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(320px, 420px);
    align-items: stretch;
}

.library-main {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

.library-summary {
    flex: 0 0 auto;
}

.story-gallery {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    gap: 1rem;
    align-content: start;
    padding-right: 0.25rem;
}

.reader-workspace {
    display: flex;
    justify-content: center;
    min-height: 0;
    overflow: hidden;
}

.reader-shell {
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
    width: 100%;
    max-width: 1580px;
}

.reader-pane-tabs {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.8rem 1rem 0;
    flex-shrink: 0;
}

.reader-pane-tabs button {
    background: rgba(15, 23, 42, 0.08);
    color: var(--text-secondary);
    border: 1px solid rgba(148, 163, 184, 0.16);
    box-shadow: none;
    padding: 0.55rem 0.95rem;
    font-size: 0.8rem;
    font-weight: 700;
    border-radius: 999px;
}

.reader-pane-tabs button:hover:not(:disabled) {
    background: rgba(59, 130, 246, 0.08);
    border-color: rgba(59, 130, 246, 0.26);
    color: var(--accent);
    transform: none;
}

.reader-pane-tabs button.active {
    background: rgba(59, 130, 246, 0.14);
    border-color: rgba(59, 130, 246, 0.34);
    color: var(--accent);
}

.panel {
    background: var(--bg-panel);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-md);
    display: flex;
    flex-direction: column;
    overflow: hidden;
}

.library-main,
.chapter-column {
    min-height: 0;
}

.sidebar-head,
.column-head {
    padding: 1.5rem;
    border-bottom: 1px solid var(--border);
    background: var(--bg-panel-hover);
}

.sidebar-head h2,
.column-head h2 {
    font-size: 1.25rem;
    font-weight: 700;
    color: var(--text-primary);
}

.library-root {
    padding: 1rem 1.5rem 0;
}

.meta {
    font-size: 0.85rem;
    color: var(--text-secondary);
}

.chapter-list {
    flex: 1;
    overflow-y: auto;
    padding: 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
}

.story-card,
.chapter-card {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    background: var(--bg-panel);
    border: 1px solid var(--border);
    padding: 1rem;
    border-radius: var(--radius-md);
    text-align: left;
    color: var(--text-primary);
}

.compact-story-card {
    gap: 0.7rem;
    padding: 0.95rem 1rem;
    cursor: pointer;
    min-height: 170px;
    justify-content: space-between;
}

.gallery-story-card {
    overflow: hidden;
    transition:
        transform 0.18s ease,
        border-color 0.18s ease,
        box-shadow 0.18s ease;
}

.gallery-story-card:hover {
    transform: translateY(-2px);
    border-color: rgba(59, 130, 246, 0.28);
}

.story-card-top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.8rem;
}

.story-chapter-total {
    white-space: nowrap;
    font-size: 0.78rem;
    padding: 0.24rem 0.55rem;
    border-radius: 999px;
    background: rgba(59, 130, 246, 0.08);
    color: var(--accent);
    font-weight: 700;
}

.story-progress-line {
    margin: 0;
    font-size: 0.88rem;
    color: var(--text-primary);
    font-weight: 600;
}

.story-card-actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.8rem;
}

.continue-button {
    padding: 0.5rem 0.8rem;
    font-size: 0.82rem;
    border-radius: 999px;
}

.story-card.active,
.chapter-card.active {
    background: var(--bg-active);
    color: var(--text-active);
    border-color: var(--bg-active);
}

.chapter-card.chapter-read:not(.active) {
    background: rgba(96, 165, 250, 0.08);
    border-left: 3px solid rgba(96, 165, 250, 0.4);
}

.chapter-card.chapter-read:not(.active) strong {
    color: rgba(96, 165, 250, 0.85);
}

.story-card.active *,
.chapter-card.active * {
    color: var(--text-active) !important;
}

.story-path,
.story-time {
    font-size: 0.8rem;
    color: var(--text-secondary);
}

.reader-column {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    background: var(--bg-panel);
}

.reader-layout {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: minmax(430px, 560px) minmax(0, 1fr);
}

.reader-layout.is-console-only,
.reader-layout.is-text-only,
.reader-layout.edge-read-aloud-layout {
    grid-template-columns: minmax(0, 1fr);
}

.reader-text-pane {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    border-left: 1px solid var(--border);
}

.reader-text-pane.edge-read-aloud-text-pane {
    border-left: none;
}

.edge-read-aloud-mode .reader-body {
    padding-top: 1rem;
}

.edge-read-aloud-dock {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.85rem;
    padding: 0.9rem 1.1rem;
    border-top: 1px solid var(--border);
    background: rgba(15, 23, 42, 0.22);
    flex-wrap: wrap;
    flex-shrink: 0;
}

.edge-read-aloud-dock.active {
    position: sticky;
    bottom: 0;
    z-index: 5;
    backdrop-filter: blur(8px);
}

.edge-dock-meta {
    display: flex;
    align-items: center;
    gap: 0.9rem;
    flex-wrap: wrap;
}

.edge-dock-scan {
    min-width: 9rem;
}

.edge-dock-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
}

.edge-dock-button {
    min-width: 6.75rem;
}

.edge-dock-play {
    min-width: 7.5rem;
}

.reader-head {
    padding: 1.1rem 1.35rem;
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1rem;
    border-bottom: 1px solid var(--border);
    background: var(--bg-panel-hover);
    flex-shrink: 0;
}

.reader-head h2 {
    margin: 0 0 0.3rem;
    font-size: 1.35rem;
}

.reader-actions {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.5rem;
}

.reader-font-controls {
    display: inline-flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.55rem;
    padding: 0.55rem 0.8rem;
    border: 1px solid rgba(148, 163, 184, 0.2);
    border-radius: 999px;
    background: rgba(15, 23, 42, 0.5);
}

.reader-font-controls span,
.reader-font-controls strong {
    font-size: 0.82rem;
    color: var(--text-secondary);
}

.reader-play-button {
    min-width: 8rem;
}

.reader-play-button.is-playing {
    color: #eff6ff;
    border-color: rgba(96, 165, 250, 0.5);
    background: rgba(59, 130, 246, 0.18);
}

.reader-font-controls strong {
    color: var(--text-primary);
}

.reader-font-controls--compact {
    padding: 0.45rem 0.7rem;
    background: rgba(15, 23, 42, 0.36);
}

.reader-font-controls--compact .font-size-slider {
    width: 5.75rem;
}

.font-size-button {
    min-width: 3rem;
    padding-inline: 0.7rem;
}

.font-size-slider {
    width: 7rem;
}

.premium-player-column {
    padding: 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    min-height: 0;
    overflow: hidden;
    background:
        radial-gradient(
            circle at top left,
            rgba(59, 130, 246, 0.08),
            transparent 32%
        ),
        linear-gradient(
            180deg,
            rgba(15, 23, 42, 0.05) 0%,
            rgba(15, 23, 42, 0.01) 100%
        );
}

.reader-layout.is-console-only .premium-player-column {
    overflow-y: auto;
    align-items: stretch;
}

.console-media-strip {
    position: sticky;
    top: 0;
    z-index: 3;
    display: grid;
    grid-template-columns: minmax(0, 1.2fr) auto minmax(220px, 320px);
    gap: 0.9rem;
    align-items: center;
    padding: 0.9rem 1rem;
    border-radius: 16px;
    background: linear-gradient(
        135deg,
        rgba(15, 23, 42, 0.82),
        rgba(30, 41, 59, 0.72)
    );
    border: 1px solid rgba(96, 165, 250, 0.18);
    box-shadow: 0 14px 30px rgba(15, 23, 42, 0.18);
    backdrop-filter: blur(12px);
}

.console-media-strip__eyebrow {
    margin: 0 0 0.2rem;
    font-size: 0.7rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: rgba(191, 219, 254, 0.92);
    font-weight: 700;
}

.console-media-strip__meta h4 {
    margin: 0;
    font-size: 1rem;
    color: #f8fafc;
}

.console-media-strip__text {
    margin: 0.28rem 0 0;
    font-size: 0.8rem;
    line-height: 1.55;
    color: rgba(226, 232, 240, 0.88);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
}

.console-media-strip__controls {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    flex-wrap: wrap;
    justify-content: flex-end;
}

.console-play-btn {
    width: 42px;
    height: 42px;
}

.console-media-strip__progress {
    display: flex;
    flex-direction: column;
    gap: 0.28rem;
}

.progress-bar-container--hero {
    height: 10px;
    background: rgba(255, 255, 255, 0.14);
}

.progress-filled--hero {
    background: linear-gradient(90deg, #38bdf8, #2563eb);
}

.time-labels--hero {
    margin-top: 0;
    color: rgba(226, 232, 240, 0.82);
    font-size: 0.68rem;
}

.console-split-layout {
    display: grid;
    grid-template-columns: minmax(280px, 355px) minmax(0, 1fr);
    gap: 0.9rem;
    align-items: start;
}

.player-console-head {
    padding: 0.15rem 0.15rem 0;
}

.player-console-head h3 {
    margin: 0.18rem 0 0.15rem;
    font-size: 1.18rem;
    line-height: 1.25;
    color: #0f172a;
}

.player-console-head p:last-child {
    margin: 0;
    font-size: 0.82rem;
    color: #64748b;
}

.premium-player-card {
    width: 100%;
    max-width: 355px;
    margin: 0;
    background: #e9edf4;
    border-radius: 18px;
    padding: 0.75rem 0.8rem 0.85rem;
    box-shadow:
        8px 8px 18px rgba(148, 163, 184, 0.35),
        -8px -8px 18px rgba(255, 255, 255, 0.75);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.65rem;
    position: relative;
    transition: transform 0.25s ease;
}

.premium-player-card:hover {
    transform: translateY(-2px);
}

.card-head {
    width: 100%;
    display: grid;
    grid-template-columns: 22px 1fr 22px;
    align-items: center;
    gap: 0.25rem;
}

.card-head-title {
    margin: 0;
    text-align: center;
    font-size: 0.5rem;
    letter-spacing: 0.08em;
    font-weight: 700;
    color: #334155;
}

.card-head-icon {
    width: 22px;
    height: 22px;
    border: none;
    border-radius: 999px;
    background: transparent;
    color: #475569;
    font-size: 0.9rem;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    line-height: 1;
    box-shadow: none;
    padding: 0;
}

.card-head-icon:hover:not(:disabled) {
    transform: none;
    background: rgba(148, 163, 184, 0.18);
    box-shadow: none;
}

.card-head-icon:disabled {
    opacity: 0.45;
}

.card-head-actions {
    position: relative;
    justify-self: end;
}

.player-artwork {
    width: 100%;
    max-width: 124px;
    aspect-ratio: 1 / 1;
    border-radius: 10px;
    background:
        radial-gradient(
            circle at 65% 38%,
            rgba(59, 130, 246, 0.45),
            transparent 46%
        ),
        linear-gradient(145deg, #0b1020 0%, #1f2a44 55%, #6b7280 100%);
    display: flex;
    align-items: center;
    justify-content: center;
    position: relative;
    overflow: hidden;
    box-shadow: inset 0 1px 1px rgba(255, 255, 255, 0.08);
}

.artwork-inner {
    font-size: 1.55rem;
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 2;
    filter: drop-shadow(0 2px 5px rgba(15, 23, 42, 0.4));
}

.artwork-glow {
    position: absolute;
    inset: 0;
    background: repeating-radial-gradient(
        circle at 50% 46%,
        rgba(148, 163, 184, 0.1) 0 2px,
        transparent 2px 6px
    );
    opacity: 0.35;
    z-index: 1;
    pointer-events: none;
}

.player-info-block {
    text-align: center;
    min-width: 0;
    width: 100%;
    margin-top: 0.1rem;
}

.playing-title {
    font-size: 0.9rem;
    color: #475569;
    margin: 0 0 0.12rem;
    font-weight: 700;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}

.playing-subtitle {
    font-size: 0.62rem;
    color: #94a3b8;
    margin: 0;
    font-weight: 600;
}

.player-status-row {
    margin-top: 0.45rem;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
    flex-wrap: wrap;
}

.player-status-pill,
.player-voice-label,
.segment-status-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0.18rem 0.48rem;
    border-radius: 999px;
    font-size: 0.58rem;
    font-weight: 700;
}

.player-status-pill {
    background: rgba(148, 163, 184, 0.18);
    color: #475569;
}

.player-status-pill.is-reading,
.segment-status-badge.is-reading {
    background: rgba(37, 99, 235, 0.12);
    color: #1d4ed8;
}

.player-status-pill.is-buffering,
.player-status-pill.is-connecting,
.segment-status-badge.is-rendering {
    background: rgba(14, 165, 233, 0.12);
    color: #0369a1;
}

.player-status-pill.is-transitioning,
.segment-status-badge.is-retrying {
    background: rgba(245, 158, 11, 0.14);
    color: #b45309;
}

.player-status-pill.is-finished,
.segment-status-badge.is-played {
    background: rgba(34, 197, 94, 0.14);
    color: #15803d;
}

.player-status-pill.is-error {
    background: rgba(239, 68, 68, 0.12);
    color: #b91c1c;
}

.player-status-pill.is-stopped,
.segment-status-badge.is-ready,
.segment-status-badge.is-queued {
    background: rgba(100, 116, 139, 0.12);
    color: #475569;
}

.player-voice-label {
    max-width: 100%;
    color: #64748b;
    background: rgba(255, 255, 255, 0.55);
}

.player-status-text {
    margin: 0.42rem 0 0;
    font-size: 0.58rem;
    line-height: 1.45;
    color: #64748b;
}

.segment-status-card {
    width: 100%;
    background: rgba(255, 255, 255, 0.52);
    border: 1px solid rgba(148, 163, 184, 0.2);
    border-radius: 14px;
    padding: 0.55rem;
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
}

.segment-status-card--main {
    flex: 1;
    min-height: 0;
}

.reader-layout.is-console-only .segment-status-card--main {
    flex: 0 0 auto;
    min-height: 22rem;
}

.segment-status-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.5rem;
}

.segment-status-kicker {
    margin: 0 0 0.14rem;
    font-size: 0.54rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: #94a3b8;
    font-weight: 700;
}

.segment-status-head strong {
    font-size: 0.7rem;
    color: #334155;
}

.segment-window-note {
    margin: 0.18rem 0 0;
    font-size: 0.55rem;
    line-height: 1.5;
    color: #64748b;
}

.segment-status-metrics {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 0.14rem;
    font-size: 0.56rem;
    color: #64748b;
    text-align: right;
}

.segment-status-list {
    display: flex;
    flex-direction: column;
    gap: 0.55rem;
    max-height: none;
    overflow-y: auto;
    padding-right: 0.15rem;
    min-height: 0;
}

.reader-layout.is-console-only .segment-status-list {
    overflow: visible;
    min-height: auto;
}

.segment-tiles {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 0.55rem;
}

.reader-layout.is-console-only .segment-tiles {
    grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
}

.segment-chapter-group {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
}

.segment-chapter-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.4rem;
}

.segment-chapter-head strong {
    font-size: 0.66rem;
    color: #334155;
}

.segment-chapter-head p {
    margin: 0.12rem 0 0;
    font-size: 0.56rem;
    color: #64748b;
}

.segment-status-item {
    border-radius: 12px;
    border: 1px solid rgba(148, 163, 184, 0.18);
    background: rgba(248, 250, 252, 0.72);
    padding: 0.5rem 0.58rem;
    display: flex;
    flex-direction: column;
    gap: 0.22rem;
    cursor: pointer;
}

.segment-status-item.is-active {
    border-color: rgba(37, 99, 235, 0.28);
    box-shadow: inset 0 0 0 1px rgba(37, 99, 235, 0.12);
}

.segment-status-item.is-now-playing {
    border-color: rgba(37, 99, 235, 0.58);
    box-shadow:
        inset 0 0 0 1px rgba(37, 99, 235, 0.24),
        0 12px 24px rgba(37, 99, 235, 0.14);
    background: linear-gradient(
        180deg,
        rgba(219, 234, 254, 0.95),
        rgba(239, 246, 255, 0.88)
    );
}

.segment-status-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.4rem;
}

.segment-status-top strong {
    font-size: 0.64rem;
    color: #334155;
}

.segment-now-pill {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0.18rem 0.5rem;
    border-radius: 999px;
    background: linear-gradient(90deg, #2563eb, #38bdf8);
    color: #eff6ff;
    font-size: 0.54rem;
    font-weight: 800;
    letter-spacing: 0.04em;
    text-transform: uppercase;
}

.segment-status-meta,
.segment-status-message {
    margin: 0;
    font-size: 0.55rem;
    color: #64748b;
}

.segment-status-message {
    color: #b45309;
}

.segment-status-text {
    margin: 0;
    font-size: 0.61rem;
    line-height: 1.5;
    color: #475569;
    display: -webkit-box;
    -webkit-line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
}

.segment-progress-stack {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    margin-top: 0.1rem;
}

.segment-progress-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.4rem;
    font-size: 0.52rem;
    color: #64748b;
}

.segment-progress-row strong {
    font-size: 0.52rem;
    color: #334155;
}

.segment-mini-progress {
    position: relative;
    width: 100%;
    height: 5px;
    border-radius: 999px;
    overflow: hidden;
    background: rgba(148, 163, 184, 0.16);
}

.segment-mini-progress-fill {
    height: 100%;
    width: 0;
    border-radius: inherit;
    transition:
        width 0.18s linear,
        opacity 0.18s ease;
}

.segment-mini-progress-fill.is-render {
    background: linear-gradient(90deg, #38bdf8, #0ea5e9);
}

.segment-mini-progress-fill.is-play {
    background: linear-gradient(90deg, #2563eb, #1d4ed8);
}

.segment-mini-progress-fill.is-live {
    animation: segmentPulse 1.15s ease-in-out infinite;
}

@keyframes segmentPulse {
    0%,
    100% {
        opacity: 0.65;
    }
    50% {
        opacity: 1;
    }
}

.player-main-controls {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    justify-content: center;
    flex-wrap: nowrap;
    margin-top: 0.2rem;
}

.edge-read-aloud-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    margin-top: 0.85rem;
    font-size: 0.72rem;
    color: #64748b;
}

.edge-read-aloud-toggle,
.edge-wpm-control {
    display: flex;
    align-items: center;
    gap: 0.45rem;
}

.edge-read-aloud-toggle input,
.edge-wpm-control input {
    accent-color: #2563eb;
}

.edge-wpm-control input {
    width: 4.25rem;
    border: 1px solid #cbd5e1;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.78);
    padding: 0.22rem 0.5rem;
    color: #0f172a;
}

.neume-btn {
    border: none !important;
    background: #edf1f7;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: all 0.2s ease;
    box-shadow:
        5px 5px 10px rgba(148, 163, 184, 0.35),
        -5px -5px 10px rgba(255, 255, 255, 0.9) !important;
    color: #64748b;
    padding: 0;
}

.neume-btn:hover:not(:disabled) {
    transform: translateY(-1px);
    box-shadow:
        7px 7px 12px rgba(148, 163, 184, 0.32),
        -6px -6px 12px rgba(255, 255, 255, 0.9) !important;
    background: #f2f5fa;
}

.neume-btn:active {
    box-shadow:
        inset 4px 4px 8px rgba(148, 163, 184, 0.34),
        inset -4px -4px 8px rgba(255, 255, 255, 0.9) !important;
    transform: scale(0.98);
}

.prev-btn,
.next-btn {
    width: 30px;
    height: 30px;
    border-radius: 50%;
    font-size: 0.7rem;
}

.play-pause-btn {
    width: 38px;
    height: 38px;
    border-radius: 50%;
    background: #e7edf5;
    color: #2563eb;
}

.heart-btn {
    width: 30px;
    height: 30px;
    border-radius: 50%;
}

.heart-glyph {
    font-size: 0.74rem;
    color: #f43f5e;
    line-height: 1;
    text-shadow: 0 1px 0 rgba(255, 255, 255, 0.75);
}

.skip-glyph {
    position: relative;
    display: block;
    width: 12px;
    height: 10px;
    border-left: 1.6px solid #9aa7ba;
}

.skip-glyph::before,
.skip-glyph::after {
    content: "";
    position: absolute;
    top: 1px;
    width: 0;
    height: 0;
    border-top: 4px solid transparent;
    border-bottom: 4px solid transparent;
    border-right: 4px solid #9aa7ba;
}

.skip-glyph::before {
    left: 1px;
}

.skip-glyph::after {
    left: 5px;
}

.skip-glyph.right {
    transform: scaleX(-1);
}

.center-glyph {
    position: relative;
    display: block;
    width: 0;
    height: 0;
    border-top: 6px solid transparent;
    border-bottom: 6px solid transparent;
    border-left: 10px solid #0284c7;
    margin-left: 2px;
}

.center-glyph.is-playing {
    width: 14px;
    height: 14px;
    margin-left: 0;
    border: none;
    border-radius: 3px;
    background: #0284c7;
    box-shadow: inset 0 0 0 1px #0369a1;
}

.center-glyph.is-playing::before,
.center-glyph.is-playing::after {
    content: "";
    position: absolute;
    top: 3px;
    width: 2px;
    height: 8px;
    background: #ffffff;
    border-radius: 1px;
}

.center-glyph.is-playing::before {
    left: 4px;
}

.center-glyph.is-playing::after {
    left: 8px;
}

.player-progress-section {
    width: min(100%, 260px);
    margin-top: 0.05rem;
}

.player-progress-caption {
    margin: 0 0 0.3rem;
    font-size: 0.56rem;
    font-weight: 700;
    letter-spacing: 0.03em;
    color: #64748b;
    text-align: center;
}

.progress-bar-container {
    height: 4px;
    background: #d5dde8;
    border-radius: 100px;
    position: relative;
    cursor: pointer;
    overflow: hidden;
}

.progress-bar-container--readonly {
    cursor: default;
}

.progress-filled {
    height: 100%;
    background: #7f8fa7;
    border-radius: 100px;
    width: 0%;
    transition: width 0.1s linear;
}

.progress-bar-container:hover .progress-filled {
    background: #64748b;
}

.progress-knob {
    display: none; /* Hide knob for a cleaner look like the image */
}

.time-labels {
    display: flex;
    justify-content: space-between;
    margin-top: 0.35rem;
    font-size: 0.52rem;
    color: #9aa7ba;
    font-weight: 700;
}

.setting-label {
    font-size: 0.56rem !important;
    text-transform: uppercase;
    color: #64748b;
    font-weight: 800;
}

.tune-trigger {
    font-size: 0.86rem;
    font-weight: 700;
}

.player-settings-hover {
    position: absolute;
    right: 0;
    top: calc(100% + 6px);
    width: 190px;
    padding: 0.5rem;
    border-radius: 12px;
    background: #f2f6fb;
    border: 1px solid rgba(148, 163, 184, 0.24);
    box-shadow: 0 14px 24px rgba(15, 23, 42, 0.2);
    opacity: 0;
    pointer-events: none;
    transform: translateY(6px) scale(0.98);
    transition:
        opacity 0.18s ease,
        transform 0.18s ease;
    z-index: 10;
}

.card-head-actions:hover .player-settings-hover,
.card-head-actions:focus-within .player-settings-hover {
    opacity: 1;
    pointer-events: auto;
    transform: translateY(0) scale(1);
}

.hover-voice-row {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    margin-bottom: 0.5rem;
}

.hover-voice-select {
    width: 100%;
    font-size: 0.68rem;
    border: 1px solid rgba(148, 163, 184, 0.3);
    border-radius: 8px;
    padding: 0.26rem 0.4rem;
    background: white;
    color: #0f172a;
}

.hover-sliders {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.45rem;
}

.hover-slider-col {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.25rem;
}

.vertical-slider-track {
    width: 28px;
    height: 74px;
    display: flex;
    align-items: center;
    justify-content: center;
}

.vertical-range {
    width: 74px;
    margin: 0;
    transform: rotate(-90deg);
    accent-color: #2563eb;
}

.hover-slider-col strong {
    font-size: 0.58rem;
    color: #334155;
    font-weight: 700;
}

.lyrics-hint {
    margin-top: 0.12rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.08rem;
    color: #94a3b8;
    font-size: 0.5rem;
    font-weight: 700;
    letter-spacing: 0.06em;
}

.lyrics-arrow {
    font-size: 0.48rem;
    line-height: 1;
}

.audio-player-hidden {
    display: none;
}

.reader-body {
    flex: 1;
    overflow-y: auto;
    padding: 1.75rem 2rem 2.4rem;
    font-family: var(--font-sans);
    font-size: var(--reader-font-size, 18px);
    font-weight: 450;
    letter-spacing: 0.01em;
    line-height: 2;
    scroll-behavior: smooth;
}

.reader-body > * {
    width: min(100%, 74ch);
    margin-inline: auto;
}

.reader-scan-content {
    width: 100%;
}

.reader-heading {
    margin: 0 0 1.1rem;
    color: var(--text-primary);
    font-family: var(--font-sans);
    font-size: calc(var(--reader-font-size, 18px) * 1.08);
    font-weight: 700;
    letter-spacing: -0.015em;
    white-space: pre-wrap;
}

.reader-word {
    border-radius: 0.45rem;
    transition:
        background-color 0.18s ease,
        color 0.18s ease,
        box-shadow 0.18s ease,
        text-shadow 0.18s ease;
}

.reader-word.is-active-word {
    background: transparent;
    color: #ffffff;
    box-shadow: none;
    text-shadow:
        0 0 2px rgba(255, 255, 255, 1),
        0 0 6px rgba(255, 255, 255, 0.98),
        0 0 12px rgba(103, 232, 249, 0.98),
        0 0 20px rgba(34, 211, 238, 0.9),
        0 0 32px rgba(14, 165, 233, 0.8),
        0 0 44px rgba(59, 130, 246, 0.55);
}

.reader-word.is-active-segment {
    background: transparent;
    color: #9fe9ff;
    box-shadow: none;
    text-shadow:
        0 0 1px rgba(255, 255, 255, 0.42),
        0 0 5px rgba(103, 232, 249, 0.35),
        0 0 10px rgba(34, 211, 238, 0.22);
}

.reader-word.is-active-segment.is-active-word {
    background: transparent;
    color: #ffffff;
    box-shadow: none;
    text-shadow:
        0 0 2px rgba(255, 255, 255, 1),
        0 0 8px rgba(255, 255, 255, 1),
        0 0 16px rgba(125, 211, 252, 1),
        0 0 26px rgba(34, 211, 238, 0.94),
        0 0 38px rgba(14, 165, 233, 0.86),
        0 0 52px rgba(59, 130, 246, 0.6);
}

.reader-paragraph {
    margin: 0 0 1.15rem;
    color: var(--text-primary);
    font-size: var(--reader-font-size, 18px);
    white-space: pre-wrap;
    text-align: left;
    text-indent: 0;
    line-height: 2;
    text-wrap: pretty;
}

.reader-divider {
    margin: 0 0 1rem;
    color: var(--text-muted);
    font-size: calc(var(--reader-font-size, 18px) * 0.84);
    white-space: pre-wrap;
    letter-spacing: 0.04em;
    opacity: 0.82;
}

.reader-spacer {
    height: 1.1rem;
}

.empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
    padding: 4rem;
}

.compact {
    max-width: 500px;
}

.preset-select {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin-top: 1rem;
}

.preset-select span {
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
}

.preset-select select {
    padding: 0.6rem 1rem;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    background: var(--bg-primary);
    outline: none;
    font-weight: 500;
    color: var(--text-primary);
}

.recent-strip {
    display: flex;
    gap: 0.5rem;
    padding: 1rem 1.5rem 0;
    flex-wrap: wrap;
}

.recent-chip {
    padding: 0.4rem 0.8rem;
    background: rgba(59, 130, 246, 0.1);
    color: var(--accent);
    border-radius: 99px;
    font-size: 0.85rem;
    font-weight: 600;
}

@media (max-width: 1200px) {
    .app-shell {
        height: auto;
        overflow: visible;
        min-height: 100vh;
    }

    .workspace {
        display: flex;
        flex-direction: column;
        overflow: visible;
    }

    .reader-shell {
        max-width: 100%;
    }

    .library-main,
    .chapter-column {
        height: auto;
        flex: none;
    }

    .story-gallery {
        overflow: visible;
        grid-template-columns: 1fr;
        padding-right: 0;
    }

    .reader-layout {
        grid-template-columns: 1fr;
    }

    .reader-pane-tabs {
        padding: 0.75rem 0.75rem 0;
        flex-wrap: wrap;
    }

    .premium-player-column {
        border-bottom: 1px solid var(--border);
    }

    .console-media-strip,
    .console-split-layout {
        grid-template-columns: 1fr;
    }

    .reader-text-pane {
        border-left: none;
    }

    .reader-body {
        padding: 1.25rem;
    }

    .app-header,
    .reader-head {
        flex-direction: column;
        align-items: stretch;
    }

    .reader-actions {
        width: 100%;
    }

    .reader-actions .ghost-button {
        flex: 1;
    }

    .player-main-controls {
        justify-content: flex-start;
        flex-wrap: wrap;
    }

    .edge-read-aloud-dock {
        align-items: stretch;
        flex-direction: column;
    }

    .edge-dock-actions {
        width: 100%;
    }

    .edge-dock-button {
        flex: 1;
    }
}

/* ===== Import Progress Overlay ===== */
.import-progress-overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 9999;
    backdrop-filter: blur(4px);
}

.import-progress-card {
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: 12px;
    padding: 24px;
    min-width: 360px;
    max-width: 500px;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
}

.progress-header {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 20px;
}

.progress-header h3 {
    margin: 0;
    font-size: 1.1rem;
    color: var(--color-text);
}

.spinner {
    width: 24px;
    height: 24px;
    border: 3px solid var(--color-border);
    border-top-color: var(--color-accent);
    border-radius: 50%;
    animation: spin 1s linear infinite;
}

.spinner-done {
    border-color: #22c55e;
    border-top-color: #22c55e;
    animation: none;
}

@keyframes spin {
    to {
        transform: rotate(360deg);
    }
}

.progress-phases {
    display: flex;
    gap: 8px;
    margin-bottom: 16px;
}

.phase-indicator {
    flex: 1;
    padding: 8px;
    text-align: center;
    font-size: 0.85rem;
    border-radius: 6px;
    background: var(--color-bg-secondary);
    color: var(--color-text-muted);
    transition: all 0.3s ease;
}

.phase-indicator.active {
    background: var(--color-accent);
    color: white;
    font-weight: 600;
}

.phase-indicator.done {
    background: #22c55e;
    color: white;
}

.progress-bar-container {
    width: 100%;
    height: 8px;
    background: var(--color-bg-secondary);
    border-radius: 4px;
    overflow: hidden;
    margin-bottom: 12px;
}

.progress-bar {
    height: 100%;
    background: linear-gradient(90deg, var(--color-accent), #60a5fa);
    transition: width 0.3s ease;
    border-radius: 4px;
}

.progress-message {
    margin: 0 0 16px 0;
    font-size: 0.9rem;
    color: var(--color-text);
    text-align: center;
}

.progress-error {
    text-align: center;
    color: #ef4444;
}

.btn-dismiss {
    background: var(--color-accent);
    color: white;
    border: none;
    padding: 8px 20px;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.9rem;
    transition: opacity 0.2s;
}

.btn-dismiss:hover {
    opacity: 0.8;
}

@media (max-width: 768px) {
    /* ===== Tablet & Mobile ===== */

    /* Viewport height fix for mobile browser chrome */
    .app-shell {
        height: 100dvh;
    }

    /* Library workspace: stack on narrow screens */
    .library-workspace {
        grid-template-columns: 1fr !important;
    }

    /* Reader layout: single column */
    .reader-layout {
        grid-template-columns: 1fr !important;
    }

    .reader-text-pane {
        border-left: none !important;
    }

    /* Reduce padding everywhere */
    .reader-body {
        padding: 1rem 1rem 1.5rem !important;
    }

    .reader-head {
        padding: 0.75rem 1rem !important;
    }

    .sidebar-head,
    .column-head {
        padding: 1rem !important;
    }

    .reader-actions {
        flex-wrap: wrap;
        gap: 0.5rem;
    }

    /* Player card responsive */
    .premium-player-card {
        max-width: 100%;
    }

    .player-artwork {
        max-width: 120px;
    }

    .player-settings-hover {
        width: 100%;
        max-width: 280px;
    }

    /* Touch-friendly tap targets (min 44x44px) */
    .reader-head .ghost-button,
    .reader-actions button,
    .prev-btn,
    .next-btn,
    .play-btn,
    .heart-btn {
        min-height: 44px;
        min-width: 44px;
    }

    /* Import progress card */
    .import-progress-card {
        min-width: auto;
        margin: 0 16px;
        padding: 16px;
    }

    .progress-phases {
        flex-direction: column;
    }

    .phase-indicator {
        font-size: 0.75rem;
        padding: 6px;
    }

    /* Toast container: full width on mobile */
    .toast-container {
        top: 8px;
        right: 8px;
        left: 8px;
        max-width: none;
    }
}

@media (max-width: 480px) {
    /* ===== Phone Portrait ===== */

    /* Further reduce padding */
    .reader-body {
        padding: 0.75rem 0.5rem 1rem !important;
    }

    .reader-head {
        padding: 0.5rem 0.75rem !important;
        flex-direction: column;
        gap: 0.5rem;
    }

    .reader-head .chapter-title {
        font-size: 0.95rem !important;
    }

    /* Reader text: full bleed */
    .reader-body > * {
        width: 100% !important;
    }

    /* Stack reader actions vertically */
    .reader-actions {
        flex-direction: column;
        width: 100%;
    }

    .reader-actions .ghost-button {
        width: 100%;
        justify-content: center;
    }

    /* Font controls: stack vertically */
    .reader-font-controls {
        flex-direction: column;
        align-items: stretch;
    }

    /* Player: compact mode */
    .premium-player-column {
        padding: 0.5rem !important;
    }

    .player-artwork {
        max-width: 80px;
    }

    .reader-pane-tabs {
        padding: 0.5rem 0.5rem 0;
    }

    /* Bottom sheet style for player on mobile */
    .player-main-controls {
        position: sticky;
        bottom: 0;
        background: var(--color-surface);
        border-top: 1px solid var(--color-border);
        padding: 0.75rem;
        z-index: 100;
    }

    /* Segment panel: scrollable with reduced padding */
    .segment-status-card {
        padding: 0.5rem !important;
    }

    /* Smaller buttons for compact UI */
    .card-head-icon {
        width: 36px;
        height: 36px;
    }

    /* App header: stack controls */
    .app-header {
        padding: 0.5rem 0.75rem !important;
    }

    .app-header-actions {
        flex-direction: column;
        gap: 0.5rem;
        width: 100%;
    }

    .app-header-actions button,
    .app-header-actions select {
        width: 100%;
    }

    /* App tabs: full width */
    .app-tabs {
        padding: 0 !important;
    }

    .app-tabs button {
        flex: 1;
        font-size: 0.85rem;
        padding: 0.75rem 0.5rem;
    }
}

@media (max-width: 360px) {
    /* ===== Small Phones ===== */

    .reader-body {
        padding: 0.5rem 0.25rem 0.75rem !important;
    }

    .reader-head {
        padding: 0.4rem 0.5rem !important;
    }

    /* Hide non-essential elements */
    .reader-font-controls {
        display: none;
    }

    /* Compact player */
    .player-artwork {
        display: none;
    }

    .player-main-controls {
        flex-wrap: wrap;
        justify-content: center;
    }
}
</style>
