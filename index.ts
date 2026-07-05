import inquirer from "inquirer";
import { Command } from "commander";
import { join, dirname, extname } from "node:path";
import { existsSync, chmodSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { createHash } from "node:crypto";
import { spawn } from "node:child_process";
// @ts-ignore: Bun file import
import ffmpegBinary from "./bin/ffmpeg" with { type: "file" };

const API_URL = "https://service-47o75c8f-1301683732.sh.apigw.tencentcs.com/release/lyric";
const OUTPUT_FILE = "./lyric.txt";
const LRC_ERROR_SIGNATURE = "ºw^~)Þ";

async function extractEmbeddedFfmpeg(): Promise<string> {
  const exeName = process.platform === "win32" ? "ffmpeg.exe" : "ffmpeg";
  const version = "7.1-minimal";
  const hash = createHash("md5").update(version + process.platform).digest("hex").slice(0, 8);
  const extractDir = join(tmpdir(), `get-lyrics-ffmpeg-${hash}`);
  const extractPath = join(extractDir, exeName);

  if (existsSync(extractPath)) {
    return extractPath;
  }

  const file = ffmpegBinary as Blob;
  const buf = Buffer.from(await file.arrayBuffer());

  await Bun.write(extractPath, buf);
  chmodSync(extractPath, 0o755);

  return extractPath;
}

async function resolveFfmpegPath(): Promise<string> {
  const exe = process.platform === "win32" ? "ffmpeg.exe" : "ffmpeg";

  const candidates = [
    process.env.FFMPEG_PATH,
    join(import.meta.dir, "bin", exe),
    join(import.meta.dir, "..", "bin", exe),
    join(process.cwd(), "bin", exe),
  ];

  for (const p of candidates) {
    if (p && existsSync(p)) {
      return p;
    }
  }

  try {
    return await extractEmbeddedFfmpeg();
  } catch {
    return "ffmpeg";
  }
}

const FFMPEG_PATH = await resolveFfmpegPath();

function ffmpeg(args: string[]): Promise<void> {
  return new Promise((resolve, reject) => {
    const proc = spawn(FFMPEG_PATH, args, { stdio: "pipe" });
    let stderr = "";
    proc.stderr.on("data", (data) => { stderr += data.toString(); });
    proc.on("error", reject);
    proc.on("close", (code) => {
      if (code === 0) {
        resolve();
      } else {
        reject(new Error(`ffmpeg exited with code ${code}\n${stderr}`));
      }
    });
  });
}

async function writeLyricMetadata(filename: string, lyrics: string): Promise<void> {
  const ext = extname(filename).toLowerCase();
  const isMp3 = ext === ".mp3";

  const tmpOut = join(
    dirname(filename),
    `.get-lyrics-tmp-${Date.now()}${ext}`
  );

  try {
    const args: string[] = [
      "-y",
      "-i", filename,
    ];

    if (isMp3) {
      args.push(
        "-metadata", `lyrics=${lyrics}`,
        "-metadata", `lyric=${lyrics}`,
        "-id3v2_version", "3",
        "-write_id3v1", "1",
      );
    } else {
      args.push(
        "-metadata", `lyrics=${lyrics}`,
      );
    }

    args.push(
      "-c", "copy",
      "-map", "0",
      tmpOut
    );

    await ffmpeg(args);

    await Bun.write(filename, Bun.file(tmpOut));
  } finally {
    if (existsSync(tmpOut)) {
      rmSync(tmpOut);
    }
  }
}

interface LyricResponse {
  data: {
    lyric: string;
    trans: string;
  };
}

interface CliOptions {
  songmid?: string;
  embed?: boolean;
  filename?: string;
}

const program = new Command();

program
  .name("get-lyrics")
  .description("获取QQ音乐歌词的工具")
  .version("1.0.0")
  .option("-s, --songmid <id>", "歌曲 songmid")
  .option("-e, --embed", "嵌入歌词到歌曲文件")
  .option("-f, --filename <path>", "歌曲文件路径（嵌入模式下使用）")
  .parse();

const opts = program.opts<CliOptions>();

async function fetchLyric(songmid: string): Promise<LyricResponse> {
  const url = `${API_URL}?${new URLSearchParams({ songmid })}`;
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`请求失败，状态码: ${res.status}`);
  }
  return (await res.json()) as LyricResponse;
}

interface PromptAnswers {
  songmid?: string;
  embed?: boolean;
  filename?: string;
}

async function promptUser(): Promise<{ songmid: string; embed: boolean; filename?: string }> {
  const promptList = [
    {
      type: "input",
      name: "songmid",
      message: "Input the songmid: ",
      when: () => !opts.songmid
    },
    {
      type: "confirm",
      name: "embed",
      message: "Whether to embed lyrics to a song or not",
      when: () => opts.embed === undefined
    },
    {
      type: "input",
      name: "filename",
      message: "Input the filename: ",
      when: (answers: PromptAnswers) => answers.embed === true && !opts.filename
    }
  ];

  const answers = await inquirer.prompt<PromptAnswers>(promptList);

  return {
    songmid: opts.songmid ?? answers.songmid ?? "",
    embed: opts.embed ?? answers.embed ?? false,
    filename: opts.filename ?? answers.filename
  };
}

async function saveLyricToFile(lyric: string, trans: string): Promise<void> {
  if (lyric === LRC_ERROR_SIGNATURE) {
    console.log("请求失败，songmid可能存在错误！");
    return;
  }

  await Bun.write(OUTPUT_FILE, lyric);
  console.log("歌词写入成功! \n歌词在本目录下lyric.txt内");

  if (trans === "") {
    console.log("本歌曲不存在翻译！");
  } else {
    const file = Bun.file(OUTPUT_FILE);
    const existing = await file.text();
    await Bun.write(OUTPUT_FILE, existing + trans);
    console.log("翻译写入成功! ");
  }
}

async function embedLyric(filename: string, lyric: string, trans: string): Promise<void> {
  await writeLyricMetadata(filename, lyric + trans);
  console.log("歌词嵌入成功! ");
}

async function main(): Promise<void> {
  try {
    const { songmid, embed, filename } = await promptUser();

    if (!songmid) {
      console.error("错误: songmid 不能为空");
      process.exit(1);
    }

    const lrc = await fetchLyric(songmid);
    const lyric = lrc.data.lyric;
    const trans = lrc.data.trans;

    if (!embed) {
      await saveLyricToFile(lyric, trans);
    } else {
      if (!filename) {
        console.error("错误: 嵌入模式下需要提供文件名");
        process.exit(1);
      }
      await embedLyric(filename, lyric, trans);
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    console.error(`操作失败: ${message}`);
    process.exit(1);
  }
}

await main();
