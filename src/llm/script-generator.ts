import axios from "axios";
import { ScriptSchema, type Script } from "../render/script-schema.js";
import type { Config } from "../config.js";
import type { ScrapedContent } from "../scraper/content-fetcher.js";
import { log } from "../utils/logger.js";

const SYSTEM_PROMPT = `Bạn là biên kịch video ngắn (TikTok / Shorts / Reels) chuyên nghiệp về công nghệ cho kênh công nghệ tiếng Việt.
Nhiệm vụ của bạn là đọc nội dung bài viết/mã nguồn được cung cấp và tạo ra một kịch bản video định dạng JSON chuẩn xác 100%.

YÊU CẦU CẤU TRÚC JSON:
Trả về duy nhất 1 JSON object (không giải thích thêm, bọc trong \`\`\`json ... \`\`\`), đúng cấu trúc sau:
{
  "version": "1.0",
  "metadata": {
    "title": "Tiêu đề video hấp dẫn (tiếng Việt)",
    "source": {
      "url": "<URL_GOC>",
      "domain": "<DOMAIN>",
      "image": "<URL_ANH_HOAC_NULL>"
    },
    "channel": "<TEN_KENH>"
  },
  "voice": {
    "provider": "edge-tts",
    "voiceId": "\${VOICE_ID}",
    "speed": 1.0
  },
  "scenes": [
    // Bắt buộc từ 5 đến 8 cảnh.
    // Mỗi scene BẮT BUỘC có đủ: id, type, voiceText, templateData
    {
      "id": "hook",
      "type": "hook",
      "voiceText": "Câu thoại mở đầu giật tít thu hút người nghe trong 3 giây.",
      "templateData": {
        "template": "hook",
        "headline": "Tiêu đề giật tít (tối đa 40 ký tự)",
        "subhead": "Phụ đề tò mò (tối đa 40 ký tự)",
        "bgSrc": "$source.image",
        "kenBurns": "zoom-in"
      }
    },
    {
      "id": "body-1",
      "type": "body",
      "voiceText": "Câu thoại giải thích vấn đề hoặc bối cảnh.",
      "templateData": {
        "template": "callout",
        "statement": "Nội dung nhấn mạnh ngắn gọn (tối đa 80 ký tự)",
        "tag": "Điểm nổi bật"
      }
    },
    {
      "id": "body-2",
      "type": "body",
      "voiceText": "Câu thoại giới thiệu các tính năng chính.",
      "templateData": {
        "template": "feature-list",
        "title": "Tính năng nổi bật (tối đa 40 ký tự)",
        "bullets": [
          "Ý 1 ngắn gọn (tối đa 50 ký tự)",
          "Ý 2 ngắn gọn (tối đa 50 ký tự)",
          "Ý 3 ngắn gọn (tối đa 50 ký tự)"
        ]
      }
    },
    {
      "id": "body-3",
      "type": "body",
      "voiceText": "Câu thoại đưa ra số liệu chứng minh hoặc so sánh.",
      "templateData": {
        "template": "stat-hero",
        "value": "100%",
        "label": "Mô tả số liệu (tối đa 40 ký tự)",
        "context": "Bối cảnh số liệu (tối đa 50 ký tự)"
      }
    },
    {
      "id": "outro",
      "type": "outro",
      "voiceText": "Theo dõi kênh để xem bản tin công nghệ mới mỗi ngày.",
      "templateData": {
        "template": "outro",
        "ctaTop": "Xem bản tin mới mỗi ngày",
        "channelName": "<TEN_KENH>",
        "source": "<DOMAIN>"
      }
    }
  ]
}

CÁC TEMPLATE ĐƯỢC PHÉP DÙNG TRONG templateData:
1. hook: headline (max 40), subhead (max 40), bgSrc: "$source.image", kenBurns: "zoom-in" | "zoom-out" | "pan-left" | "pan-right"
2. comparison: left { label (max 30), value (max 20), color: "cyan" }, right { label (max 30), value (max 20), color: "purple", winner: true }
3. stat-hero: value (max 20), label (max 40), context (max 50)
4. feature-list: title (max 40), bullets (mảng 1-4 chuỗi, mỗi chuỗi TỐI ĐA 50 ký tự)
5. callout: statement (tối đa 80 ký tự), tag (tối đa 20 ký tự)
6. outro: ctaTop (max 30), channelName (max 30), source (max 40)

⚠️ QUY TẮC BẮT BUỘC VỀ LỜI THOẠI (voiceText - Phát âm AI TTS):
- voiceText sẽ được AI Voice đọc thành tiếng. AI đọc máy móc từng ký tự nếu viết số hay từ viết tắt!
- BẮT BUỘC phiên âm chữ viết tắt và số trong voiceText:
  * "AI" -> viết là "ây ai"
  * "API" -> viết là "ây pi ai"
  * Số phiên bản thập phân: "5.5" -> viết là "năm chấm năm" (không viết 5.5 vì AI sẽ đọc là năm rưỡi)
  * Số phần trăm: "82.7%" -> viết là "tám mươi hai phẩy bảy phần trăm"
  * Giá tiền: "5$" -> viết là "năm đô la"
  * Tên kênh LeviaTech: CHỈ trong voiceText mới phiên âm là "Lê-vi-a Tếch" để AI đọc tự nhiên. Còn trên màn hình (metadata.channel, templateData.channelName) BẮT BUỘC giữ nguyên "LeviaTech".
  * Tên riêng tiếng Anh nổi tiếng (Google, Apple, Microsoft, TikTok, Python) giữ nguyên.
- Không dùng emoji, không dùng URL, không dùng ký tự đặc biệt (&, $, %, #, ->) trong voiceText.
- Mỗi câu thoại voiceText kết thúc bằng dấu chấm (.) hoặc dấu hỏi (?).
- Tổng thời lượng đọc cả bài từ 150-200 từ (~55 đến 65 giây).`;

