package provider

import "story-tts/backend/internal/model"

type EdgePreset struct {
	Rate   string
	Pitch  string
	Volume string
}

func ResolveEdgePreset(preset model.ProsodyPreset) EdgePreset {
	switch preset {
	case model.PresetGentle:
		return EdgePreset{Pitch: "-2Hz", Volume: "+0%"}
	case model.PresetTense:
		return EdgePreset{Pitch: "+4Hz", Volume: "+3%"}
	case model.PresetClimax:
		return EdgePreset{Pitch: "+8Hz", Volume: "+6%"}
	default:
		return EdgePreset{}
	}
}
