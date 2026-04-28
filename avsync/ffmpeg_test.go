package avsync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunFFmpeg(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "test.wav")
	err := runFFmpeg(runFFmpegArgs{
		args:    []string{"-f", "lavfi", "-t", "0.1", "-i", "anullsrc=r=8000:cl=mono", "-f", "wav", outFile},
		timeout: 0,
	})
	if err != nil {
		t.Fatalf("runFFmpeg failed: %v", err)
	}
	info, err := os.Stat(outFile)
	if err != nil {
		t.Fatalf("output file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("output file is empty")
	}
}

func TestRunFFmpegFailure(t *testing.T) {
	err := runFFmpeg(runFFmpegArgs{
		args:    []string{"-i", "nonexistent_file_12345.wav", "-f", "null", "-"},
		timeout: 0,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent input")
	}
}
