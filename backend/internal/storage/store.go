package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"story-tts/backend/internal/model"
)

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_busy_timeout=5000", filepath.ToSlash(dbPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`create table if not exists stories (
			id integer primary key autoincrement,
			slug text not null unique,
			title text not null,
			author text not null default '',
			source_type text not null,
			source_path text not null,
			library_path text not null,
			default_preset text not null,
			last_build_job_id integer,
			last_error text not null default '',
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists chapters (
			id integer primary key autoincrement,
			story_id integer not null references stories(id) on delete cascade,
			chapter_index integer not null,
			title text not null,
			source_file_path text not null,
			library_file_path text not null,
			normalized_text text not null default '',
			checksum text not null,
			preset text not null,
			last_build_job_id integer,
			last_error text not null default '',
			created_at text not null,
			updated_at text not null,
			unique(story_id, chapter_index)
		);`,
		`create table if not exists build_jobs (
			id integer primary key autoincrement,
			type text not null,
			status text not null,
			story_id integer references stories(id) on delete set null,
			chapter_id integer references chapters(id) on delete set null,
			requested_preset text not null default '',
			progress_current integer not null default 0,
			progress_total integer not null default 0,
			last_error text not null default '',
			payload_json text not null default '',
			started_at text,
			finished_at text,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists segments (
			id integer primary key autoincrement,
			chapter_id integer not null references chapters(id) on delete cascade,
			segment_index integer not null,
			text text not null,
			status text not null,
			audio_path text not null default '',
			error text not null default '',
			created_at text not null,
			updated_at text not null,
			unique(chapter_id, segment_index)
		);`,
		`create table if not exists artifacts (
			id integer primary key autoincrement,
			story_id integer not null references stories(id) on delete cascade,
			chapter_id integer references chapters(id) on delete cascade,
			kind text not null,
			file_path text not null,
			duration_ms integer not null default 0,
			checksum text not null default '',
			created_at text not null,
			updated_at text not null,
			unique(kind, file_path)
		);`,
		`create table if not exists telegram_accounts (
			id integer primary key autoincrement,
			phone text not null default '',
			session_file text not null,
			auth_state text not null,
			last_phone_code_hash text not null default '',
			last_error text not null default '',
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists telegram_bot_profiles (
			id integer primary key autoincrement,
			name text not null,
			bot_username text not null,
			search_template text not null default '',
			chapter_template text not null default '',
			document_rule text not null default '',
			story_title_rule text not null default '',
			chapter_title_rule text not null default '',
			enabled integer not null default 1,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists reader_progress (
			story_id integer primary key references stories(id) on delete cascade,
			chapter_index integer not null default 1,
			scroll_percent real not null default 0,
			audio_position_sec real not null default 0,
			updated_at text not null
		);`,
		`create table if not exists recent_stories (
			story_id integer primary key references stories(id) on delete cascade,
			last_opened_at text not null
		);`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

const storySelectColumns = `
	select
		s.id,
		s.slug,
		s.title,
		s.author,
		s.source_type,
		s.source_path,
		s.library_path,
		s.default_preset,
		s.last_build_job_id,
		s.last_error,
		s.created_at,
		s.updated_at,
		(select count(1) from chapters c where c.story_id = s.id) as chapter_count,
		r.last_opened_at
	from stories s
	left join recent_stories r on r.story_id = s.id
`

func (s *Store) UpsertStory(ctx context.Context, story model.Story) (model.Story, error) {
	now := nowText()
	result, err := s.db.ExecContext(ctx, `
		insert into stories (slug, title, author, source_type, source_path, library_path, default_preset, last_build_job_id, last_error, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(slug) do update set
			title = excluded.title,
			author = excluded.author,
			source_type = excluded.source_type,
			source_path = excluded.source_path,
			library_path = excluded.library_path,
			default_preset = excluded.default_preset,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at
	`, story.Slug, story.Title, story.Author, story.SourceType, story.SourcePath, story.LibraryPath, story.DefaultPreset, story.LastBuildJobID, story.LastError, now, now)
	if err != nil {
		return model.Story{}, err
	}
	if id, err := result.LastInsertId(); err == nil && id > 0 {
		story.ID = id
	}
	return s.GetStoryBySlug(ctx, story.Slug)
}

func (s *Store) GetStoryBySlug(ctx context.Context, slug string) (model.Story, error) {
	row := s.db.QueryRowContext(ctx, storySelectColumns+` where s.slug = ?`, slug)
	return scanStory(row)
}

func (s *Store) GetStory(ctx context.Context, id int64) (model.Story, error) {
	row := s.db.QueryRowContext(ctx, storySelectColumns+` where s.id = ?`, id)
	return scanStory(row)
}

func (s *Store) ListStories(ctx context.Context) ([]model.Story, error) {
	rows, err := s.db.QueryContext(ctx, storySelectColumns+` order by coalesce(r.last_opened_at, s.updated_at) desc, s.title asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.Story
	for rows.Next() {
		item, err := scanStory(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) DeleteStory(ctx context.Context, storyID int64) error {
	_, err := s.db.ExecContext(ctx, `delete from stories where id = ?`, storyID)
	return err
}

func (s *Store) ReplaceChapters(ctx context.Context, storyID int64, chapters []model.Chapter) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `delete from chapters where story_id = ?`, storyID); err != nil {
		return err
	}

	now := nowText()
	for _, chapter := range chapters {
		if _, err = tx.ExecContext(ctx, `
			insert into chapters (story_id, chapter_index, title, source_file_path, library_file_path, normalized_text, checksum, preset, last_build_job_id, last_error, created_at, updated_at)
			values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, storyID, chapter.ChapterIndex, chapter.Title, chapter.SourceFilePath, chapter.LibraryFilePath, chapter.NormalizedText, chapter.Checksum, chapter.Preset, chapter.LastBuildJobID, chapter.LastError, now, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) ListChaptersByStory(ctx context.Context, storyID int64) ([]model.Chapter, error) {
	rows, err := s.db.QueryContext(ctx, `select id, story_id, chapter_index, title, source_file_path, library_file_path, normalized_text, checksum, preset, last_build_job_id, last_error, created_at, updated_at from chapters where story_id = ? order by chapter_index asc`, storyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.Chapter
	for rows.Next() {
		item, err := scanChapter(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetChapter(ctx context.Context, chapterID int64) (model.Chapter, error) {
	row := s.db.QueryRowContext(ctx, `select id, story_id, chapter_index, title, source_file_path, library_file_path, normalized_text, checksum, preset, last_build_job_id, last_error, created_at, updated_at from chapters where id = ?`, chapterID)
	return scanChapter(row)
}

func (s *Store) UpdateChapterBuildState(ctx context.Context, chapterID, jobID int64, lastError string) error {
	_, err := s.db.ExecContext(ctx, `update chapters set last_build_job_id = ?, last_error = ?, updated_at = ? where id = ?`, jobID, lastError, nowText(), chapterID)
	return err
}

func (s *Store) ReplaceSegments(ctx context.Context, chapterID int64, segments []model.Segment) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `delete from segments where chapter_id = ?`, chapterID); err != nil {
		return err
	}

	now := nowText()
	for _, segment := range segments {
		if _, err = tx.ExecContext(ctx, `
			insert into segments (chapter_id, segment_index, text, status, audio_path, error, created_at, updated_at)
			values (?, ?, ?, ?, ?, ?, ?, ?)
		`, chapterID, segment.SegmentIndex, segment.Text, segment.Status, segment.AudioPath, segment.Error, now, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) UpdateSegmentState(ctx context.Context, chapterID int64, segmentIndex int, status model.SegmentStatus, audioPath, segErr string) error {
	_, err := s.db.ExecContext(ctx, `update segments set status = ?, audio_path = ?, error = ?, updated_at = ? where chapter_id = ? and segment_index = ?`,
		status, audioPath, segErr, nowText(), chapterID, segmentIndex)
	return err
}

func (s *Store) ListSegmentsByChapter(ctx context.Context, chapterID int64) ([]model.Segment, error) {
	rows, err := s.db.QueryContext(ctx, `select id, chapter_id, segment_index, text, status, audio_path, error, created_at, updated_at from segments where chapter_id = ? order by segment_index asc`, chapterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.Segment
	for rows.Next() {
		item, err := scanSegment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) UpsertArtifact(ctx context.Context, artifact model.Artifact) error {
	now := nowText()
	_, err := s.db.ExecContext(ctx, `
		insert into artifacts (story_id, chapter_id, kind, file_path, duration_ms, checksum, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(kind, file_path) do update set
			duration_ms = excluded.duration_ms,
			checksum = excluded.checksum,
			updated_at = excluded.updated_at
	`, artifact.StoryID, artifact.ChapterID, artifact.Kind, artifact.FilePath, artifact.DurationMS, artifact.Checksum, now, now)
	return err
}

func (s *Store) DeleteArtifactsForChapter(ctx context.Context, chapterID int64) error {
	_, err := s.db.ExecContext(ctx, `delete from artifacts where chapter_id = ?`, chapterID)
	return err
}

func (s *Store) DeleteFullArtifactsForStory(ctx context.Context, storyID int64) error {
	_, err := s.db.ExecContext(ctx, `delete from artifacts where story_id = ? and kind = ?`, storyID, model.ArtifactKindFullMP3)
	return err
}

func (s *Store) DeleteArtifactsForStory(ctx context.Context, storyID int64) error {
	_, err := s.db.ExecContext(ctx, `delete from artifacts where story_id = ?`, storyID)
	return err
}

func (s *Store) ListArtifactsByStory(ctx context.Context, storyID int64) ([]model.Artifact, error) {
	rows, err := s.db.QueryContext(ctx, `select id, story_id, chapter_id, kind, file_path, duration_ms, checksum, created_at, updated_at from artifacts where story_id = ? order by kind asc, updated_at desc`, storyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.Artifact
	for rows.Next() {
		item, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateJob(ctx context.Context, job model.BuildJob) (model.BuildJob, error) {
	now := nowText()
	result, err := s.db.ExecContext(ctx, `
		insert into build_jobs (type, status, story_id, chapter_id, requested_preset, progress_current, progress_total, last_error, payload_json, started_at, finished_at, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, job.Type, job.Status, job.StoryID, job.ChapterID, job.RequestedPreset, job.ProgressCurrent, job.ProgressTotal, job.LastError, job.PayloadJSON, nil, nil, now, now)
	if err != nil {
		return model.BuildJob{}, err
	}
	jobID, err := result.LastInsertId()
	if err != nil {
		return model.BuildJob{}, err
	}
	return s.GetJob(ctx, jobID)
}

func (s *Store) UpdateJobStatus(ctx context.Context, jobID int64, status model.JobStatus, current, total int, lastError string) error {
	now := nowText()
	var startedAt any
	var finishedAt any
	if status == model.JobStatusRunning {
		startedAt = now
	}
	if status == model.JobStatusCompleted || status == model.JobStatusFailed || status == model.JobStatusCancelled {
		finishedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
		update build_jobs
		set status = ?, progress_current = ?, progress_total = ?, last_error = ?, started_at = coalesce(started_at, ?), finished_at = coalesce(?, finished_at), updated_at = ?
		where id = ?
	`, status, current, total, lastError, startedAt, finishedAt, now, jobID)
	return err
}

func (s *Store) UpdateJobProgress(ctx context.Context, jobID int64, current, total int) error {
	_, err := s.db.ExecContext(ctx, `update build_jobs set progress_current = ?, progress_total = ?, updated_at = ? where id = ?`, current, total, nowText(), jobID)
	return err
}

func (s *Store) GetJob(ctx context.Context, jobID int64) (model.BuildJob, error) {
	row := s.db.QueryRowContext(ctx, `select id, type, status, story_id, chapter_id, requested_preset, progress_current, progress_total, last_error, payload_json, started_at, finished_at, created_at, updated_at from build_jobs where id = ?`, jobID)
	return scanJob(row)
}

func (s *Store) ListJobs(ctx context.Context, limit int) ([]model.BuildJob, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `select id, type, status, story_id, chapter_id, requested_preset, progress_current, progress_total, last_error, payload_json, started_at, finished_at, created_at, updated_at from build_jobs order by created_at desc limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.BuildJob
	for rows.Next() {
		item, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) UpsertTelegramAccount(ctx context.Context, account model.TelegramAccount) (model.TelegramAccount, error) {
	now := nowText()
	var existingID sql.NullInt64
	_ = s.db.QueryRowContext(ctx, `select id from telegram_accounts order by id asc limit 1`).Scan(&existingID)

	if existingID.Valid {
		account.ID = existingID.Int64
		_, err := s.db.ExecContext(ctx, `
			update telegram_accounts
			set phone = ?, session_file = ?, auth_state = ?, last_phone_code_hash = ?, last_error = ?, updated_at = ?
			where id = ?
		`, account.Phone, account.SessionFile, account.AuthState, account.LastPhoneCodeHash, account.LastError, now, account.ID)
		if err != nil {
			return model.TelegramAccount{}, err
		}
	} else {
		result, err := s.db.ExecContext(ctx, `
			insert into telegram_accounts (phone, session_file, auth_state, last_phone_code_hash, last_error, created_at, updated_at)
			values (?, ?, ?, ?, ?, ?, ?)
		`, account.Phone, account.SessionFile, account.AuthState, account.LastPhoneCodeHash, account.LastError, now, now)
		if err != nil {
			return model.TelegramAccount{}, err
		}
		account.ID, _ = result.LastInsertId()
	}

	return s.GetTelegramAccount(ctx)
}

func (s *Store) GetTelegramAccount(ctx context.Context) (model.TelegramAccount, error) {
	row := s.db.QueryRowContext(ctx, `select id, phone, session_file, auth_state, last_phone_code_hash, last_error, created_at, updated_at from telegram_accounts order by id asc limit 1`)
	return scanTelegramAccount(row)
}

func (s *Store) SaveBotProfile(ctx context.Context, profile model.TelegramBotProfile) (model.TelegramBotProfile, error) {
	now := nowText()
	if profile.ID == 0 {
		result, err := s.db.ExecContext(ctx, `
			insert into telegram_bot_profiles (name, bot_username, search_template, chapter_template, document_rule, story_title_rule, chapter_title_rule, enabled, created_at, updated_at)
			values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, profile.Name, profile.BotUsername, profile.SearchTemplate, profile.ChapterTemplate, profile.DocumentRule, profile.StoryTitleRule, profile.ChapterTitleRule, boolToInt(profile.Enabled), now, now)
		if err != nil {
			return model.TelegramBotProfile{}, err
		}
		profile.ID, _ = result.LastInsertId()
	} else {
		_, err := s.db.ExecContext(ctx, `
			update telegram_bot_profiles
			set name = ?, bot_username = ?, search_template = ?, chapter_template = ?, document_rule = ?, story_title_rule = ?, chapter_title_rule = ?, enabled = ?, updated_at = ?
			where id = ?
		`, profile.Name, profile.BotUsername, profile.SearchTemplate, profile.ChapterTemplate, profile.DocumentRule, profile.StoryTitleRule, profile.ChapterTitleRule, boolToInt(profile.Enabled), now, profile.ID)
		if err != nil {
			return model.TelegramBotProfile{}, err
		}
	}

	row := s.db.QueryRowContext(ctx, `select id, name, bot_username, search_template, chapter_template, document_rule, story_title_rule, chapter_title_rule, enabled, created_at, updated_at from telegram_bot_profiles where id = ?`, profile.ID)
	return scanBotProfile(row)
}

func (s *Store) ListBotProfiles(ctx context.Context) ([]model.TelegramBotProfile, error) {
	rows, err := s.db.QueryContext(ctx, `select id, name, bot_username, search_template, chapter_template, document_rule, story_title_rule, chapter_title_rule, enabled, created_at, updated_at from telegram_bot_profiles order by updated_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.TelegramBotProfile
	for rows.Next() {
		item, err := scanBotProfile(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) SaveReaderProgress(ctx context.Context, progress model.ReaderProgress) error {
	now := nowText()
	_, err := s.db.ExecContext(ctx, `
		insert into reader_progress (story_id, chapter_index, scroll_percent, audio_position_sec, updated_at)
		values (?, ?, ?, ?, ?)
		on conflict(story_id) do update set
			chapter_index = excluded.chapter_index,
			scroll_percent = excluded.scroll_percent,
			audio_position_sec = excluded.audio_position_sec,
			updated_at = excluded.updated_at
	`, progress.StoryID, progress.ChapterIndex, progress.ScrollPercent, progress.AudioPositionSec, now)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		insert into recent_stories (story_id, last_opened_at)
		values (?, ?)
		on conflict(story_id) do update set
			last_opened_at = excluded.last_opened_at
	`, progress.StoryID, now)
	return err
}

func (s *Store) GetReaderProgress(ctx context.Context, storyID int64) (model.ReaderProgress, error) {
	row := s.db.QueryRowContext(ctx, `
		select story_id, chapter_index, scroll_percent, audio_position_sec, updated_at
		from reader_progress
		where story_id = ?
	`, storyID)
	return scanReaderProgress(row)
}

func scanReaderProgress(scanner interface{ Scan(dest ...any) error }) (model.ReaderProgress, error) {
	var item model.ReaderProgress
	var updatedAt string
	if err := scanner.Scan(&item.StoryID, &item.ChapterIndex, &item.ScrollPercent, &item.AudioPositionSec, &updatedAt); err != nil {
		return model.ReaderProgress{}, err
	}
	parsed := mustParseTime(updatedAt)
	item.UpdatedAt = &parsed
	return item, nil
}

func scanStory(scanner interface{ Scan(dest ...any) error }) (model.Story, error) {
	var item model.Story
	var sourceType, preset, createdAt, updatedAt string
	var lastBuildJobID sql.NullInt64
	var lastOpenedAt sql.NullString
	if err := scanner.Scan(&item.ID, &item.Slug, &item.Title, &item.Author, &sourceType, &item.SourcePath, &item.LibraryPath, &preset, &lastBuildJobID, &item.LastError, &createdAt, &updatedAt, &item.ChapterCount, &lastOpenedAt); err != nil {
		return model.Story{}, err
	}
	item.SourceType = model.SourceType(sourceType)
	item.DefaultPreset = model.ProsodyPreset(preset)
	item.LastBuildJobID = nullInt64Ptr(lastBuildJobID)
	item.LastOpenedAt = nullTimePtr(lastOpenedAt)
	item.CreatedAt = mustParseTime(createdAt)
	item.UpdatedAt = mustParseTime(updatedAt)
	return item, nil
}

func scanChapter(scanner interface{ Scan(dest ...any) error }) (model.Chapter, error) {
	var item model.Chapter
	var preset, createdAt, updatedAt string
	var lastBuildJobID sql.NullInt64
	if err := scanner.Scan(&item.ID, &item.StoryID, &item.ChapterIndex, &item.Title, &item.SourceFilePath, &item.LibraryFilePath, &item.NormalizedText, &item.Checksum, &preset, &lastBuildJobID, &item.LastError, &createdAt, &updatedAt); err != nil {
		return model.Chapter{}, err
	}
	item.Preset = model.ProsodyPreset(preset)
	item.LastBuildJobID = nullInt64Ptr(lastBuildJobID)
	item.CreatedAt = mustParseTime(createdAt)
	item.UpdatedAt = mustParseTime(updatedAt)
	return item, nil
}

func scanSegment(scanner interface{ Scan(dest ...any) error }) (model.Segment, error) {
	var item model.Segment
	var status, createdAt, updatedAt string
	if err := scanner.Scan(&item.ID, &item.ChapterID, &item.SegmentIndex, &item.Text, &status, &item.AudioPath, &item.Error, &createdAt, &updatedAt); err != nil {
		return model.Segment{}, err
	}
	item.Status = model.SegmentStatus(status)
	item.CreatedAt = mustParseTime(createdAt)
	item.UpdatedAt = mustParseTime(updatedAt)
	return item, nil
}

func scanArtifact(scanner interface{ Scan(dest ...any) error }) (model.Artifact, error) {
	var item model.Artifact
	var chapterID sql.NullInt64
	var kind, createdAt, updatedAt string
	if err := scanner.Scan(&item.ID, &item.StoryID, &chapterID, &kind, &item.FilePath, &item.DurationMS, &item.Checksum, &createdAt, &updatedAt); err != nil {
		return model.Artifact{}, err
	}
	item.Kind = model.ArtifactKind(kind)
	item.ChapterID = nullInt64Ptr(chapterID)
	item.CreatedAt = mustParseTime(createdAt)
	item.UpdatedAt = mustParseTime(updatedAt)
	return item, nil
}

func scanJob(scanner interface{ Scan(dest ...any) error }) (model.BuildJob, error) {
	var item model.BuildJob
	var storyID, chapterID sql.NullInt64
	var jobType, status, createdAt, updatedAt string
	var startedAt, finishedAt sql.NullString
	if err := scanner.Scan(&item.ID, &jobType, &status, &storyID, &chapterID, &item.RequestedPreset, &item.ProgressCurrent, &item.ProgressTotal, &item.LastError, &item.PayloadJSON, &startedAt, &finishedAt, &createdAt, &updatedAt); err != nil {
		return model.BuildJob{}, err
	}
	item.Type = model.JobType(jobType)
	item.Status = model.JobStatus(status)
	item.StoryID = nullInt64Ptr(storyID)
	item.ChapterID = nullInt64Ptr(chapterID)
	item.StartedAt = nullTimePtr(startedAt)
	item.FinishedAt = nullTimePtr(finishedAt)
	item.CreatedAt = mustParseTime(createdAt)
	item.UpdatedAt = mustParseTime(updatedAt)
	return item, nil
}

func scanTelegramAccount(scanner interface{ Scan(dest ...any) error }) (model.TelegramAccount, error) {
	var item model.TelegramAccount
	var createdAt, updatedAt string
	if err := scanner.Scan(&item.ID, &item.Phone, &item.SessionFile, &item.AuthState, &item.LastPhoneCodeHash, &item.LastError, &createdAt, &updatedAt); err != nil {
		return model.TelegramAccount{}, err
	}
	item.CreatedAt = mustParseTime(createdAt)
	item.UpdatedAt = mustParseTime(updatedAt)
	return item, nil
}

func scanBotProfile(scanner interface{ Scan(dest ...any) error }) (model.TelegramBotProfile, error) {
	var item model.TelegramBotProfile
	var enabled int
	var createdAt, updatedAt string
	if err := scanner.Scan(&item.ID, &item.Name, &item.BotUsername, &item.SearchTemplate, &item.ChapterTemplate, &item.DocumentRule, &item.StoryTitleRule, &item.ChapterTitleRule, &enabled, &createdAt, &updatedAt); err != nil {
		return model.TelegramBotProfile{}, err
	}
	item.Enabled = enabled == 1
	item.CreatedAt = mustParseTime(createdAt)
	item.UpdatedAt = mustParseTime(updatedAt)
	return item, nil
}

func nowText() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func mustParseTime(raw string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func nullInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}

func nullTimePtr(value sql.NullString) *time.Time {
	if !value.Valid {
		return nil
	}
	parsed := mustParseTime(value.String)
	return &parsed
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
