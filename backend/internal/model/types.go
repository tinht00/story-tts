package model

import "time"

type SourceType string

const (
	SourceTypeLocal    SourceType = "local"
	SourceTypeTelegram SourceType = "telegram"
)

type ProsodyPreset string

const (
	PresetStable ProsodyPreset = "stable"
	PresetGentle ProsodyPreset = "gentle"
	PresetTense  ProsodyPreset = "tense"
	PresetClimax ProsodyPreset = "climax"
)

type JobType string

const (
	JobTypeScanLocal     JobType = "scan_local"
	JobTypeBuildStory    JobType = "build_story"
	JobTypeBuildChapter  JobType = "build_chapter"
	JobTypeMergeFullBook JobType = "merge_full_book"
	JobTypeTelegramFetch JobType = "telegram_fetch"
)

type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusRetryable JobStatus = "retryable"
	JobStatusCancelled JobStatus = "cancelled"
)

type SegmentStatus string

const (
	SegmentStatusQueued    SegmentStatus = "queued"
	SegmentStatusReady     SegmentStatus = "ready"
	SegmentStatusSynthDone SegmentStatus = "synth_done"
	SegmentStatusFailed    SegmentStatus = "failed"
)

type ArtifactKind string

const (
	ArtifactKindChapterMP3 ArtifactKind = "chapter_mp3"
	ArtifactKindFullMP3    ArtifactKind = "full_mp3"
)

type Story struct {
	ID             int64         `json:"id"`
	Slug           string        `json:"slug"`
	Title          string        `json:"title"`
	Author         string        `json:"author"`
	SourceType     SourceType    `json:"sourceType"`
	SourcePath     string        `json:"sourcePath"`
	LibraryPath    string        `json:"libraryPath"`
	DefaultPreset  ProsodyPreset `json:"defaultPreset"`
	LastBuildJobID *int64        `json:"lastBuildJobId,omitempty"`
	LastError      string        `json:"lastError,omitempty"`
	ChapterCount   int           `json:"chapterCount"`
	LastOpenedAt   *time.Time    `json:"lastOpenedAt,omitempty"`
	CreatedAt      time.Time     `json:"createdAt"`
	UpdatedAt      time.Time     `json:"updatedAt"`
}

