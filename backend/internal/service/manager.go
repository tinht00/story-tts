package service

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"story-tts/backend/internal/audio"
	"story-tts/backend/internal/config"
	"story-tts/backend/internal/library"
	"story-tts/backend/internal/model"
	"story-tts/backend/internal/provider"
	"story-tts/backend/internal/storage"
	"story-tts/backend/internal/telegram"
)

type Manager struct {
	cfg      config.Config
	store    *storage.Store
	provider provider.Provider
	merger   audio.Merger
	tg       *telegram.Manager
	chunker  library.ChunkPlanner

	queue chan int64
	once  sync.Once

	qrMu      sync.RWMutex
	qrRuntime *telegramQRRuntime
}

type telegramQRRuntime struct {
	session model.TelegramQRLogin
	cancel  context.CancelFunc
}

var (
	ttsDividerRe    = regexp.MustCompile(`[-=_*~]{3,}`)
	ttsWhitespaceRe = regexp.MustCompile(`\s+`)
)

const edgeRetryWordLimit = 80

func NewManager(cfg config.Config, store *storage.Store, ttsProvider provider.Provider, merger audio.Merger, tg *telegram.Manager) (*Manager, error) {
	return &Manager{
		cfg:      cfg,
		store:    store,
		provider: ttsProvider,
		merger:   merger,
		tg:       tg,
		chunker:  library.NewChunkPlanner(900),
		queue:    make(chan int64, 32),
	}, nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.once.Do(func() {
		go m.worker(ctx)
	})
	<-ctx.Done()
	return nil
}

func (m *Manager) Summary() model.ConfigSummary {
	return model.ConfigSummary{
		ListenAddr:           m.cfg.ListenAddr,
		LibraryDir:           m.cfg.LibraryDir,
		DataDir:              m.cfg.DataDir,
		FFmpegPath:           m.cfg.FFmpegPath,
		EdgeBinary:           m.cfg.Edge.BinaryPath,
		EdgeVoice:            m.cfg.Edge.DefaultVoice,
		RealtimeTTSBaseURL:   m.cfg.RealtimeTTS.BaseURL,
		RealtimeDefaultVoice: m.cfg.RealtimeTTS.DefaultVoice,
		RealtimeDefaultSpeed: m.cfg.RealtimeTTS.DefaultSpeed,
		RealtimeDefaultPitch: m.cfg.RealtimeTTS.DefaultPitch,
	}
}

func (m *Manager) LibraryDir() string {
	return m.cfg.LibraryDir
}

func (m *Manager) AppState(ctx context.Context) (model.AppState, error) {
	return model.AppState{
		ConfigSummary: m.Summary(),
	}, nil
}

func (m *Manager) ScanLocalStory(ctx context.Context, sourceDir, title string) (model.StoryDetail, error) {
	files, err := library.ScanLocalTXT(sourceDir)
	if err != nil {
		return model.StoryDetail{}, err
	}
	if len(files) == 0 {
		return model.StoryDetail{}, fmt.Errorf("khong tim thay file txt trong %s", sourceDir)
	}
	if title == "" {
		title = filepath.Base(sourceDir)
	}

	slug := library.Slugify(title)
	paths := library.ResolveStoryPaths(m.cfg.LibraryDir, slug)
	if err := library.EnsureStoryDirs(paths); err != nil {
		return model.StoryDetail{}, err
	}

	story, err := m.store.UpsertStory(ctx, model.Story{
		Slug:          slug,
		Title:         title,
		SourceType:    model.SourceTypeLocal,
		SourcePath:    sourceDir,
		LibraryPath:   paths.Root,
		DefaultPreset: model.PresetStable,
	})
	if err != nil {
		return model.StoryDetail{}, err
	}

	var chapters []model.Chapter
	for idx, file := range files {
		parsed, err := library.ParseChapterFile(file)
		if err != nil {
			return model.StoryDetail{}, err
		}

		libraryFile := filepath.Join(paths.SourceChapters, library.ChapterFileName(idx+1, parsed.Title))
		if err := copyFile(file, libraryFile); err != nil {
			return model.StoryDetail{}, err
		}
		chapters = append(chapters, library.ToModelChapter(story.ID, idx+1, libraryFile, parsed, story.DefaultPreset))
	}

	if err := m.store.ReplaceChapters(ctx, story.ID, chapters); err != nil {
		return model.StoryDetail{}, err
	}

	return m.GetStoryDetail(ctx, story.ID)
}

