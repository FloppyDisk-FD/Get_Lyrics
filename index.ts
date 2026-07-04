import ffmetadata from "ffmetadata";
import inquirer from "inquirer";
import { Command } from "commander";
import { promisify } from "node:util";
import { join } from "node:path";
import { existsSync } from "node:fs";

const writeMetadata = promisify(ffmetadata.write);
const readMetadata = promisify(ffmetadata.read);

const API_URL = "https://service-47o75c8f-1301683732.sh.apigw.tencentcs.com/release/lyric";
const OUTPUT_FILE = "./lyric.txt";
const LRC_ERROR_SIGNATURE = "ºw^~)Þ";

function resolveFfmpegPath(): string {
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

  return "ffmpeg";
}

const FFMPEG_PATH = resolveFfmpegPath();
ffmetadata.setFfmpegPath(FFMPEG_PATH);

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
  const data = {
    lyrics: lyric + trans
  };
  await writeMetadata(filename, data);
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
