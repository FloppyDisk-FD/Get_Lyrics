package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	defaultAPI    = "https://api.i-meto.com/meting/api"
	outputFile    = "./lyric.txt"
	ffmpegVersion = "7.1-minimal"
)

type Song struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Artist   []string `json:"artist"`
	Album    string   `json:"album"`
	PicID    string   `json:"pic_id"`
	URLID    string   `json:"url_id"`
	LyricID  string   `json:"lyric_id"`
	Source   string   `json:"source"`
}

type LyricResult struct {
	Lyric string `json:"lyric"`
	Trans string `json:"tlyric"`
}

type APIConfig struct {
	BaseURL string
	Server  string
}

var apiConfig APIConfig

func init() {
	apiConfig = APIConfig{
		BaseURL: defaultAPI,
		Server:  "netease",
	}
	if envURL := os.Getenv("METING_API"); envURL != "" {
		apiConfig.BaseURL = envURL
	}
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

	data, err := ffmpegFS.ReadFile(ffmpegEmbedPath())
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

func metingAPI(t, id string, extra map[string]string) ([]byte, error) {
	params := url.Values{}
	params.Set("server", apiConfig.Server)
	params.Set("type", t)
	params.Set("id", id)
	params.Set("format", "json")
	for k, v := range extra {
		params.Set(k, v)
	}

	url := apiConfig.BaseURL + "?" + params.Encode()
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("request failed, status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func searchSongs(keyword string, limit int) ([]Song, error) {
	body, err := metingAPI("search", keyword, map[string]string{
		"limit": strconv.Itoa(limit),
	})
	if err != nil {
		return nil, err
	}

	var songs []Song
	if err := json.Unmarshal(body, &songs); err != nil {
		return nil, fmt.Errorf("parse search result: %w", err)
	}

	return songs, nil
}

func getLyric(songID string) (string, string, error) {
	body, err := metingAPI("lyric", songID, nil)
	if err != nil {
		return "", "", err
	}

	var result LyricResult
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("parse lyric result: %w", err)
	}

	return result.Lyric, result.Trans, nil
}

func saveLyricToFile(lyric, trans string) error {
	if lyric == "" {
		return fmt.Errorf("歌词为空")
	}

	content := lyric
	if err := os.WriteFile(outputFile, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write lyric: %w", err)
	}
	fmt.Println("\n✓ 歌词写入成功！文件: lyric.txt")

	if trans != "" {
		f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("open lyric for append: %w", err)
		}
		defer f.Close()
		if _, err := f.WriteString("\n\n" + trans); err != nil {
			return fmt.Errorf("append trans: %w", err)
		}
		fmt.Println("✓ 翻译已追加")
	} else {
		fmt.Println("ℹ 本歌曲暂无翻译")
	}

	return nil
}

func embedLyric(ffmpegPath, filename, lyric, trans string) error {
	ext := strings.ToLower(filepath.Ext(filename))
	isMp3 := ext == ".mp3"

	tmpOut := filepath.Join(filepath.Dir(filename), fmt.Sprintf(".get-lyrics-tmp-%d%s", os.Getpid(), ext))
	defer os.Remove(tmpOut)

	args := []string{"-y", "-i", filename}

	fullLyric := lyric
	if trans != "" {
		fullLyric = lyric + "\n\n" + trans
	}

	if isMp3 {
		args = append(args,
			"-metadata", "lyrics="+fullLyric,
			"-metadata", "lyric="+fullLyric,
			"-id3v2_version", "3",
			"-write_id3v1", "1",
		)
	} else {
		args = append(args,
			"-metadata", "lyrics="+fullLyric,
		)
	}

	args = append(args, "-c", "copy", "-map", "0", tmpOut)

	fmt.Print("  正在嵌入歌词... ")
	cmd := exec.Command(ffmpegPath, args...)
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		fmt.Println("失败")
		return fmt.Errorf("ffmpeg failed: %w", err)
	}
	fmt.Println("完成")

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

	return nil
}