func (m *Manager) ListStories(ctx context.Context) ([]model.Story, error) {
	return m.store.ListStories(ctx)
}

func (m *Manager) GetStoryDetail(ctx context.Context, storyID int64) (model.StoryDetail, error) {
	story, err := m.store.GetStory(ctx, storyID)
	if err != nil {
		return model.StoryDetail{}, err
	}
	chapters, err := m.store.ListChaptersByStory(ctx, storyID)
	if err != nil {
		return model.StoryDetail{}, err
	}
	artifacts, err := m.store.ListArtifactsByStory(ctx, storyID)
	if err != nil {
		return model.StoryDetail{}, err
	}
	return model.StoryDetail{Story: story, Chapters: chapters, Artifacts: artifacts}, nil
}

func (m *Manager) ImportFolder(ctx context.Context, req model.ImportFolderRequest) (model.LibrarySnapshot, error) {
	rootName := strings.TrimSpace(req.RootName)
	if rootName == "" {
		rootName = "library"
	}
	rootPrefix := strings.Trim(strings.ReplaceAll(filepath.ToSlash(rootName), "\\", "/"), "/")

	existingStories, err := m.store.ListStories(ctx)
	if err != nil {
		return model.LibrarySnapshot{}, err
	}

	importedBySource := make(map[string]model.ImportFolderStory, len(req.Stories))
	importedTitles := make(map[string]struct{}, len(req.Stories))
	for _, importedStory := range req.Stories {
		storyRelativePath := strings.Trim(strings.ReplaceAll(importedStory.RelativePath, "\\", "/"), "/")
		if storyRelativePath == "" {
			continue
		}
		sourcePath := strings.Trim(strings.ReplaceAll(filepath.ToSlash(filepath.Join(rootPrefix, storyRelativePath)), "\\", "/"), "/")
		importedBySource[sourcePath] = importedStory
		title := strings.TrimSpace(importedStory.Title)
		if title == "" {
			title = filepath.Base(storyRelativePath)
		}
		importedTitles[title] = struct{}{}
	}

	for _, existingStory := range existingStories {
		normalizedSource := strings.Trim(strings.ReplaceAll(existingStory.SourcePath, "\\", "/"), "/")
		if normalizedSource == "" {
			continue
		}
		_, existsInCurrentRoot := importedBySource[normalizedSource]
		if existsInCurrentRoot {
			continue
		}

		shouldDelete := strings.HasPrefix(normalizedSource, rootPrefix+"/")
		if !shouldDelete && !strings.Contains(normalizedSource, "/") {
			if _, duplicatedLegacyTitle := importedTitles[existingStory.Title]; duplicatedLegacyTitle {
				shouldDelete = true
			}
		}
		if !shouldDelete {
			continue
		}

		if err := m.store.DeleteStory(ctx, existingStory.ID); err != nil {
			return model.LibrarySnapshot{}, err
		}
		if existingStory.LibraryPath != "" {
			if err := os.RemoveAll(existingStory.LibraryPath); err != nil {
				return model.LibrarySnapshot{}, err
			}
		}
	}

	for _, importedStory := range req.Stories {
		if len(importedStory.Chapters) == 0 {
			continue
		}

		storyRelativePath := strings.Trim(strings.ReplaceAll(importedStory.RelativePath, "\\", "/"), "/")
		storyTitle := strings.TrimSpace(importedStory.Title)
		if storyTitle == "" {
			storyTitle = filepath.Base(storyRelativePath)
		}
		sourcePath := strings.Trim(strings.ReplaceAll(filepath.ToSlash(filepath.Join(rootName, storyRelativePath)), "\\", "/"), "/")
		slug := stableStorySlug(sourcePath, storyTitle)
		paths := library.ResolveStoryPaths(m.cfg.LibraryDir, slug)
		if err := resetStoryWorkspace(paths); err != nil {
			return model.LibrarySnapshot{}, err
		}
		if err := library.EnsureStoryDirs(paths); err != nil {
			return model.LibrarySnapshot{}, err
		}

		story, err := m.store.UpsertStory(ctx, model.Story{
			Slug:          slug,
			Title:         storyTitle,
			SourceType:    model.SourceTypeLocal,
			SourcePath:    sourcePath,
			LibraryPath:   paths.Root,
			DefaultPreset: model.PresetStable,
		})
		if err != nil {
			return model.LibrarySnapshot{}, err
		}

		if err := m.store.DeleteArtifactsForStory(ctx, story.ID); err != nil {
			return model.LibrarySnapshot{}, err
		}

		chapters := make([]model.Chapter, 0, len(importedStory.Chapters))
		for idx, importedChapter := range importedStory.Chapters {
			chapterTitle := strings.TrimSpace(importedChapter.Title)
			if chapterTitle == "" {
				chapterTitle = strings.TrimSuffix(filepath.Base(importedChapter.RelativePath), filepath.Ext(importedChapter.RelativePath))
			}
			parsed := library.ParseChapterContent(chapterTitle, []byte(importedChapter.Content), importedChapter.RelativePath)
			libraryFile := filepath.Join(paths.SourceChapters, library.ChapterFileName(idx+1, parsed.Title))
			if err := os.WriteFile(libraryFile, []byte(parsed.NormalizedText), 0o644); err != nil {
				return model.LibrarySnapshot{}, err
			}
			chapters = append(chapters, library.ToModelChapter(story.ID, idx+1, libraryFile, parsed, story.DefaultPreset))
		}

		if err := m.store.ReplaceChapters(ctx, story.ID, chapters); err != nil {
			return model.LibrarySnapshot{}, err
		}
	}

	stories, err := m.store.ListStories(ctx)
	if err != nil {
		return model.LibrarySnapshot{}, err
	}
	return model.LibrarySnapshot{Stories: stories}, nil
}

