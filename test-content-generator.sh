#!/bin/bash

# Copyright 2025 LiveKit, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

DURATION=120

# ffmpeg with drawtext support (macOS: ffmpeg@7 from Homebrew, Linux: system ffmpeg)
if [ -x "/opt/homebrew/opt/ffmpeg@7/bin/ffmpeg" ]; then
    FFMPEG="/opt/homebrew/opt/ffmpeg@7/bin/ffmpeg"
elif command -v ffmpeg &>/dev/null; then
    FFMPEG="ffmpeg"
else
    echo "Error: ffmpeg not found" >&2
    exit 1
fi

# Verify drawtext filter is available
FILTER_LIST=$("${FFMPEG}" -filters 2>/dev/null || true)
if ! echo "${FILTER_LIST}" | grep -q drawtext; then
    echo "Error: ffmpeg drawtext filter not available (need freetype support)" >&2
    echo "On macOS: brew install ffmpeg@7" >&2
    exit 1
fi

# Font detection (macOS primary, Linux fallback)
if [ -f "/Library/Fonts/Arial.ttf" ]; then
    FONT="/Library/Fonts/Arial.ttf"
elif [ -f "/Library/Fonts/Arial Unicode.ttf" ]; then
    FONT="/Library/Fonts/Arial Unicode.ttf"
elif [ -f "/System/Library/Fonts/Helvetica.ttc" ]; then
    FONT="/System/Library/Fonts/Helvetica.ttc"
elif [ -f "/usr/share/fonts/truetype/freefont/FreeSans.ttf" ]; then
    FONT="/usr/share/fonts/truetype/freefont/FreeSans.ttf"
elif [ -f "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf" ]; then
    FONT="/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
else
    echo "Error: no suitable font found" >&2
    exit 1
fi

# ─────────────────────────────────────────────────────────────────────
# Per-participant configuration
#
# Audio: beep frequencies form a C major chord (C-E-G) when mixed.
#        Fixed frequencies (no sweep) for bandpass isolation in tests.
# Video: distinct text colors for visual layout identification.
#        Flash sync marker is always white for reliable detection.
# ─────────────────────────────────────────────────────────────────────

NAMES=(      p0    p1     p2     )
LABELS=(     P0    P1     P2     )
BEEP_FREQS=( 523   659    784    )  # C5, E5, G5
BG_FREQS=(   262   330    392    )  # C4, E4, G4
TEXT_COLORS=( white cyan   yellow )

# ─────────────────────────────────────────────────────────────────────
# Video generators (per-participant)
# ─────────────────────────────────────────────────────────────────────

generate_h264() {
    local name=$1 label=$2 color=$3
    local outfile="livekit_avsync_${name}_video_${color}_1080p25.h264"
    echo "  ${outfile}"
    "${FFMPEG}" -y \
      -f lavfi -r 25 -t ${DURATION} -i "color=black:size=1920x1080:rate=25" \
      -loop 1 -t ${DURATION} -i livekit-logo.png \
      -filter_complex "\
[0:v]drawtext=fontfile=${FONT}:text='${label}':x=20:y=16:fontsize=36:fontcolor=${color}:box=1:boxcolor=0x00000099,\
drawtext=fontfile=${FONT}:text='%{eif\\:t\\:d}':x=(w-tw)/2:y=(h-th)/2:fontsize=220:fontcolor=${color}:box=1:boxcolor=0x000000bb,\
drawtext=fontfile=${FONT}:text='%{pts\\:hms}':x=w-tw-20:y=20:fontsize=28:fontcolor=${color}:box=1:boxcolor=0x00000099,\
drawbox=enable='lt(mod(t,1),0.05)':x=0:y=0:w=iw:h=90:color=white@0.85:t=fill[basev];\
[1:v]scale=-1:100[logo];\
[basev][logo]overlay=shortest=1:x=20:y=main_h-overlay_h-20,format=yuv420p[v]" \
      -map "[v]" -an \
      -c:v libx264 -pix_fmt yuv420p -profile:v baseline -level 4.0 -bf 0 \
      -colorspace bt2020nc -color_primaries bt2020 -color_trc smpte2084 -color_range tv \
      -x264-params "keyint=50:min-keyint=50:scenecut=0:ref=1:8x8dct=0:cabac=0:weightp=0" \
      -f h264 "${outfile}"
}

