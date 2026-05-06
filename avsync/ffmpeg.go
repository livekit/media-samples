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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

const (
	defaultFFmpegPath = "ffmpeg"
	defaultTimeout    = 5 * time.Minute
)

type runFFmpegArgs struct {
	args    []string
	timeout time.Duration // 0 = 60s default
}

func runFFmpeg(r runFFmpegArgs) ([]byte, error) {
	timeout := r.timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpegPath(), append([]string{"-hide_banner", "-nostats", "-loglevel", "repeat+info"}, r.args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("ffmpeg timed out after %s", timeout)
		}
		return nil, fmt.Errorf("ffmpeg failed: %w\nstderr:\n%s", err, stderr.String())
	}
	return stderr.Bytes(), nil
}

func ffmpegPath() string {
	if path, err := exec.LookPath("/opt/homebrew/opt/ffmpeg@7/bin/ffmpeg"); err == nil {
		return path
	}
	return defaultFFmpegPath
}