func (m *Manager) GetChapterContent(ctx context.Context, chapterID int64) (model.ChapterContent, error) {
	chapter, err := m.store.GetChapter(ctx, chapterID)
	if err != nil {
		return model.ChapterContent{}, err
	}
	story, err := m.store.GetStory(ctx, chapter.StoryID)
	if err != nil {
		return model.ChapterContent{}, err
	}
	text := library.NormalizeChapterText(chapter.NormalizedText)

	return model.ChapterContent{
		StoryID:        story.ID,
		ChapterID:      chapter.ID,
		ChapterIndex:   chapter.ChapterIndex,
		StoryTitle:     story.Title,
		ChapterTitle:   chapter.Title,
		Text:           text,
		CharacterCount: len([]rune(text)),
		UpdatedAt:      chapter.UpdatedAt,
	}, nil
}

func (m *Manager) GetReaderProgress(ctx context.Context, storyID int64) (model.ReaderProgress, error) {
	progress, err := m.store.GetReaderProgress(ctx, storyID)
	if err == nil {
		return progress, nil
	}
	if isNoRows(err) {
		return model.ReaderProgress{StoryID: storyID}, nil
	}
	return model.ReaderProgress{}, err
}

func (m *Manager) SaveReaderProgress(ctx context.Context, progress model.ReaderProgress) (model.ReaderProgress, error) {
	if progress.StoryID <= 0 {
		return model.ReaderProgress{}, fmt.Errorf("story_id khong hop le")
	}
	if progress.ChapterIndex <= 0 {
		progress.ChapterIndex = 1
	}
	if progress.ScrollPercent < 0 {
		progress.ScrollPercent = 0
	}
	if progress.ScrollPercent > 1 {
		progress.ScrollPercent = 1
	}
	if progress.AudioPositionSec < 0 {
		progress.AudioPositionSec = 0
	}
	if err := m.store.SaveReaderProgress(ctx, progress); err != nil {
		return model.ReaderProgress{}, err
	}
	return m.store.GetReaderProgress(ctx, progress.StoryID)
}

func (m *Manager) QueueBuildStory(ctx context.Context, storyID int64, preset model.ProsodyPreset) (model.BuildJob, error) {
	payload, _ := json.Marshal(map[string]any{"storyId": storyID})
	job, err := m.store.CreateJob(ctx, model.BuildJob{
		Type:            model.JobTypeBuildStory,
		Status:          model.JobStatusQueued,
		StoryID:         &storyID,
		RequestedPreset: string(preset),
		PayloadJSON:     string(payload),
	})
	if err != nil {
		return model.BuildJob{}, err
	}
	m.queue <- job.ID
	return job, nil
}

