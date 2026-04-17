package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr   string
	ProjectDir   string
	DataDir      string
	LibraryDir   string
	DBPath       string
	FFmpegPath   string
	Edge         EdgeConfig
	RealtimeTTS  RealtimeTTSConfig
	Telegram     TelegramConfig
}

type EdgeConfig struct {
	BinaryPath   string
	DefaultVoice string
	OutputFormat string
}

type RealtimeTTSConfig struct {
	BaseURL      string
	DefaultVoice string
	DefaultSpeed int
	DefaultPitch int
}

type TelegramConfig struct {
	AppID       int
	AppHash     string
	SessionFile string
	DeviceModel string
	SystemVer   string
	AppVersion  string
	LangCode    string
}

func Load() (Config, error) {
	projectDir, err := resolveProjectDir()
	if err != nil {
		return Config{}, err
	}
	loadEnvFiles(
		filepath.Join(projectDir, "backend", ".env"),
		filepath.Join(projectDir, ".env"),
	)

	dataDir := envOrDefault("STORY_TTS_DATA_DIR", filepath.Join(projectDir, "data"))
	libraryDir := envOrDefault("STORY_TTS_LIBRARY_DIR", filepath.Join(projectDir, "library"))
	dbPath := envOrDefault("STORY_TTS_DB_PATH", filepath.Join(dataDir, "story_tts.sqlite"))
	telegramSession := envOrDefault("STORY_TTS_TELEGRAM_SESSION", filepath.Join(dataDir, "telegram", "session.json"))

	cfg := Config{
		ListenAddr: envOrDefault("STORY_TTS_LISTEN_ADDR", ":18080"),
		ProjectDir: projectDir,
		DataDir:    dataDir,
		LibraryDir: libraryDir,
		DBPath:     dbPath,
		FFmpegPath: envOrDefault("STORY_TTS_FFMPEG_PATH", "ffmpeg"),
		Edge: EdgeConfig{
			BinaryPath:   envOrDefault("STORY_TTS_EDGE_BINARY", "edge-tts"),
			DefaultVoice: envOrDefault("STORY_TTS_EDGE_VOICE", "vi-VN-NamMinhNeural"),
			OutputFormat: envOrDefault("STORY_TTS_EDGE_OUTPUT_FORMAT", "audio-24khz-48kbitrate-mono-mp3"),
		},
		RealtimeTTS: RealtimeTTSConfig{
			BaseURL:      envOrDefault("STORY_TTS_REALTIME_TTS_BASE_URL", "http://127.0.0.1:8010"),
			DefaultVoice: envOrDefault("STORY_TTS_REALTIME_TTS_VOICE", envOrDefault("STORY_TTS_EDGE_VOICE", "vi-VN-NamMinhNeural")),
			DefaultSpeed: envIntOrDefault("STORY_TTS_REALTIME_TTS_SPEED", 0),
			DefaultPitch: envIntOrDefault("STORY_TTS_REALTIME_TTS_PITCH", 0),
		},
		Telegram: TelegramConfig{
			AppID:       envInt("STORY_TTS_TELEGRAM_APP_ID"),
			AppHash:     os.Getenv("STORY_TTS_TELEGRAM_APP_HASH"),
			SessionFile: telegramSession,
			DeviceModel: envOrDefault("STORY_TTS_TELEGRAM_DEVICE_MODEL", "story-tts"),
			SystemVer:   envOrDefault("STORY_TTS_TELEGRAM_SYSTEM_VERSION", "Windows"),
			AppVersion:  envOrDefault("STORY_TTS_TELEGRAM_APP_VERSION", "0.1.0"),
			LangCode:    envOrDefault("STORY_TTS_TELEGRAM_LANG_CODE", "vi"),
		},
	}

	for _, dir := range []string{cfg.DataDir, cfg.LibraryDir, filepath.Dir(cfg.Telegram.SessionFile)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Config{}, err
		}
	}

	return cfg, nil
}

func resolveProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if filepath.Base(wd) == "backend" {
		return filepath.Dir(wd), nil
	}
	if filepath.Base(filepath.Dir(wd)) == "backend" {
		return filepath.Dir(filepath.Dir(wd)), nil
	}
	if _, err := os.Stat(filepath.Join(wd, "backend")); err == nil {
		return wd, nil
	}
	return "", errors.New("khong xac dinh duoc thu muc goc cua project story-tts")
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string) int {
	value := os.Getenv(key)
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func envIntOrDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func loadEnvFiles(paths ...string) {
	for _, path := range paths {
		loadEnvFile(path)
	}
}

func loadEnvFile(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		_ = os.Setenv(key, value)
	}
}
