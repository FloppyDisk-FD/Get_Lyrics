//go:build windows

package main

import "embed"

//go:embed bin/ffmpeg.exe
var ffmpegFS embed.FS

func ffmpegEmbedPath() string {
	return "bin/ffmpeg.exe"
}
