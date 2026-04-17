package audio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Merger interface {
	MergeMP3(ctx context.Context, inputs []string, outputPath string) error
}

type FFmpegMerger struct {
	binaryPath string
}

func NewFFmpegMerger(binaryPath string) *FFmpegMerger {
	return &FFmpegMerger{binaryPath: binaryPath}
}

func (m *FFmpegMerger) MergeMP3(ctx context.Context, inputs []string, outputPath string) error {
	if len(inputs) == 0 {
		return fmt.Errorf("khong co file nao de merge")
	}
	absoluteOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return err
	}
	outputPath = absoluteOutput
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}

	listFile, err := os.CreateTemp(filepath.Dir(outputPath), "concat-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(listFile.Name())

	for _, input := range inputs {
		absoluteInput, err := filepath.Abs(input)
		if err != nil {
			_ = listFile.Close()
			return err
		}
		line := fmt.Sprintf("file '%s'\n", strings.ReplaceAll(filepath.ToSlash(absoluteInput), "'", "'\\''"))
		if _, err := listFile.WriteString(line); err != nil {
			_ = listFile.Close()
			return err
		}
	}
	if err := listFile.Close(); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, m.binaryPath,
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile.Name(),
		"-c", "copy",
		outputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg merge that bai: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