generate_vp8() {
    local name=$1 label=$2 color=$3
    local outfile="livekit_avsync_${name}_video_${color}_1080p24.vp8.ivf"
    echo "  ${outfile}"
    "${FFMPEG}" -y \
      -f lavfi -r 24 -t ${DURATION} -i "color=black:size=1920x1080:rate=24" \
      -loop 1 -t ${DURATION} -i livekit-logo.png \
      -filter_complex "\
[0:v]drawtext=fontfile=${FONT}:text='${label}':x=20:y=16:fontsize=36:fontcolor=${color}:box=1:boxcolor=0x00000099,\
drawtext=fontfile=${FONT}:text='%{eif\\:t\\:d}':x=(w-tw)/2:y=(h-th)/2:fontsize=220:fontcolor=${color}:box=1:boxcolor=0x000000bb,\
drawtext=fontfile=${FONT}:text='%{pts\\:hms}':x=w-tw-20:y=20:fontsize=28:fontcolor=${color}:box=1:boxcolor=0x00000099,\
drawbox=enable='lt(mod(t,1),0.05)':x=0:y=0:w=iw:h=90:color=white@0.85:t=fill,format=yuv420p[basev];\
[1:v]scale=-1:100[logo];\
[basev][logo]overlay=shortest=1:x=20:y=main_h-overlay_h-20[v]" \
      -map "[v]" -an \
      -c:v libvpx -pix_fmt yuv420p -r 24 -g 48 -deadline good -speed 4 -threads 4 \
      -f ivf "${outfile}"
}

generate_vp9() {
    local name=$1 label=$2 color=$3
    local outfile="livekit_avsync_${name}_video_${color}_1080p24.vp9.ivf"
    echo "  ${outfile}"
    "${FFMPEG}" -y \
      -f lavfi -r 24 -t ${DURATION} -i "color=black:size=1920x1080:rate=24" \
      -loop 1 -t ${DURATION} -i livekit-logo.png \
      -filter_complex "\
[0:v]drawtext=fontfile=${FONT}:text='${label}':x=20:y=16:fontsize=36:fontcolor=${color}:box=1:boxcolor=0x00000099,\
drawtext=fontfile=${FONT}:text='%{eif\\:t\\:d}':x=(w-tw)/2:y=(h-th)/2:fontsize=220:fontcolor=${color}:box=1:boxcolor=0x000000bb,\
drawtext=fontfile=${FONT}:text='%{pts\\:hms}':x=w-tw-20:y=20:fontsize=28:fontcolor=${color}:box=1:boxcolor=0x00000099,\
drawbox=enable='lt(mod(t,1),0.05)':x=0:y=0:w=iw:h=90:color=white@0.85:t=fill,format=yuv444p12le[basev];\
[1:v]scale=-1:100[logo];\
[basev][logo]overlay=shortest=1:x=20:y=main_h-overlay_h-20[v]" \
      -map "[v]" -an \
      -c:v libvpx-vp9 -pix_fmt yuv444p12le -profile:v 3 -r 24 -g 48 -row-mt 1 -deadline good -threads 4 \
      -colorspace bt2020nc -color_primaries bt2020 -color_trc smpte2084 -color_range tv \
      -f ivf "${outfile}"
}

# ─────────────────────────────────────────────────────────────────────
# Audio generators (per-participant)
# ─────────────────────────────────────────────────────────────────────

# Beep expression: 40ms Hann-windowed tone at the start of each second.
# 40ms spans at least one full MP3 frame (~26ms at 44.1kHz, 1152 samples), so
# the tone survives MP3's per-frame perceptual masking. Shorter beeps got
# dropped by lossy codecs.
beep_expr() {
    local freq=$1
    echo "0.12*if(lt(mod(t,1),0.04),(1-cos(2*PI*mod(t,1)/0.04))/2*sin(2*PI*${freq}*t),0)"
}

# Background expression: continuous low-amplitude tone
bg_expr() {
    local freq=$1
    echo "0.02*sin(2*PI*${freq}*t)"
}

generate_opus() {
    local name=$1 beep_freq=$2 bg_freq=$3
    local outfile="livekit_avsync_${name}_audio_${beep_freq}hz_48k.ogg"
    local beep; beep=$(beep_expr "${beep_freq}")
    local bg; bg=$(bg_expr "${bg_freq}")
    echo "  ${outfile}"
    "${FFMPEG}" -y \
      -f lavfi -t ${DURATION} \
      -i "aevalsrc=exprs='${beep} | ${beep}':s=48000:channel_layout=stereo" \
      -f lavfi -t ${DURATION} \
      -i "aevalsrc=exprs='${bg} | ${bg}':s=48000:channel_layout=stereo" \
      -filter_complex "[0:a][1:a]amix=inputs=2:normalize=0, aformat=channel_layouts=stereo:sample_rates=48000[a]" \
      -map "[a]" -c:a libopus -b:a 128k -ar 48000 -ac 2 \
      "${outfile}"
}

