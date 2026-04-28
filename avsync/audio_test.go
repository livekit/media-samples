package avsync

import (
	"testing"
	"time"
)

func TestAnalyzeAudio(t *testing.T) {
	cfg := Config{
		FilePath:     "../livekit_avsync_p0_audio_523hz_48k.ogg",
		Participants: AllParticipants,
		Timeout:      30 * time.Second,
	}

	result, err := analyzeAudio(cfg)
	if err != nil {
		t.Fatalf("analyzeAudio failed: %v", err)
	}

	p0Count, p1Count, p2Count := 0, 0, 0
	for _, b := range result.Beeps {
		switch b.Participant {
		case "p0":
			p0Count++
		case "p1":
			p1Count++
		case "p2":
			p2Count++
		}
	}

	if p0Count < 115 || p0Count > 125 {
		t.Errorf("expected ~120 p0 beeps, got %d", p0Count)
	}
	if p1Count > 5 {
		t.Errorf("expected ~0 p1 beeps, got %d", p1Count)
	}
	if p2Count > 5 {
		t.Errorf("expected ~0 p2 beeps, got %d", p2Count)
	}

	var lastP0 time.Duration
	for _, b := range result.Beeps {
		if b.Participant == "p0" {
			if lastP0 > 0 {
				gap := b.PTS - lastP0
				if gap < 900*time.Millisecond || gap > 1100*time.Millisecond {
					t.Errorf("p0 beep gap: %s (expected ~1s)", gap)
				}
			}
			lastP0 = b.PTS
		}
	}
}

func TestAnalyzeAudioSilence(t *testing.T) {
	cfg := Config{
		FilePath:     "../livekit_avsync_p0_audio_523hz_48k.ogg",
		Participants: []Participant{P0},
		Timeout:      30 * time.Second,
	}

	result, err := analyzeAudio(cfg)
	if err != nil {
		t.Fatalf("analyzeAudio failed: %v", err)
	}

	for _, sr := range result.Silence {
		if sr.Duration > 2*time.Second {
			t.Errorf("unexpected long silence: %s-%s (%s)", sr.Start, sr.End, sr.Duration)
		}
	}
}
