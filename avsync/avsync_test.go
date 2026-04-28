package avsync

import (
	"image"
	"testing"
	"time"
)

func TestAnalyze(t *testing.T) {
	result, err := Analyze(Config{
		FilePath: "../livekit_avsync_p0_video_white_1080p25.h264",
		Regions: []Region{
			{Name: "full", Rect: image.Rect(0, 0, 1920, 1080)},
		},
		Participants: AllParticipants,
		Timeout:      60 * time.Second,
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	frames := result.Video.Regions["full"]
	if len(frames) == 0 {
		t.Fatal("no video frames")
	}

	flashes := result.Video.Flashes["full"]
	if len(flashes) < 115 {
		t.Errorf("expected ~120 flashes, got %d", len(flashes))
	}

	for i, f := range frames {
		if f.Participant != "p0" {
			t.Errorf("frame %d: got %q, want p0", i, f.Participant)
			break
		}
	}

	// Video-only file — audio may fail gracefully
	if len(result.Audio.Beeps) > 0 {
		t.Logf("note: %d beeps detected in video-only file", len(result.Audio.Beeps))
	}
}

func TestAnalyzeAudioOnly(t *testing.T) {
	result, err := Analyze(Config{
		FilePath:     "../livekit_avsync_p0_audio_523hz_48k.ogg",
		Participants: AllParticipants,
		Timeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(result.Video.Regions) > 0 {
		t.Error("expected no video regions for audio-only file")
	}

	p0Count := 0
	for _, b := range result.Audio.Beeps {
		if b.Participant == "p0" {
			p0Count++
		}
	}
	if p0Count < 115 {
		t.Errorf("expected ~120 p0 beeps, got %d", p0Count)
	}
}