func promptString(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func promptInt(reader *bufio.Reader, prompt string, min, max int) (int, bool) {
	for {
		fmt.Print(prompt)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "q" || line == "quit" || line == "exit" {
			return 0, false
		}
		n, err := strconv.Atoi(line)
		if err != nil || n < min || n > max {
			fmt.Printf("  请输入 %d-%d 之间的数字（或输入 q 退出）\n", min, max)
			continue
		}
		return n, true
	}
}

func printSongList(songs []Song) {
	fmt.Println("\n═══════════════════════════════════════════")
	for i, s := range songs {
		artist := strings.Join(s.Artist, ", ")
		idx := i + 1
		fmt.Printf("  %2d. %s\n", idx, s.Name)
		fmt.Printf("      %s - %s\n", artist, s.Album)
	}
	fmt.Println("═══════════════════════════════════════════")
}

func selectSongInteractive(reader *bufio.Reader) (*Song, error) {
	keyword := promptString(reader, "\n🎵 请输入歌曲名或歌手: ")
	if keyword == "" {
		return nil, fmt.Errorf("关键词不能为空")
	}

	fmt.Print("  正在搜索... ")
	songs, err := searchSongs(keyword, 10)
	if err != nil {
		fmt.Println("失败")
		return nil, err
	}
	fmt.Printf("找到 %d 首\n", len(songs))

	if len(songs) == 0 {
		return nil, fmt.Errorf("未找到相关歌曲")
	}

	printSongList(songs)

	n, ok := promptInt(reader, "\n请选择歌曲编号 (1-"+strconv.Itoa(len(songs))+", q 退出): ", 1, len(songs))
	if !ok {
		return nil, fmt.Errorf("已取消")
	}

	return &songs[n-1], nil
}

func interactiveMode(reader *bufio.Reader) {
	fmt.Println("╔═══════════════════════════════════════════╗")
	fmt.Println("║        Get Lyrics - 歌词获取工具          ║")
	fmt.Println("╠═══════════════════════════════════════════╣")
	fmt.Printf("║  当前平台: %-26s ║\n", platformName(apiConfig.Server))
	fmt.Println("║                                           ║")
	fmt.Println("║  1. 搜索歌曲并保存歌词                    ║")
	fmt.Println("║  2. 搜索歌曲并嵌入歌词到音频文件          ║")
	fmt.Println("║  3. 切换音乐平台                          ║")
	fmt.Println("║  4. 退出                                  ║")
	fmt.Println("╚═══════════════════════════════════════════╝")

	for {
		choice, ok := promptInt(reader, "\n请选择操作 (1-4): ", 1, 4)
		if !ok {
			continue
		}

		switch choice {
		case 1:
			song, err := selectSongInteractive(reader)
			if err != nil {
				fmt.Printf("\n✗ %v\n", err)
				continue
			}
			fmt.Printf("\n  已选择: %s - %s\n", strings.Join(song.Artist, ", "), song.Name)
			fmt.Print("  正在获取歌词... ")
			lyric, trans, err := getLyric(song.ID)
			if err != nil {
				fmt.Println("失败")
				fmt.Printf("✗ %v\n", err)
				continue
			}
			fmt.Println("完成")
			if err := saveLyricToFile(lyric, trans); err != nil {
				fmt.Printf("✗ %v\n", err)
			}

		case 2:
			song, err := selectSongInteractive(reader)
			if err != nil {
				fmt.Printf("\n✗ %v\n", err)
				continue
			}
			fmt.Printf("\n  已选择: %s - %s\n", strings.Join(song.Artist, ", "), song.Name)

			filename := promptString(reader, "\n📁 请输入音频文件路径: ")
			if filename == "" {
				fmt.Println("✗ 文件路径不能为空")
				continue
			}
			if _, err := os.Stat(filename); os.IsNotExist(err) {
				fmt.Printf("✗ 文件不存在: %s\n", filename)
				continue
			}

			fmt.Print("  正在获取歌词... ")
			lyric, trans, err := getLyric(song.ID)
			if err != nil {
				fmt.Println("失败")
				fmt.Printf("✗ %v\n", err)
				continue
			}
			fmt.Println("完成")

			ffmpegPath := resolveFfmpegPath()
			if err := embedLyric(ffmpegPath, filename, lyric, trans); err != nil {
				fmt.Printf("✗ %v\n", err)
				continue
			}
			fmt.Println("\n✓ 歌词嵌入成功！")

		case 3:
			switchPlatform(reader)

		case 4:
			fmt.Println("\n👋 再见！")
			return
		}
	}
}

func switchPlatform(reader *bufio.Reader) {
	fmt.Println("\n可用平台:")
	fmt.Println("  1. 网易云音乐 (netease)")
	fmt.Println("  2. QQ音乐 (tencent)")
	fmt.Println("  3. 酷狗音乐 (kugou)")
	fmt.Println("  4. 酷我音乐 (kuwo)")
	fmt.Println("  5. 百度音乐 (baidu)")

	n, ok := promptInt(reader, "\n请选择平台 (1-5): ", 1, 5)
	if !ok {
		return
	}

	platforms := []string{"netease", "tencent", "kugou", "kuwo", "baidu"}
	apiConfig.Server = platforms[n-1]
	fmt.Printf("\n✓ 已切换到: %s\n", platformName(apiConfig.Server))
}

func platformName(server string) string {
	names := map[string]string{
		"netease": "网易云音乐",
		"tencent": "QQ音乐",
		"kugou":   "酷狗音乐",
		"kuwo":    "酷我音乐",
		"baidu":   "百度音乐",
	}
	if n, ok := names[server]; ok {
		return n
	}
	return server
}

func main() {
	var (
		songID    = flag.String("id", "", "歌曲 ID")
		keyword   = flag.String("k", "", "搜索关键词（交互式选择）")
		embed     = flag.Bool("e", false, "嵌入歌词到歌曲文件")
		filename  = flag.String("f", "", "音频文件路径（嵌入模式使用）")
		server    = flag.String("p", "netease", "音乐平台: netease/tencent/kugou/kuwo/baidu")
		apiURL    = flag.String("api", "", "Meting API 地址")
		showHelp  = flag.Bool("h", false, "显示帮助")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "get-lyrics - 多平台歌词获取工具\n\n")
		fmt.Fprintf(os.Stderr, "用法:\n")
		fmt.Fprintf(os.Stderr, "  get-lyrics                    交互模式\n")
		fmt.Fprintf(os.Stderr, "  get-lyrics -k 关键词          搜索歌曲并选择\n")
		fmt.Fprintf(os.Stderr, "  get-lyrics -id 歌曲ID         直接通过ID获取歌词\n")
		fmt.Fprintf(os.Stderr, "  get-lyrics -k 关键词 -e -f a.mp3   搜索并嵌入歌词\n\n")
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showHelp {
		flag.Usage()
		return
	}

	if *apiURL != "" {
		apiConfig.BaseURL = *apiURL
	}
	apiConfig.Server = *server

	reader := bufio.NewReader(os.Stdin)

	hasID := *songID != ""
	hasKeyword := *keyword != ""
	hasEmbed := *embed
	hasFile := *filename != ""

	if !hasID && !hasKeyword && !hasEmbed && !hasFile {
		interactiveMode(reader)
		return
	}

	var song *Song

	if hasID {
		song = &Song{ID: *songID}
	} else if hasKeyword {
		var err error
		fmt.Print("正在搜索... ")
		songs, err := searchSongs(*keyword, 10)
		if err != nil {
			fmt.Fprintf(os.Stderr, "搜索失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("找到 %d 首\n", len(songs))

		if len(songs) == 0 {
			fmt.Fprintln(os.Stderr, "未找到相关歌曲")
			os.Exit(1)
		}

		printSongList(songs)
		n, ok := promptInt(reader, "\n请选择歌曲编号 (1-"+strconv.Itoa(len(songs))+"): ", 1, len(songs))
		if !ok {
			fmt.Fprintln(os.Stderr, "已取消")
			os.Exit(1)
		}
		song = &songs[n-1]
	} else {
		flag.Usage()
		os.Exit(1)
	}

	fmt.Print("正在获取歌词... ")
	lyric, trans, err := getLyric(song.ID)
	if err != nil {
		fmt.Println("失败")
		fmt.Fprintf(os.Stderr, "获取歌词失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("完成")

	if hasEmbed {
		if !hasFile {
			fmt.Fprintln(os.Stderr, "错误: 嵌入模式需要指定音频文件路径 (-f)")
			os.Exit(1)
		}
		if _, err := os.Stat(*filename); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "错误: 文件不存在: %s\n", *filename)
			os.Exit(1)
		}
		ffmpegPath := resolveFfmpegPath()
		if err := embedLyric(ffmpegPath, *filename, lyric, trans); err != nil {
			fmt.Fprintf(os.Stderr, "嵌入失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ 歌词嵌入成功！")
	} else {
		if err := saveLyricToFile(lyric, trans); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}
}
