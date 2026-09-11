import axios from "axios";
import { readFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { log } from "../utils/logger.js";

export interface ScrapedContent {
  title: string;
  content: string;
  ogImage: string | null;
  domain: string;
  displaySource: string;
  url: string;
  slug: string;
}

export function generateSlug(title: string): string {
  // Strip Vietnamese diacritics
  const normalized = title
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/[đĐ]/g, "d")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40)
    .replace(/-+$/, "");

  return normalized || "video-news";
}

export async function fetchContent(input: string): Promise<ScrapedContent> {
  const isUrl = input.startsWith("http://") || input.startsWith("https://");

  if (!isUrl) {
    if (!existsSync(input)) {
      throw new Error(`File not found: ${input}`);
    }
    const raw = await readFile(input, "utf8");
    const lines = raw.split(/\r?\n/).map((l) => l.trim()).filter(Boolean);
    if (lines.length === 0) {
      throw new Error(`Input file is empty: ${input}`);
    }
    const title = lines[0].slice(0, 80);
    const content = lines.slice(1).join("\n\n") || title;
    return {
      title,
      content,
      ogImage: null,
      domain: "local",
      displaySource: "Tệp tin cục bộ",
      url: input,
      slug: generateSlug(title),
    };
  }

  const urlObj = new URL(input);
  const domain = urlObj.hostname.replace(/^www\./, "");

  // Specialized handler for GitHub repos
  if (domain === "github.com") {
    const parts = urlObj.pathname.split("/").filter(Boolean);
    if (parts.length >= 2) {
      const owner = parts[0];
      const repo = parts[1];
      log.info(`Detected GitHub repo: ${owner}/${repo}`);

      const ogImage = `https://opengraph.githubassets.com/1/${owner}/${repo}`;
      let readme = "";
      let repoDesc = "";

      // 1. Try official GitHub API for raw readme
      try {
        const readmeResp = await axios.get<string>(
          `https://api.github.com/repos/${owner}/${repo}/readme`,
          {
            timeout: 10000,
            headers: {
              "User-Agent": "AutoVideoGen/2.0",
              Accept: "application/vnd.github.raw",
            },
          }
        );
        if (readmeResp.status === 200 && typeof readmeResp.data === "string") {
          readme = readmeResp.data;
        }
      } catch (e: any) {
        log.warn(`GitHub API readme fetch failed (${e.message}), trying raw branch fallback`);
      }

      // Also try fetching repo details (description, stars)
      try {
        const repoResp = await axios.get<{ description?: string }>(
          `https://api.github.com/repos/${owner}/${repo}`,
          {
            timeout: 10000,
            headers: { "User-Agent": "AutoVideoGen/2.0" },
          }
        );
        if (repoResp.data?.description) {
          repoDesc = repoResp.data.description;
        }
      } catch {
        // ignore
      }

      // 2. Fallback: try raw.githubusercontent with branch and filename permutations
      if (!readme) {
        const branches = ["main", "master", "HEAD"];
        const files = ["README.md", "readme.md", "readme_en.md", "README_vi.md", "readme_vi.md"];
        for (const branch of branches) {
          if (readme) break;
          for (const file of files) {
            try {
              const rawUrl = `https://raw.githubusercontent.com/${owner}/${repo}/${branch}/${file}`;
              const resp = await axios.get<string>(rawUrl, { timeout: 8000 });
              if (resp.status === 200 && typeof resp.data === "string" && resp.data.length > 50) {
                readme = resp.data;
                break;
              }
            } catch {
              // continue
            }
          }
        }
      }

      const cleanContent = stripMarkdown(readme).slice(0, 3500);
      const title = repoDesc ? `${repo}: ${repoDesc}` : `${repo} - Dự án mã nguồn mở ${owner}`;
      const displaySource = `github.com/${owner}/${repo}`;

      return {
        title: title.slice(0, 80),
        content: cleanContent || repoDesc || `Dự án mã nguồn mở ${repo} của tác giả ${owner}`,
        ogImage,
        domain: displaySource,
        displaySource,
        url: input,
        slug: generateSlug(repo),
      };
    }
  }

  // General web page scraping
  log.info(`Fetching web article: ${input}`);
  const resp = await axios.get<string>(input, {
    timeout: 15000,
    headers: {
      "User-Agent":
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
      Accept: "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
      "Accept-Language": "vi,en-US;q=0.9,en;q=0.8",
    },
  });

  const html = resp.data;

  // Extract metadata
  const titleMatch =
    html.match(/<meta\s+property=["']og:title["']\s+content=["']([^"']+)["']/i) ||
    html.match(/<meta\s+name=["']twitter:title["']\s+content=["']([^"']+)["']/i) ||
    html.match(/<title[^>]*>([^<]+)<\/title>/i);
  const title = titleMatch ? decodeHtmlEntities(titleMatch[1].trim()) : domain;

  const imageMatch =
    html.match(/<meta\s+property=["']og:image["']\s+content=["']([^"']+)["']/i) ||
    html.match(/<meta\s+name=["']twitter:image["']\s+content=["']([^"']+)["']/i);
  let ogImage = imageMatch ? imageMatch[1].trim() : null;
  if (ogImage && !ogImage.startsWith("http")) {
    try {
      ogImage = new URL(ogImage, input).toString();
    } catch {
      ogImage = null;
    }
  }

  const cleanText = cleanHtmlToText(html).slice(0, 3500);

  return {
    title,
    content: cleanText,
    ogImage,
    domain,
    displaySource: domain,
    url: input,
    slug: generateSlug(title),
  };
}

function cleanHtmlToText(html: string): string {
  return html
    .replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, "")
    .replace(/<style\b[^<]*(?:(?!<\/style>)<[^<]*)*<\/style>/gi, "")
    .replace(/<nav\b[^<]*(?:(?!<\/nav>)<[^<]*)*<\/nav>/gi, "")
    .replace(/<footer\b[^<]*(?:(?!<\/footer>)<[^<]*)*<\/footer>/gi, "")
    .replace(/<header\b[^<]*(?:(?!<\/header>)<[^<]*)*<\/header>/gi, "")
    .replace(/<[^>]+>/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

function stripMarkdown(md: string): string {
  return md
    .replace(/```[\s\S]*?```/g, "")
    .replace(/`([^`]+)`/g, "$1")
    .replace(/!\[.*?\]\(.*?\)/g, "")
    .replace(/\[(.*?)\]\(.*?\)/g, "$1")
    .replace(/#{1,6}\s+/g, "")
    .replace(/[*_~]{1,3}/g, "")
    .replace(/https?:\/\/\S+/g, "") // remove URLs from prompt text to prevent AI triggering URL safety
    .replace(/\n\s*\n/g, "\n\n")
    .trim();
}

function decodeHtmlEntities(str: string): string {
  return str
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'");
}
