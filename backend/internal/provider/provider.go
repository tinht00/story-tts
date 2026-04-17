package provider

import (
	"context"

	"story-tts/backend/internal/model"
)

type Provider interface {
	Name() string
	Synthesize(ctx context.Context, input SynthesizeInput) error
}

type SynthesizeInput struct {
	Text       string
	OutputPath string
	Voice      string
	Preset     model.ProsodyPreset
}