func (m *Manager) QueueBuildChapter(ctx context.Context, chapterID int64, preset model.ProsodyPreset) (model.BuildJob, error) {
	payload, _ := json.Marshal(map[string]any{"chapterId": chapterID})
	job, err := m.store.CreateJob(ctx, model.BuildJob{
		Type:            model.JobTypeBuildChapter,
		Status:          model.JobStatusQueued,
		ChapterID:       &chapterID,
		RequestedPreset: string(preset),
		PayloadJSON:     string(payload),
	})
	if err != nil {
		return model.BuildJob{}, err
	}
	m.queue <- job.ID
	return job, nil
}

func (m *Manager) ListJobs(ctx context.Context, limit int) ([]model.BuildJob, error) {
	return m.store.ListJobs(ctx, limit)
}

func (m *Manager) GetJob(ctx context.Context, jobID int64) (model.BuildJob, error) {
	return m.store.GetJob(ctx, jobID)
}

func (m *Manager) SaveBotProfile(ctx context.Context, profile model.TelegramBotProfile) (model.TelegramBotProfile, error) {
	return m.store.SaveBotProfile(ctx, profile)
}

func (m *Manager) ListBotProfiles(ctx context.Context) ([]model.TelegramBotProfile, error) {
	return m.store.ListBotProfiles(ctx)
}

func (m *Manager) TelegramSendCode(ctx context.Context, phone string) (model.TelegramAccount, error) {
	result, err := m.tg.SendCode(ctx, phone)
	account := model.TelegramAccount{
		Phone:             phone,
		SessionFile:       m.tg.SessionFile(),
		AuthState:         "code_sent",
		LastPhoneCodeHash: result.PhoneCodeHash,
	}
	if err != nil {
		account.AuthState = "error"
		account.LastError = err.Error()
	}
	return m.store.UpsertTelegramAccount(ctx, account)
}

func (m *Manager) TelegramSignIn(ctx context.Context, phone, code string) (model.TelegramAccount, error) {
	account, err := m.store.GetTelegramAccount(ctx)
	if err != nil {
		return model.TelegramAccount{}, err
	}
	err = m.tg.SignIn(ctx, phone, code, account.LastPhoneCodeHash)
	account.Phone = phone
	account.SessionFile = m.tg.SessionFile()
	account.AuthState = "authenticated"
	account.LastError = ""
	if err != nil {
		account.AuthState = "password_required"
		account.LastError = err.Error()
	}
	return m.store.UpsertTelegramAccount(ctx, account)
}

func (m *Manager) TelegramPassword(ctx context.Context, password string) (model.TelegramAccount, error) {
	account, err := m.store.GetTelegramAccount(ctx)
	if err != nil {
		return model.TelegramAccount{}, err
	}
	err = m.tg.Password(ctx, password)
	account.AuthState = "authenticated"
	account.LastError = ""
	if err != nil {
		account.AuthState = "error"
		account.LastError = err.Error()
	}
	return m.store.UpsertTelegramAccount(ctx, account)
}

func (m *Manager) StartTelegramQRLogin(ctx context.Context) (model.TelegramQRLogin, error) {
	if !m.tg.IsConfigured() {
		account := model.TelegramAccount{
			SessionFile: m.tg.SessionFile(),
			AuthState:   "error",
			LastError:   "telegram app id/app hash chua duoc cau hinh",
		}
		if existing, err := m.store.GetTelegramAccount(ctx); err == nil {
			account.ID = existing.ID
			account.Phone = existing.Phone
		}
		_, _ = m.store.UpsertTelegramAccount(ctx, account)
		return model.TelegramQRLogin{}, fmt.Errorf(account.LastError)
	}

	m.qrMu.Lock()
	if m.qrRuntime != nil {
		switch m.qrRuntime.session.Status {
		case "starting", "pending":
			session := m.qrRuntime.session
			m.qrMu.Unlock()
			return session, nil
		default:
			if m.qrRuntime.cancel != nil {
				m.qrRuntime.cancel()
			}
			m.qrRuntime = nil
		}
	}

	session := model.TelegramQRLogin{
		ID:     newTelegramQRSessionID(),
		Status: "starting",
	}
	runCtx, cancel := context.WithCancel(context.Background())
	m.qrRuntime = &telegramQRRuntime{
		session: session,
		cancel:  cancel,
	}
	m.qrMu.Unlock()

	go m.runTelegramQRLogin(runCtx, session.ID)

	return session, nil
}

