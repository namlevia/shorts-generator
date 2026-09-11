import { spawn } from "node:child_process";
import { relative } from "node:path";
import { log } from "../utils/logger.js";

export interface RenderArgs {
  compositionDir: string;  // path to composition directory
  outputPath: string;      // path for .mp4
  fps?: number;            // default 30
  quality?: "draft" | "standard" | "high"; // default "standard"
}

export async function renderWithHyperframes(args: RenderArgs): Promise<void> {
  const { compositionDir, outputPath, fps = 30, quality = "standard" } = args;

  // Make paths relative to cwd to avoid spaces in drive path (e.g. "F:\auto gen video")
  const relCompDir = relative(process.cwd(), compositionDir) || compositionDir;
  const relOutPath = relative(process.cwd(), outputPath) || outputPath;

  const spawnArgs = [
    "hyperframes",
    "render",
    relCompDir,
    "--output",
    relOutPath,
    "--fps",
    String(fps),
    "--quality",
    quality,
  ];

  await new Promise<void>((resolve, reject) => {
    const proc = spawn("npx", spawnArgs, {
      stdio: ["ignore", "inherit", "inherit"],
      shell: true,
    });

    proc.on("close", (code) => {
      if (code === 0) {
        resolve();
      } else {
        reject(
          new Error(
            `hyperframes render failed with exit code ${code}`
          )
        );
      }
    });

    proc.on("error", (err) => {
      reject(err);
    });
  });

  log.info(`Rendered: ${outputPath}`);
}
