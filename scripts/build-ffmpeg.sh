#!/usr/bin/env bash
set -euo pipefail

FFMPEG_VERSION="7.1"
SRC_DIR="/tmp/ffmpeg-src"
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="${PROJECT_ROOT}/bin"

BUILD_JOBS=$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)

EXE_NAME="ffmpeg"
if [[ "$OSTYPE" == "msys"* || "$OSTYPE" == "cygwin"* ]]; then
  EXE_NAME="ffmpeg.exe"
fi

echo "==> FFmpeg minimal build for metadata operations only"
echo "    Version: ${FFMPEG_VERSION}"
echo "    Output:  ${BIN_DIR}/${EXE_NAME}"
echo "    Jobs:    ${BUILD_JOBS}"

mkdir -p "${BIN_DIR}"
rm -rf "${SRC_DIR}"
mkdir -p "${SRC_DIR}"

echo ""
echo "==> Downloading FFmpeg ${FFMPEG_VERSION}..."
curl -fsSL "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz" \
  | tar -xJ -C "${SRC_DIR}" --strip-components=1

cd "${SRC_DIR}"

echo ""
echo "==> Configuring FFmpeg (minimal build)..."

EXTRA_CFLAGS=""
EXTRA_LDFLAGS=""

# Windows: 静态链接 MSVC 运行库，避免用户系统缺少 vcruntime.dll 导致 0xc0000135 错误
if [[ "$OSTYPE" == "msys"* || "$OSTYPE" == "cygwin"* ]]; then
  EXTRA_CFLAGS="-MT"
  EXTRA_LDFLAGS="-static"
fi

./configure \
  --disable-doc \
  --disable-network \
  --disable-iconv \
  --disable-sdl2 \
  --disable-xlib \
  --disable-zlib \
  --disable-bzlib \
  --disable-lzma \
  --disable-avdevice \
  --disable-postproc \
  --disable-swresample \
  --disable-swscale \
  --disable-ffplay \
  --disable-ffprobe \
  --disable-encoders \
  --disable-decoders \
  --disable-muxers \
  --disable-demuxers \
  --disable-parsers \
  --disable-bsfs \
  --disable-indevs \
  --disable-outdevs \
  --disable-filters \
  --disable-devices \
  --disable-hwaccels \
  --enable-protocol=file \
  --enable-protocol=pipe \
  --enable-muxer=ffmetadata \
  --enable-demuxer=ffmetadata \
  --enable-muxer=mp3 \
  --enable-demuxer=mp3 \
  --enable-muxer=mp4 \
  --enable-demuxer=mov \
  --enable-muxer=flac \
  --enable-demuxer=flac \
  --enable-muxer=oga \
  --enable-demuxer=ogg \
  --enable-muxer=opus \
  --enable-muxer=wav \
  --enable-demuxer=wav \
  --enable-muxer=aiff \
  --enable-demuxer=aiff \
  --enable-muxer=wv \
  --enable-demuxer=wv \
  --extra-cflags="$EXTRA_CFLAGS" \
  --extra-ldflags="$EXTRA_LDFLAGS" \
  2>&1 | tail -50

echo ""
echo "==> Building FFmpeg..."
make -j"${BUILD_JOBS}" 2>&1 | tail -20

echo ""
echo "==> Copying binary to ${BIN_DIR}..."
cp "ffmpeg" "${BIN_DIR}/${EXE_NAME}"

if command -v strip &>/dev/null; then
  echo "==> Stripping symbols..."
  strip "${BIN_DIR}/${EXE_NAME}" || true
fi

echo ""
echo "==> Build complete!"
ls -lh "${BIN_DIR}/${EXE_NAME}"

echo ""
echo "==> Verifying..."
"${BIN_DIR}/${EXE_NAME}" -version | head -3