func (m *Manager) CancelTelegramQRLogin() *model.TelegramQRLogin {
	m.qrMu.Lock()
	defer m.qrMu.Unlock()

	if m.qrRuntime == nil {
		return nil
	}
	if m.qrRuntime.cancel != nil {
		m.qrRuntime.cancel()
	}
	session := m.qrRuntime.session
	return &session
}

func (m *Manager) currentTelegramQRLogin() *model.TelegramQRLogin {
	m.qrMu.RLock()
	defer m.qrMu.RUnlock()

	if m.qrRuntime == nil {
		return nil
	}
	session := m.qrRuntime.session
	return &session
}

func (m *Manager) CurrentTelegramQRLogin() *model.TelegramQRLogin {
	return m.currentTelegramQRLogin()
}

func (m *Manager) runTelegramQRLogin(ctx context.Context, sessionID string) {
	account, err := m.tg.RunQRLogin(ctx, func(snapshot model.TelegramQRLogin) {
		m.updateTelegramQRSession(sessionID, func(session *model.TelegramQRLogin) {
			session.Status = snapshot.Status
			session.LoginURL = snapshot.LoginURL
			session.QRCodeDataURL = snapshot.QRCodeDataURL
			session.ExpiresAt = snapshot.ExpiresAt
			session.LastError = snapshot.LastError
			if snapshot.Phone != "" {
				session.Phone = snapshot.Phone
			}
		})
	})
	if err != nil {
		if ctx.Err() != nil {
			m.finishTelegramQRLogin(sessionID, "cancelled", "")
			return
		}
		m.finishTelegramQRLogin(sessionID, "error", err.Error())
		return
	}

	if _, storeErr := m.store.UpsertTelegramAccount(context.Background(), account); storeErr != nil {
		m.finishTelegramQRLogin(sessionID, "error", storeErr.Error())
		return
	}
	m.updateTelegramQRSession(sessionID, func(session *model.TelegramQRLogin) {
		session.Phone = account.Phone
	})
	m.finishTelegramQRLogin(sessionID, "authenticated", "")
}

func (m *Manager) updateTelegramQRSession(sessionID string, apply func(session *model.TelegramQRLogin)) {
	m.qrMu.Lock()
	defer m.qrMu.Unlock()

	if m.qrRuntime == nil || m.qrRuntime.session.ID != sessionID {
		return
	}
	apply(&m.qrRuntime.session)
}

func (m *Manager) finishTelegramQRLogin(sessionID, status, lastError string) {
	m.updateTelegramQRSession(sessionID, func(session *model.TelegramQRLogin) {
		session.Status = status
		session.LastError = lastError
		if status == "authenticated" || status == "cancelled" || status == "error" {
			session.LoginURL = ""
			session.QRCodeDataURL = ""
			session.ExpiresAt = nil
		}
	})
}

func newTelegramQRSessionID() string {
	sum := sha1.Sum([]byte(fmt.Sprintf("telegram-qr-%d-%d", os.Getpid(), time.Now().UnixNano())))
	return hex.EncodeToString(sum[:8])
}

func (m *Manager) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case jobID := <-m.queue:
			_ = m.processJob(ctx, jobID)
		}
	}
}

func (m *Manager) processJob(ctx context.Context, jobID int64) error {
	job, err := m.store.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	if err := m.store.UpdateJobStatus(ctx, jobID, model.JobStatusRunning, job.ProgressCurrent, job.ProgressTotal, ""); err != nil {
		return err
	}

	switch job.Type {
	case model.JobTypeBuildStory:
		err = m.runBuildStory(ctx, job)
	case model.JobTypeBuildChapter:
		err = m.runBuildChapter(ctx, job)
	default:
		err = fmt.Errorf("job type %s chua duoc ho tro", job.Type)
	}

	if err != nil {
		_ = m.store.UpdateJobStatus(ctx, job.ID, model.JobStatusFailed, job.ProgressCurrent, job.ProgressTotal, err.Error())
		return err
	}
	return m.store.UpdateJobStatus(ctx, job.ID, model.JobStatusCompleted, job.ProgressTotal, job.ProgressTotal, "")
}