generate_wav() {
    local name=$1 beep_freq=$2 bg_freq=$3
    local outfile="livekit_avsync_${name}_audio_${beep_freq}hz_48k.wav"
    local beep; beep=$(beep_expr "${beep_freq}")
    local bg; bg=$(bg_expr "${bg_freq}")
    echo "  ${outfile}"
    "${FFMPEG}" -y \
      -f lavfi -t ${DURATION} \
      -i "aevalsrc=exprs='${beep} | ${beep}':s=48000:channel_layout=stereo" \
      -f lavfi -t ${DURATION} \
      -i "aevalsrc=exprs='${bg} | ${bg}':s=48000:channel_layout=stereo" \
      -filter_complex "[0:a][1:a]amix=inputs=2:normalize=0, aformat=sample_fmts=s16:channel_layouts=stereo:sample_rates=48000[a]" \
      -map "[a]" -c:a pcm_s16le \
      "${outfile}"
}

generate_pcmu() {
    local name=$1 beep_freq=$2 bg_freq=$3
    local outfile="livekit_avsync_${name}_audio_${beep_freq}hz_8k.pcmu.wav"
    local beep; beep=$(beep_expr "${beep_freq}")
    local bg; bg=$(bg_expr "${bg_freq}")
    echo "  ${outfile}"
    "${FFMPEG}" -y \
      -f lavfi -t ${DURATION} \
      -i "aevalsrc=exprs='${beep}':s=8000:channel_layout=mono" \
      -f lavfi -t ${DURATION} \
      -i "aevalsrc=exprs='${bg}':s=8000:channel_layout=mono" \
      -filter_complex "[0:a][1:a]amix=inputs=2:normalize=0, aformat=channel_layouts=mono:sample_rates=8000[a]" \
      -map "[a]" -c:a pcm_mulaw -ar 8000 -ac 1 \
      "${outfile}"
}

generate_pcma() {
    local name=$1 beep_freq=$2 bg_freq=$3
    local outfile="livekit_avsync_${name}_audio_${beep_freq}hz_8k.pcma.wav"
    local beep; beep=$(beep_expr "${beep_freq}")
    local bg; bg=$(bg_expr "${bg_freq}")
    echo "  ${outfile}"
    "${FFMPEG}" -y \
      -f lavfi -t ${DURATION} \
      -i "aevalsrc=exprs='${beep}':s=8000:channel_layout=mono" \
      -f lavfi -t ${DURATION} \
      -i "aevalsrc=exprs='${bg}':s=8000:channel_layout=mono" \
      -filter_complex "[0:a][1:a]amix=inputs=2:normalize=0, aformat=channel_layouts=mono:sample_rates=8000[a]" \
      -map "[a]" -c:a pcm_alaw -ar 8000 -ac 1 \
      "${outfile}"
}

# =====================================================================
# Generate per-participant samples
# =====================================================================

echo ""
echo "=== Generating per-participant samples ==="
for i in "${!NAMES[@]}"; do
    echo ""
    echo "--- ${LABELS[$i]} (${TEXT_COLORS[$i]}, beep=${BEEP_FREQS[$i]}Hz, bg=${BG_FREQS[$i]}Hz) ---"
    generate_h264  "${NAMES[$i]}" "${LABELS[$i]}" "${TEXT_COLORS[$i]}"
    generate_opus  "${NAMES[$i]}" "${BEEP_FREQS[$i]}" "${BG_FREQS[$i]}"
    if [ "${NAMES[$i]}" = "p0" ]; then
        generate_vp8   "${NAMES[$i]}" "${LABELS[$i]}" "${TEXT_COLORS[$i]}"
        generate_vp9   "${NAMES[$i]}" "${LABELS[$i]}" "${TEXT_COLORS[$i]}"
        generate_wav   "${NAMES[$i]}" "${BEEP_FREQS[$i]}" "${BG_FREQS[$i]}"
        generate_pcmu  "${NAMES[$i]}" "${BEEP_FREQS[$i]}" "${BG_FREQS[$i]}"
        generate_pcma  "${NAMES[$i]}" "${BEEP_FREQS[$i]}" "${BG_FREQS[$i]}"
    fi
done

echo ""
echo "=== Done ==="
