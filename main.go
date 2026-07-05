package main

import (
	"bufio"
	"bytes"
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
	"time"
)

const (
	defaultAPI    = "https://api.i-meto.com/meting/api"
	outputFile    = "./lyric.txt"
	ffmpegVersion = "7.1-minimal"
)

// Song 对应 Meting API 搜索结果的实际返回结构
// 字段: title/author/url/pic/lrc（lrc 为获取歌词的完整 URL，含 auth 参数）
type Song struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	URL    string `json:"url"`
	Pic    string `json:"pic"`
	Lrc    string `json:"lrc"`
}

type APIConfig struct {
	BaseURL string
	Server  string
}

var apiConfig APIConfig

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

func init() {
	apiConfig = APIConfig{
		BaseURL: defaultAPI,
		Server:  "netease",
	}
	if envURL := os.Getenv("METING_API"); envURL != "" {
		apiConfig.BaseURL = envURL
	}

	// 自动配置系统代理：优先环境变量，Windows 下回退到注册表系统代理
	transport := &http.Transport{}
	if proxyURL := resolveSystemProxy(); proxyURL != nil {
		transport.Proxy = http.ProxyURL(proxyURL)
	} else {
		// 环境变量代理（HTTP_PROXY/HTTPS_PROXY）
		transport.Proxy = http.ProxyFromEnvironment
	}
	httpClient.Transport = transport
}

// resolveSystemProxy 读取系统代理设置。
// Windows: 从注册表读取 IE/系统代理配置（浏览器使用的同一套设置）。
// 其他平台: 返回 nil，交给 http.ProxyFromEnvironment 处理。
func resolveSystemProxy() *url.URL {
	if runtime.GOOS != "windows" {
		return nil
	}

	// 仅 Windows：通过 reg query 读取注册表中的代理设置
	out, err := exec.Command("reg", "query",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyEnable").CombinedOutput()
	if err != nil {
		return nil
	}
	// 未启用系统代理
	if !strings.Contains(string(out), "0x1") {
		return nil
	}

	out, err = exec.Command("reg", "query",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyServer").CombinedOutput()
	if err != nil {
		return nil
	}

	// 解析 ProxyServer 值，格式可能为 "host:port" 或 "http=host:port;https=..."
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ProxyServer") {
			continue
		}
		// 取 REG_SZ 后的值
		parts := strings.SplitN(line, "REG_SZ", 2)
		if len(parts) != 2 {
			continue
		}
		raw := strings.TrimSpace(parts[1])
		if raw == "" {
			continue
		}

		// 多协议格式：http=127.0.0.1:7890;https=127.0.0.1:7890
		if strings.Contains(raw, "=") {
			for _, seg := range strings.Split(raw, ";") {
				seg = strings.TrimSpace(seg)
				if strings.HasPrefix(seg, "http=") || strings.HasPrefix(seg, "https=") {
					addr := strings.TrimPrefix(strings.TrimPrefix(seg, "http="), "https=")
					if u, err := parseProxyAddr(addr); err == nil {
						return u
					}
				}
			}
			return nil
		}
		// 单一格式：127.0.0.1:7890
		if u, err := parseProxyAddr(raw); err == nil {
			return u
		}
	}
	return nil
}

// parseProxyAddr 将 host:port 转为 *url.URL
func parseProxyAddr(addr string) (*url.URL, error) {
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	return url.Parse(addr)
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
			if ffmpegWorks(p) {
				return p
			}
		}
	}

	if path, err := extractEmbeddedFfmpeg(); err == nil {
		if ffmpegWorks(path) {
			return path
		}
	}

	if path, err := exec.LookPath(name); err == nil {
		if ffmpegWorks(path) {
			return path
		}
	}

	return name
}

// ffmpegWorks 检测给定路径的 ffmpeg 是否能正常运行
func ffmpegWorks(path string) bool {
	cmd := exec.Command(path, "-version")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
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
	resp, err := httpClient.Get(url)
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

// getLyric 直接请求搜索结果中的 lrc URL 获取歌词文本
// Meting API 的 lrc 字段返回纯文本 LRC 格式歌词（非 JSON）
func getLyric(lrcURL string) (string, error) {
	if lrcURL == "" {
		return "", fmt.Errorf("歌词 URL 为空")
	}
	resp, err := httpClient.Get(lrcURL)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("request failed, status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read lyric: %w", err)
	}

	return string(body), nil
}

func saveLyricToFile(lyric string) error {
	if lyric == "" {
		return fmt.Errorf("歌词为空")
	}

	if err := os.WriteFile(outputFile, []byte(lyric), 0o644); err != nil {
		return fmt.Errorf("write lyric: %w", err)
	}
	fmt.Println("\n✓ 歌词写入成功！文件: lyric.txt")
	return nil
}