func (m *Manager) runBuildStory(ctx context.Context, job model.BuildJob) error {
	if job.StoryID == nil {
		return fmt.Errorf("job build story khong co story_id")
	}
	chapters, err := m.store.ListChaptersByStory(ctx, *job.StoryID)
	if err != nil {
		return err
	}
	if err := m.store.UpdateJobProgress(ctx, job.ID, 0, len(chapters)); err != nil {
		return err
	}

	for idx, chapter := range chapters {
		if _, err := m.buildSingleChapter(ctx, job.ID, chapter, model.ProsodyPreset(job.RequestedPreset)); err != nil {
			return err
		}
		if err := m.store.UpdateJobProgress(ctx, job.ID, idx+1, len(chapters)); err != nil {
			return err
		}
	}

	return m.mergeFullBook(ctx, *job.StoryID)
}

func (m *Manager) runBuildChapter(ctx context.Context, job model.BuildJob) error {
	if job.ChapterID == nil {
		return fmt.Errorf("job build chapter khong co chapter_id")
	}
	chapter, err := m.store.GetChapter(ctx, *job.ChapterID)
	if err != nil {
		return err
	}
	if err := m.store.UpdateJobProgress(ctx, job.ID, 0, 1); err != nil {
		return err
	}
	if _, err := m.buildSingleChapter(ctx, job.ID, chapter, model.ProsodyPreset(job.RequestedPreset)); err != nil {
		return err
	}
	if err := m.store.UpdateJobProgress(ctx, job.ID, 1, 1); err != nil {
		return err
	}
	return m.mergeFullBook(ctx, chapter.StoryID)
}

func (m *Manager) buildSingleChapter(ctx context.Context, jobID int64, chapter model.Chapter, override model.ProsodyPreset) (string, error) {
	preset := chapter.Preset
	if override != "" {
		preset = override
	}

	plans := m.chunker.Plan(sanitizeTTSInput(chapter.NormalizedText))
	if len(plans) == 0 {
		return "", fmt.Errorf("chuong %d khong co noi dung de synthesize", chapter.ChapterIndex)
	}

	if err := m.store.UpdateChapterBuildState(ctx, chapter.ID, jobID, ""); err != nil {
		return "", err
	}

	var segments []model.Segment
	for _, plan := range plans {
		segments = append(segments, model.Segment{
			ChapterID:    chapter.ID,
			SegmentIndex: plan.Index,
			Text:         plan.Text,
			Status:       model.SegmentStatusQueued,
		})
	}
	if err := m.store.ReplaceSegments(ctx, chapter.ID, segments); err != nil {
		return "", err
	}
	if err := m.store.DeleteArtifactsForChapter(ctx, chapter.ID); err != nil {
		return "", err
	}
	if err := m.store.DeleteFullArtifactsForStory(ctx, chapter.StoryID); err != nil {
		return "", err
	}

	story, err := m.store.GetStory(ctx, chapter.StoryID)
	if err != nil {
		return "", err
	}
	paths := library.ResolveStoryPaths(m.cfg.LibraryDir, story.Slug)
	if err := library.EnsureStoryDirs(paths); err != nil {
		return "", err
	}

	segmentDir := filepath.Join(paths.WorkSegments, fmt.Sprintf("%03d", chapter.ChapterIndex))
	if err := os.MkdirAll(segmentDir, 0o755); err != nil {
		return "", err
	}

	var outputs []string
	for _, plan := range plans {
		outputPath := filepath.Join(segmentDir, fmt.Sprintf("%04d.mp3", plan.Index))
		if err := m.synthesizeWithFallback(ctx, plan.Text, outputPath, m.cfg.Edge.DefaultVoice, preset, 0); err != nil {
			_ = m.store.UpdateSegmentState(ctx, chapter.ID, plan.Index, model.SegmentStatusFailed, outputPath, err.Error())
			_ = m.store.UpdateChapterBuildState(ctx, chapter.ID, jobID, err.Error())
			return "", err
		}
		if err := m.store.UpdateSegmentState(ctx, chapter.ID, plan.Index, model.SegmentStatusSynthDone, outputPath, ""); err != nil {
			return "", err
		}
		outputs = append(outputs, outputPath)
	}

	chapterAudio := filepath.Join(paths.ArtifactChapters, library.ChapterAudioName(chapter.ChapterIndex, chapter.Title))
	if err := m.merger.MergeMP3(ctx, outputs, chapterAudio); err != nil {
		_ = m.store.UpdateChapterBuildState(ctx, chapter.ID, jobID, err.Error())
		return "", err
	}
	if err := m.store.UpdateChapterBuildState(ctx, chapter.ID, jobID, ""); err != nil {
		return "", err
	}

	chapterID := chapter.ID
	if err := m.store.UpsertArtifact(ctx, model.Artifact{
		StoryID:   chapter.StoryID,
		ChapterID: &chapterID,
		Kind:      model.ArtifactKindChapterMP3,
		FilePath:  chapterAudio,
		Checksum:  checksumFile(chapterAudio),
	}); err != nil {
		return "", err
	}

	return chapterAudio, nil
}

