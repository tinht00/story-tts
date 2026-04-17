package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"story-tts/backend/internal/config"
)

type EdgeProvider struct {
	binaryPath   string
	defaultVoice string
	outputFormat string
}

func NewEdgeProvider(cfg config.EdgeConfig) *EdgeProvider {
	return &EdgeProvider{
		binaryPath:   cfg.BinaryPath,
		defaultVoice: cfg.DefaultVoice,
		outputFormat: cfg.OutputFormat,
	}
}

func (p *EdgeProvider) Name() string {
	return "edge-tts-cli"
}

func (p *EdgeProvider) Synthesize(ctx context.Context, input SynthesizeInput) error {
	if strings.TrimSpace(input.Text) == "" {
		return errors.New("doan van synthesize dang rong")
	}
	if err := os.MkdirAll(filepath.Dir(input.OutputPath), 0o755); err != nil {
		return err
	}
	textFile, err := os.CreateTemp("", "story-tts-edge-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(textFile.Name())
	if _, err := textFile.WriteString(input.Text); err != nil {
		textFile.Close()
		return err
	}
	if err := textFile.Close(); err != nil {
		return err
	}

	preset := ResolveEdgePreset(input.Preset)
	voice := input.Voice
	if voice == "" {
		voice = p.defaultVoice
	}

	args := []string{
		"--voice", voice,
		"--file", textFile.Name(),
		"--write-media", input.OutputPath,
	}
	if preset.Rate != "" {
		args = append(args, fmt.Sprintf("--rate=%s", preset.Rate))
	}
	if preset.Pitch != "" {
		args = append(args, fmt.Sprintf("--pitch=%s", preset.Pitch))
	}
	if preset.Volume != "" {
		args = append(args, fmt.Sprintf("--volume=%s", preset.Volume))
	}

	cmd := exec.CommandContext(ctx, p.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("edge-tts that bai: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