type Chapter struct {
	ID              int64         `json:"id"`
	StoryID         int64         `json:"storyId"`
	ChapterIndex    int           `json:"chapterIndex"`
	Title           string        `json:"title"`
	SourceFilePath  string        `json:"sourceFilePath"`
	LibraryFilePath string        `json:"libraryFilePath"`
	NormalizedText  string        `json:"normalizedText,omitempty"`
	Checksum        string        `json:"checksum"`
	Preset          ProsodyPreset `json:"preset"`
	LastBuildJobID  *int64        `json:"lastBuildJobId,omitempty"`
	LastError       string        `json:"lastError,omitempty"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}

type Segment struct {
	ID           int64         `json:"id"`
	ChapterID    int64         `json:"chapterId"`
	SegmentIndex int           `json:"segmentIndex"`
	Text         string        `json:"text"`
	Status       SegmentStatus `json:"status"`
	AudioPath    string        `json:"audioPath,omitempty"`
	Error        string        `json:"error,omitempty"`
	CreatedAt    time.Time     `json:"createdAt"`
	UpdatedAt    time.Time     `json:"updatedAt"`
}

type Artifact struct {
	ID         int64        `json:"id"`
	StoryID    int64        `json:"storyId"`
	ChapterID  *int64       `json:"chapterId,omitempty"`
	Kind       ArtifactKind `json:"kind"`
	FilePath   string       `json:"filePath"`
	DurationMS int64        `json:"durationMs"`
	Checksum   string       `json:"checksum"`
	CreatedAt  time.Time    `json:"createdAt"`
	UpdatedAt  time.Time    `json:"updatedAt"`
}

type BuildJob struct {
	ID              int64      `json:"id"`
	Type            JobType    `json:"type"`
	Status          JobStatus  `json:"status"`
	StoryID         *int64     `json:"storyId,omitempty"`
	ChapterID       *int64     `json:"chapterId,omitempty"`
	RequestedPreset string     `json:"requestedPreset,omitempty"`
	ProgressCurrent int        `json:"progressCurrent"`
	ProgressTotal   int        `json:"progressTotal"`
	LastError       string     `json:"lastError,omitempty"`
	PayloadJSON     string     `json:"payloadJson,omitempty"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type TelegramAccount struct {
	ID                int64     `json:"id"`
	Phone             string    `json:"phone"`
	SessionFile       string    `json:"sessionFile"`
	AuthState         string    `json:"authState"`
	LastPhoneCodeHash string    `json:"lastPhoneCodeHash,omitempty"`
	LastError         string    `json:"lastError,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type TelegramBotProfile struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	BotUsername      string    `json:"botUsername"`
	SearchTemplate   string    `json:"searchTemplate"`
	ChapterTemplate  string    `json:"chapterTemplate"`
	DocumentRule     string    `json:"documentRule"`
	StoryTitleRule   string    `json:"storyTitleRule"`
	ChapterTitleRule string    `json:"chapterTitleRule"`
	Enabled          bool      `json:"enabled"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type StoryDetail struct {
	Story     Story      `json:"story"`
	Chapters  []Chapter  `json:"chapters"`
	Artifacts []Artifact `json:"artifacts"`
}

type AppState struct {
	ConfigSummary ConfigSummary `json:"config"`
}

type ConfigSummary struct {
	ListenAddr           string `json:"listenAddr"`
	LibraryDir           string `json:"libraryDir"`
	DataDir              string `json:"dataDir"`
	FFmpegPath           string `json:"ffmpegPath"`
	EdgeBinary           string `json:"edgeBinary"`
	EdgeVoice            string `json:"edgeVoice"`
	RealtimeTTSBaseURL   string `json:"realtimeTtsBaseUrl"`
	RealtimeDefaultVoice string `json:"realtimeDefaultVoice"`
	RealtimeDefaultSpeed int    `json:"realtimeDefaultSpeed"`
	RealtimeDefaultPitch int    `json:"realtimeDefaultPitch"`
}

type TelegramQRLogin struct {
	ID            string     `json:"id"`
	Status        string     `json:"status"`
	LoginURL      string     `json:"loginUrl,omitempty"`
	QRCodeDataURL string     `json:"qrCodeDataUrl,omitempty"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	Phone         string     `json:"phone,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
}

type ImportFolderRequest struct {
	RootName string              `json:"rootName"`
	Stories  []ImportFolderStory `json:"stories"`
}

type ImportFolderStory struct {
	RelativePath string                `json:"relativePath"`
	Title        string                `json:"title"`
	Chapters     []ImportFolderChapter `json:"chapters"`
}

type ImportFolderChapter struct {
	RelativePath string `json:"relativePath"`
	Title        string `json:"title"`
	Content      string `json:"content"`
}

type LibrarySnapshot struct {
	Stories []Story `json:"stories"`
}

type ChapterContent struct {
	StoryID        int64     `json:"storyId"`
	ChapterID      int64     `json:"chapterId"`
	ChapterIndex   int       `json:"chapterIndex"`
	StoryTitle     string    `json:"storyTitle"`
	ChapterTitle   string    `json:"chapterTitle"`
	Text           string    `json:"text"`
	CharacterCount int       `json:"characterCount"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type ReaderProgress struct {
	StoryID          int64      `json:"storyId"`
	ChapterIndex     int        `json:"chapterIndex"`
	ScrollPercent    float64    `json:"scrollPercent"`
	AudioPositionSec float64    `json:"audioPositionSec"`
	UpdatedAt        *time.Time `json:"updatedAt,omitempty"`
}

type ChunkPlan struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}
