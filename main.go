package main

import (
	"bufio"
	"crypto/md5"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed bin/ffmpeg
var ffmpegFS embed.FS

const (
	apiURL          = "https://service-47o75c8f-1301683732.sh.apigw.tencentcs.com/release/lyric"
	outputFile      = "./lyric.txt"
	lrcErrorSig     = "ºw^~)Þ"
	ffmpegVersion   = "7.1-minimal"
)

type LyricResponse struct {
	Data struct {
		Lyric string `json:"lyric"`
		Trans string `json:"trans"`
	} `json:"data"`
}

func ffmpegName() string {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
}

func extractEmbeddedFfmpeg() (string, error) {
	name := ffmpegName()
	hash := md5.Sum([]byte(ffmpegVersion + runtime.GOOS))
	hashStr := hex.EncodeToString(hash[:])[:8]
	extractDir := filepath.Join(os.TempDir(), "get-lyrics-ffmpeg-"+hashStr)
	extractPath := filepath.Join(extractDir, name)

	if _, err := os.Stat(extractPath); err == nil {
		return extractPath, nil
	}

	data, err := ffmpegFS.ReadFile("bin/ffmpeg")
	if err != nil {
		return "", fmt.Errorf("read embedded ffmpeg: %w", err)
	}

	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return "", fmt.Errorf("create extract dir: %w", err)
	}

	if err := os.WriteFile(extractPath, data, 0o755); err != nil {
		return "", fmt.Errorf("write ffmpeg: %w", err)
	}

	return extractPath, nil
}

func resolveFfmpegPath() string {
	name := ffmpegName()

	candidates := []string{
		os.Getenv("FFMPEG_PATH"),
		filepath.Join("bin", name),
	}

	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, "bin", name))
	}

	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	if path, err := extractEmbeddedFfmpeg(); err == nil {
		return path
	}

	return "ffmpeg"
}

func fetchLyric(songmid string) (*LyricResponse, error) {
	url := apiURL + "?songmid=" + songmid
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("request failed, status: %d", resp.StatusCode)
	}

	var result LyricResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &result, nil
}

func saveLyricToFile(lyric, trans string) error {
	if lyric == lrcErrorSig {
		fmt.Println("请求失败，songmid可能存在错误！")
		return nil
	}

	content := lyric
	if err := os.WriteFile(outputFile, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write lyric: %w", err)
	}
	fmt.Println("歌词写入成功! \n歌词在本目录下lyric.txt内")

	if trans == "" {
		fmt.Println("本歌曲不存在翻译！")
	} else {
		f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("open lyric for append: %w", err)
		}
		defer f.Close()
		if _, err := f.WriteString(trans); err != nil {
			return fmt.Errorf("append trans: %w", err)
		}
		fmt.Println("翻译写入成功! ")
	}

	return nil
}

func embedLyric(ffmpegPath, filename, lyric, trans string) error {
	ext := strings.ToLower(filepath.Ext(filename))
	isMp3 := ext == ".mp3"

	tmpOut := filepath.Join(filepath.Dir(filename), fmt.Sprintf(".get-lyrics-tmp-%d%s", os.Getpid(), ext))
	defer os.Remove(tmpOut)

	args := []string{"-y", "-i", filename}

	if isMp3 {
		args = append(args,
			"-metadata", "lyrics="+lyric+trans,
			"-metadata", "lyric="+lyric+trans,
			"-id3v2_version", "3",
			"-write_id3v1", "1",
		)
	} else {
		args = append(args,
			"-metadata", "lyrics="+lyric+trans,
		)
	}

	args = append(args, "-c", "copy", "-map", "0", tmpOut)

	cmd := exec.Command(ffmpegPath, args...)
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w", err)
	}

	src, err := os.Open(tmpOut)
	if err != nil {
		return fmt.Errorf("open tmp: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}

	fmt.Println("歌词嵌入成功! ")
	return nil
}

func promptString(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func promptConfirm(reader *bufio.Reader, prompt string) bool {
	fmt.Print(prompt + " (y/n): ")
	line, _ := reader.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

func main() {
	songmid := flag.String("s", "", "歌曲 songmid")
	embed := flag.Bool("e", false, "嵌入歌词到歌曲文件")
	filename := flag.String("f", "", "歌曲文件路径（嵌入模式下使用）")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "get-lyrics - 获取QQ音乐歌词的工具\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  get-lyrics [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	reader := bufio.NewReader(os.Stdin)

	hasCliArgs := flag.NFlag() > 0

	if *songmid == "" {
		*songmid = promptString(reader, "Input the songmid: ")
	}

	embedFlag := *embed
	if !hasCliArgs {
		embedFlag = promptConfirm(reader, "Whether to embed lyrics to a song or not")
	}

	if *songmid == "" {
		fmt.Fprintln(os.Stderr, "错误: songmid 不能为空")
		os.Exit(1)
	}

	lrc, err := fetchLyric(*songmid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "操作失败: %v\n", err)
		os.Exit(1)
	}

	lyric := lrc.Data.Lyric
	trans := lrc.Data.Trans

	if !embedFlag {
		if err := saveLyricToFile(lyric, trans); err != nil {
			fmt.Fprintf(os.Stderr, "操作失败: %v\n", err)
			os.Exit(1)
		}
	} else {
		fname := *filename
		if fname == "" {
			fname = promptString(reader, "Input the filename: ")
		}
		if fname == "" {
			fmt.Fprintln(os.Stderr, "错误: 嵌入模式下需要提供文件名")
			os.Exit(1)
		}

		ffmpegPath := resolveFfmpegPath()

		if err := embedLyric(ffmpegPath, fname, lyric, trans); err != nil {
			fmt.Fprintf(os.Stderr, "操作失败: %v\n", err)
			os.Exit(1)
		}
	}
}