func (m *Manager) mergeFullBook(ctx context.Context, storyID int64) error {
	detail, err := m.GetStoryDetail(ctx, storyID)
	if err != nil {
		return err
	}

	paths := library.ResolveStoryPaths(m.cfg.LibraryDir, detail.Story.Slug)
	var inputs []string
	for _, chapter := range detail.Chapters {
		for _, artifact := range detail.Artifacts {
			if artifact.Kind == model.ArtifactKindChapterMP3 && artifact.ChapterID != nil && *artifact.ChapterID == chapter.ID {
				inputs = append(inputs, artifact.FilePath)
			}
		}
	}
	if len(inputs) == 0 {
		return nil
	}

	fullPath := filepath.Join(paths.ArtifactFull, detail.Story.Slug+".mp3")
	if err := m.merger.MergeMP3(ctx, inputs, fullPath); err != nil {
		return err
	}
	return m.store.UpsertArtifact(ctx, model.Artifact{
		StoryID:  storyID,
		Kind:     model.ArtifactKindFullMP3,
		FilePath: fullPath,
		Checksum: checksumFile(fullPath),
	})
}

func resetStoryWorkspace(paths library.StoryPaths) error {
	for _, dir := range []string{paths.SourceChapters, paths.ArtifactChapters, paths.ArtifactFull, paths.WorkSegments} {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}

func stableStorySlug(relativePath, title string) string {
	base := library.Slugify(title)
	return fmt.Sprintf("%s-%s", base, checksumText(relativePath)[:8])
}

func checksumText(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (m *Manager) synthesizeWithFallback(ctx context.Context, text, outputPath, voice string, preset model.ProsodyPreset, depth int) error {
	text = sanitizeTTSInput(text)
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("doan van sau khi lam sach dang rong")
	}

	err := m.provider.Synthesize(ctx, provider.SynthesizeInput{
		Text:       text,
		OutputPath: outputPath,
		Voice:      voice,
		Preset:     preset,
	})
	if err == nil {
		if stat, statErr := os.Stat(outputPath); statErr == nil && stat.Size() > 0 {
			return nil
		}
		err = fmt.Errorf("edge-tts khong tao ra audio hop le")
	}

	if depth >= 2 {
		if silenceErr := m.writeSilenceMP3(ctx, outputPath, 350); silenceErr == nil {
			log.Printf("tts chen silence fallback depth=%d sau loi: %v", depth, err)
			return nil
		}
		return err
	}

	parts := splitTextForRetry(text, retryChunkLimit(depth))
	if len(parts) <= 1 {
		return err
	}

	log.Printf("tts fallback depth=%d, chia doan loi thanh %d phan", depth+1, len(parts))
	tempDir := filepath.Join(filepath.Dir(outputPath), fmt.Sprintf(".retry-%d", time.Now().UnixNano()))
	if mkErr := os.MkdirAll(tempDir, 0o755); mkErr != nil {
		return fmt.Errorf("khong tao duoc thu muc retry: %w", mkErr)
	}
	defer os.RemoveAll(tempDir)

	outputs := make([]string, 0, len(parts))
	skipped := 0
	for index, part := range parts {
		partOutput := filepath.Join(tempDir, fmt.Sprintf("%04d.mp3", index))
		if retryErr := m.synthesizeWithFallback(ctx, part, partOutput, voice, preset, depth+1); retryErr != nil {
			skipped++
			log.Printf("tts bo qua subpart depth=%d index=%d: %v", depth+1, index, retryErr)
			continue
		}
		outputs = append(outputs, partOutput)
	}
	if len(outputs) == 0 {
		return fmt.Errorf("%w; retry depth=%d that bai tren %d/%d subpart", err, depth+1, skipped, len(parts))
	}
	if skipped > 0 {
		log.Printf("tts fallback depth=%d bo qua %d/%d subpart loi, van merge phan con lai", depth+1, skipped, len(parts))
	}

	if len(outputs) == 1 {
		return copyFile(outputs[0], outputPath)
	}
	return m.merger.MergeMP3(ctx, outputs, outputPath)
}

func (m *Manager) writeSilenceMP3(ctx context.Context, outputPath string, durationMS int) error {
	if strings.TrimSpace(m.cfg.FFmpegPath) == "" {
		return fmt.Errorf("ffmpeg chua duoc cau hinh")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	duration := fmt.Sprintf("%.2f", float64(durationMS)/1000)
	cmd := exec.CommandContext(
		ctx,
		m.cfg.FFmpegPath,
		"-f", "lavfi",
		"-i", "anullsrc=r=24000:cl=mono",
		"-t", duration,
		"-q:a", "9",
		"-acodec", "libmp3lame",
		"-y",
		outputPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("khong tao duoc silence mp3: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func sanitizeTTSInput(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	text = norm.NFC.String(text)
	text = strings.ReplaceAll(text, "\u00a0", " ")
	text = library.NormalizeChapterText(text)
	text = ttsDividerRe.ReplaceAllString(text, " ")
	text = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, text)
	text = ttsWhitespaceRe.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func splitTextForRetry(text string, limit int) []string {
	clauses := splitRetryClauses(text)
	if len(clauses) == 0 {
		return nil
	}

	var out []string
	var current strings.Builder
	currentWords := 0
	flush := func() {
		chunk := strings.TrimSpace(current.String())
		if chunk != "" {
			out = append(out, chunk)
		}
		current.Reset()
		currentWords = 0
	}

	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		clauseWords := len(strings.Fields(clause))
		if clauseWords > edgeRetryWordLimit || len(clause) > limit {
			for _, hardPart := range splitHardClause(clause, limit) {
				hardWords := len(strings.Fields(hardPart))
				if current.Len() > 0 && (currentWords+hardWords > edgeRetryWordLimit || current.Len()+len(hardPart)+1 > limit) {
					flush()
				}
				if current.Len() > 0 {
					current.WriteByte(' ')
				}
				current.WriteString(hardPart)
				currentWords += hardWords
				flush()
			}
			continue
		}

		if current.Len() > 0 && (currentWords+clauseWords > edgeRetryWordLimit || current.Len()+len(clause)+1 > limit) {
			flush()
		}
		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(clause)
		currentWords += clauseWords
	}

	if current.Len() > 0 {
		flush()
	}
	return out
}

func retryChunkLimit(depth int) int {
	switch depth {
	case 0:
		return 180
	case 1:
		return 120
	default:
		return 90
	}
}

func splitRetryClauses(text string) []string {
	var clauses []string
	var current strings.Builder
	for _, r := range text {
		current.WriteRune(r)
		switch r {
		case '.', '!', '?', ';', ':', ',', '…':
			clauses = append(clauses, strings.TrimSpace(current.String()))
			current.Reset()
		}
	}
	if current.Len() > 0 {
		clauses = append(clauses, strings.TrimSpace(current.String()))
	}
	return clauses
}

func splitHardClause(text string, limit int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var out []string
	var current strings.Builder
	currentWords := 0
	for _, word := range words {
		if current.Len() > 0 && (current.Len()+len(word)+1 > limit || currentWords+1 > edgeRetryWordLimit) {
			out = append(out, strings.TrimSpace(current.String()))
			current.Reset()
			currentWords = 0
		}
		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(word)
		currentWords++
	}
	if current.Len() > 0 {
		out = append(out, strings.TrimSpace(current.String()))
	}
	return out
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func checksumFile(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	sum := sha1.New()
	if _, err := io.Copy(sum, file); err != nil {
		return ""
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func isNoRows(err error) bool {
	return err == sql.ErrNoRows || (err != nil && strings.Contains(err.Error(), "sql: no rows"))
}