func embedLyric(ffmpegPath, filename, lyric string) error {
	ext := strings.ToLower(filepath.Ext(filename))
	isMp3 := ext == ".mp3"

	tmpOut := filepath.Join(filepath.Dir(filename), fmt.Sprintf(".get-lyrics-tmp-%d%s", os.Getpid(), ext))
	defer os.Remove(tmpOut)

	args := []string{"-y", "-i", filename}

	if isMp3 {
		args = append(args,
			"-metadata", "lyrics="+lyric,
			"-metadata", "lyric="+lyric,
			"-id3v2_version", "3",
			"-write_id3v1", "1",
		)
	} else {
		args = append(args,
			"-metadata", "lyrics="+lyric,
		)
	}

	args = append(args, "-c", "copy", "-map", "0", tmpOut)

	fmt.Print("  正在嵌入歌词... ")
	cmd := exec.Command(ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		fmt.Println("失败")
		exitCode := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = fmt.Sprintf(" (退出码: %s)", exitErr.ExitCode())
		}
		errMsg := stderr.String()
		if strings.Contains(errMsg, "STATUS_DLL_NOT_FOUND") || strings.Contains(errMsg, "0xc0000135") || strings.Contains(errMsg, "缺少 DLL") {
			return fmt.Errorf("ffmpeg 启动失败：缺少依赖的 DLL 文件（常见于 Windows）。请安装 MSVC 运行库，或在系统 PATH 中安装 ffmpeg 后重试。原始错误: %w%s", err, exitCode)
		}
		if errMsg != "" {
			return fmt.Errorf("ffmpeg failed%s: %s\n%s", exitCode, err, errMsg)
		}
		return fmt.Errorf("ffmpeg failed%s: %w", exitCode, err)
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
		idx := i + 1
		fmt.Printf("  %2d. %s\n", idx, s.Title)
		fmt.Printf("      %s\n", s.Author)
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
			fmt.Printf("\n  已选择: %s - %s\n", song.Author, song.Title)
			fmt.Print("  正在获取歌词... ")
			lyric, err := getLyric(song.Lrc)
			if err != nil {
				fmt.Println("失败")
				fmt.Printf("✗ %v\n", err)
				continue
			}
			fmt.Println("完成")
			if err := saveLyricToFile(lyric); err != nil {
				fmt.Printf("✗ %v\n", err)
			}

		case 2:
			song, err := selectSongInteractive(reader)
			if err != nil {
				fmt.Printf("\n✗ %v\n", err)
				continue
			}
			fmt.Printf("\n  已选择: %s - %s\n", song.Author, song.Title)

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
			lyric, err := getLyric(song.Lrc)
			if err != nil {
				fmt.Println("失败")
				fmt.Printf("✗ %v\n", err)
				continue
			}
			fmt.Println("完成")

			ffmpegPath := resolveFfmpegPath()
			if err := embedLyric(ffmpegPath, filename, lyric); err != nil {
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
		keyword  = flag.String("k", "", "搜索关键词（交互式选择）")
		embed    = flag.Bool("e", false, "嵌入歌词到歌曲文件")
		filename = flag.String("f", "", "音频文件路径（嵌入模式使用）")
		server   = flag.String("p", "netease", "音乐平台: netease/tencent/kugou/kuwo/baidu")
		apiURL   = flag.String("api", "", "Meting API 地址")
		showHelp = flag.Bool("h", false, "显示帮助")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "get-lyrics - 多平台歌词获取工具\n\n")
		fmt.Fprintf(os.Stderr, "用法:\n")
		fmt.Fprintf(os.Stderr, "  get-lyrics                    交互模式\n")
		fmt.Fprintf(os.Stderr, "  get-lyrics -k 关键词          搜索歌曲并选择\n")
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

	hasKeyword := *keyword != ""
	hasEmbed := *embed
	hasFile := *filename != ""

	if !hasKeyword && !hasEmbed && !hasFile {
		interactiveMode(reader)
		return
	}

	if !hasKeyword {
		flag.Usage()
		os.Exit(1)
	}

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
	song := &songs[n-1]

	fmt.Print("正在获取歌词... ")
	lyric, err := getLyric(song.Lrc)
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
		if err := embedLyric(ffmpegPath, *filename, lyric); err != nil {
			fmt.Fprintf(os.Stderr, "嵌入失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ 歌词嵌入成功！")
	} else {
		if err := saveLyricToFile(lyric); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}
}
