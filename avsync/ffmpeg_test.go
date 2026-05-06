// Copyright 2026 LiveKit, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package avsync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunFFmpeg(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "test.wav")
	_, err := runFFmpeg(runFFmpegArgs{
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
	_, err := runFFmpeg(runFFmpegArgs{
		args:    []string{"-i", "nonexistent_file_12345.wav", "-f", "null", "-"},
		timeout: 0,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent input")
	}
}