function normalizeParsedJson(parsed: any, content: ScrapedContent, cfg: Config): any {
  const channelName = cfg.tiktok.displayName || "LeviaTech";

  parsed.version = "1.0";
  if (!parsed.metadata) parsed.metadata = {};
  if (!parsed.metadata.title) parsed.metadata.title = content.title;
  parsed.metadata.channel = channelName; // Always enforce canonical brand name on-screen
  parsed.metadata.source = {
    url: content.url,
    domain: content.domain,
    image: content.ogImage ?? parsed.metadata?.source?.image ?? null,
  };

  if (!parsed.voice) {
    parsed.voice = {
      provider: cfg.ttsProvider,
      voiceId: "${VOICE_ID}",
      speed: 1.0,
    };
  }

  if (!Array.isArray(parsed.scenes)) {
    parsed.scenes = [];
  }

  // Ensure scenes count is between 5 and 8
  if (parsed.scenes.length > 8) {
    parsed.scenes = [
      parsed.scenes[0],
      ...parsed.scenes.slice(1, 7),
      parsed.scenes[parsed.scenes.length - 1],
    ];
  }

  const lastIdx = parsed.scenes.length - 1;
  parsed.scenes.forEach((scene: any, idx: number) => {
    // 1. Ensure id
    if (!scene.id || typeof scene.id !== "string") {
      scene.id = idx === 0 ? "hook" : idx === lastIdx ? "outro" : `body-${idx}`;
    }

    // 2. Ensure type
    if (idx === 0) scene.type = "hook";
    else if (idx === lastIdx) scene.type = "outro";
    else scene.type = "body";

    // 3. Ensure templateData
    if (!scene.templateData || typeof scene.templateData !== "object") {
      scene.templateData = idx === 0 ? { template: "hook", headline: content.title.slice(0, 40) } : { template: "callout", statement: scene.voiceText.slice(0, 80) };
    }

    const td = scene.templateData;
    switch (td.template) {
      case "hook":
        td.headline = String(td.headline || content.title).slice(0, 40);
        if (td.subhead) td.subhead = String(td.subhead).slice(0, 40);
        td.bgSrc = td.bgSrc || "$source.image";
        if (!["zoom-in", "zoom-out", "pan-left", "pan-right"].includes(td.kenBurns)) {
          td.kenBurns = "zoom-in";
        }
        break;

      case "comparison":
        if (!td.left) td.left = { label: "Trước", value: "Cũ", color: "cyan" };
        if (!td.right) td.right = { label: "Sau", value: "Mới", color: "purple", winner: true };
        td.left.label = String(td.left.label).slice(0, 30);
        td.left.value = String(td.left.value).slice(0, 20);
        td.left.color = td.left.color === "purple" ? "purple" : "cyan";
        td.right.label = String(td.right.label).slice(0, 30);
        td.right.value = String(td.right.value).slice(0, 20);
        td.right.color = td.right.color === "cyan" ? "cyan" : "purple";
        break;

      case "stat-hero":
        td.value = String(td.value || "100%").slice(0, 20);
        td.label = String(td.label || "Số liệu").slice(0, 40);
        if (td.context) td.context = String(td.context).slice(0, 50);
        break;

      case "feature-list":
        td.title = String(td.title || "Tính năng chính").slice(0, 40);
        if (Array.isArray(td.bullets)) {
          td.bullets = td.bullets
            .map((b: any) => String(b).slice(0, 50))
            .filter(Boolean)
            .slice(0, 4);
        }
        if (!Array.isArray(td.bullets) || td.bullets.length === 0) {
          td.bullets = ["Dễ dàng sử dụng", "Tự động hóa hoàn toàn"];
        }
        break;

      case "callout":
        td.statement = String(td.statement || scene.voiceText || "Điểm nổi bật").slice(0, 80);
        if (td.tag) td.tag = String(td.tag).slice(0, 20);
        break;

      case "outro":
        td.ctaTop = String(td.ctaTop || "Xem bản tin mới mỗi ngày").slice(0, 30);
        td.channelName = channelName; // Always enforce canonical brand name on-screen
        td.source = String(
          td.source && td.source !== "github.com" ? td.source : (content.displaySource || content.domain)
        ).slice(0, 80);
        break;

      default:
        // fallback to callout if unrecognized
        scene.templateData = {
          template: "callout",
          statement: String(scene.voiceText || "Thông tin nổi bật").slice(0, 80),
          tag: "Tin tức",
        };
        break;
    }
  });

  return parsed;
}

