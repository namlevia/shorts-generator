#!/usr/bin/env node
import { config } from "dotenv";
config({ path: ".env.local" });

import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { loadConfig } from "./config.js";
import { fetchContent } from "./scraper/content-fetcher.js";
import { generateScriptWithLlm, generateCaption } from "./llm/script-generator.js";
import { runPipeline } from "./pipeline.js";
import { log } from "./utils/logger.js";

function getTimestampString(): string {
  const d = new Date();
  const yyyy = d.getFullYear();
  const MM = String(d.getMonth() + 1).padStart(2, "0");
  const DD = String(d.getDate()).padStart(2, "0");
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  return `${yyyy}${MM}${DD}-${hh}${mm}`;
}

async function main() {
  const args = process.argv.slice(2);
  const input = args.find((a) => !a.startsWith("--"));
  const isDryRun = args.includes("--dry-run");

  if (!input) {
    console.log(`
🎬 Auto Video Maker - Tạo video tự động 1 bước từ URL / GitHub
=============================================================
Cách sử dụng:
  npm run make -- <URL_HOAC_FILE_TXT> [options]

Ví dụ:
  npm run make -- https://github.com/cv-cat/DouYin_Spider
  npm run make -- https://github.com/cv-cat/DouYin_Spider --dry-run

Tùy chọn:
  --dry-run    Chỉ bóc tách và sinh script.json qua AI, không render video.
`);
    process.exit(1);
  }

  const cfg = loadConfig();
  console.log(`\n🚀 Bắt đầu quy trình tự động tạo video cho: ${input}\n`);

  try {
    // 1. Fetch content
    log.info("1/4. Đang bóc tách nội dung nguồn...");
    const content = await fetchContent(input);
    log.info(`  Tiêu đề: ${content.title}`);
    log.info(`  Nguồn: ${content.domain}`);
    log.info(`  Độ dài nội dung: ${content.content.length} ký tự`);

    // 2. Prepare output directory
    const timestamp = getTimestampString();
    const outDirName = `${content.slug}-${timestamp}`;
    const outputDir = join(process.cwd(), "output", outDirName);
    await mkdir(outputDir, { recursive: true });
    log.info(`  Thư mục output: output/${outDirName}`);

    // 3. Generate script via LLM
    log.info(`2/4. Đang gọi AI (${cfg.llm.model}) biên kịch kịch bản video...`);
    const script = await generateScriptWithLlm(content, cfg);
    const scriptPath = join(outputDir, "script.json");
    await writeFile(scriptPath, JSON.stringify(script, null, 2), "utf8");
    log.info(`  Đã lưu kịch bản vào: output/${outDirName}/script.json`);

    if (isDryRun) {
      log.info("\n✅ Chế độ --dry-run: Đã sinh kịch bản thành công. Dừng trước bước render.");
      console.log(`\nKiểm tra kịch bản tại: output/${outDirName}/script.json\n`);
      return;
    }

    // 4. Run Video Pipeline
    log.info("3/4. Đang kích hoạt pipeline tạo giọng đọc, ghép âm thanh & render video...");
    await runPipeline(scriptPath);

    // 5. Generate TikTok caption
    log.info("4/4. Đang tạo caption & hashtags cho TikTok...");
    const caption = await generateCaption(script.metadata.title, content.url, content.slug, cfg);
    const captionPath = join(outputDir, "caption.txt");
    await writeFile(captionPath, caption, "utf8");

    console.log("\n" + "=".repeat(60));
    console.log("🎉 XUẤT VIDEO THÀNH CÔNG!");
    console.log("=".repeat(60));
    console.log(`📹 Video:   output/${outDirName}/video.mp4`);
    console.log(`🎙 Audio:   output/${outDirName}/voice.mp3`);
    console.log(`📝 Script:  output/${outDirName}/script.txt  (dùng auto-caption CapCut)`);
    console.log(`📋 Caption: output/${outDirName}/caption.txt`);
    console.log("\nNội dung đăng bài (TikTok / Reels):");
    console.log("-".repeat(40));
    console.log(caption);
    console.log("-".repeat(40) + "\n");
  } catch (err: any) {
    log.error(`Quy trình thất bại: ${err.message}`, err);
    process.exit(1);
  }
}

main();