export async function generateScriptWithLlm(
  content: ScrapedContent,
  cfg: Config
): Promise<Script> {
  const channelName = cfg.tiktok.displayName || "LeviaTech";
  const userPrompt = `Dưới đây là thông tin bài viết/dự án cần làm video:
- Tiêu đề: ${content.title}
- Nguồn domain: ${content.domain}
- URL: ${content.url}
- Ảnh og:image: ${content.ogImage ?? "null"}
- Tên kênh: ${channelName}

NỘI DUNG CHI TIẾT:
${content.content}

Hãy tạo file script.json hoàn chỉnh cho video này theo đúng hướng dẫn JSON.`;

  log.info(`Calling LLM (${cfg.llm.model}) at ${cfg.llm.baseUrl}...`);

  const resp = await axios.post(
    `${cfg.llm.baseUrl}/chat/completions`,
    {
      model: cfg.llm.model,
      messages: [
        { role: "system", content: SYSTEM_PROMPT },
        { role: "user", content: userPrompt },
      ],
      temperature: 0.7,
      max_tokens: 4096,
    },
    {
      headers: {
        Authorization: `Bearer ${cfg.llm.apiKey}`,
        "Content-Type": "application/json",
      },
      timeout: 90000,
    }
  );

  const rawText: string = resp.data?.choices?.[0]?.message?.content ?? "";
  if (!rawText) {
    throw new Error("Empty response from LLM");
  }

  // Clean markdown json fences
  const jsonMatch = rawText.match(/```(?:json)?\s*([\s\S]*?)\s*```/);
  let jsonStr = (jsonMatch ? jsonMatch[1] : rawText).trim();

  // Robust fallback: isolate the outer JSON object { ... }
  const firstBrace = jsonStr.indexOf('{');
  const lastBrace = jsonStr.lastIndexOf('}');
  if (firstBrace !== -1 && lastBrace !== -1 && lastBrace > firstBrace) {
    jsonStr = jsonStr.substring(firstBrace, lastBrace + 1);
  }

  let parsedJson: any;
  try {
    parsedJson = JSON.parse(jsonStr);
  } catch (e: any) {
    throw new Error(`Failed to parse LLM response as JSON: ${e.message}\nResponse: ${jsonStr.slice(0, 500)}`);
  }

  // Normalize and auto-correct any small LLM formatting slips
  const normalized = normalizeParsedJson(parsedJson, content, cfg);

  // Validate with Zod schema
  const validation = ScriptSchema.safeParse(normalized);
  if (!validation.success) {
    log.warn("Script validation errors: " + JSON.stringify(validation.error.issues, null, 2));
    throw new Error(`LLM generated invalid script: ${validation.error.issues.map((i) => i.message).join(", ")}`);
  }

  log.info(`Script successfully generated with ${validation.data.scenes.length} scenes.`);
  return validation.data;
}

export async function generateCaption(
  title: string,
  sourceUrl: string,
  slug: string,
  cfg: Config
): Promise<string> {
  const channelTag = (cfg.tiktok.handle || "@leviatech").replace(/^@/, "").toLowerCase();
  const sourceLine = sourceUrl && sourceUrl.startsWith("http") ? `\n\n🔗 Link dự án: ${sourceUrl}` : "";

  try {
    const resp = await axios.post(
      `${cfg.llm.baseUrl}/chat/completions`,
      {
        model: cfg.llm.model,
        messages: [
          {
            role: "system",
            content: `Viết 1 câu caption ngắn (~10-20 từ) tiếng Việt thật cuốn hút kèm chính xác 4 hashtag cho video TikTok/Reels về công nghệ.
Định dạng trả về:
<dòng caption ngắn gọn, văn nói, có thể có 1 emoji>

#tag1 #tag2 #tag3 #tag4`,
          },
          {
            role: "user",
            content: `Tiêu đề video: ${title}\nKênh: ${cfg.tiktok.displayName}`,
          },
        ],
        temperature: 0.7,
      },
      {
        headers: {
          Authorization: `Bearer ${cfg.llm.apiKey}`,
          "Content-Type": "application/json",
        },
        timeout: 20000,
      }
    );

    const text = resp.data?.choices?.[0]?.message?.content?.trim();
    if (text && text.includes("#")) {
      const parts = text.split(/\n\s*\n/);
      if (parts.length >= 2) {
        const headline = parts.slice(0, -1).join("\n\n");
        const tags = parts[parts.length - 1];
        return `${headline}${sourceLine}\n\n${tags}`;
      }
      return `${text}${sourceLine}`;
    }
  } catch (e: any) {
    log.warn(`Caption generation failed: ${e.message}, using fallback.`);
  }

  return `${title} - Cùng khám phá ngay! 🔥${sourceLine}\n\n#congnghe #ai #${channelTag} #xuhuong`;
}
