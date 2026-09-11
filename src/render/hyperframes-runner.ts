import { spawn } from "node:child_process";
import { log } from "../utils/logger.js";

export interface RenderArgs {
  compositionDir: string;  // path to composition directory
  outputPath: string;      // path for .mp4
  fps?: number;            // default 30
  quality?: "draft" | "standard" | "high"; // default "standard"
}

export async function renderWithHyperframes(args: RenderArgs): Promise<void> {
  const { compositionDir, outputPath, fps = 30, quality = "standard" } = args;

  const isWin = process.platform === "win32";
  const cmd = isWin ? "npx.cmd" : "npx";
  const spawnArgs = [
    "hyperframes",
    "render",
    compositionDir,
    "--output",
    outputPath,
    "--fps",
    String(fps),
    "--quality",
    quality,
  ];

  await new Promise<void>((resolve, reject) => {
    const proc = spawn(cmd, spawnArgs, {
      stdio: ["ignore", "inherit", "inherit"],
      shell: false,
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

    proc.on("error", () => {
      // Fallback with shell: true and quoted arguments if direct npx.cmd fails
      const quotedArgs = spawnArgs.map((a) => (a.includes(" ") ? `"${a}"` : a));
      const fallback = spawn("npx", quotedArgs, {
        stdio: ["ignore", "inherit", "inherit"],
        shell: true,
      });

      fallback.on("close", (code) => {
        if (code === 0) resolve();
        else reject(new Error(`hyperframes render failed with exit code ${code}`));
      });
      fallback.on("error", (err) => reject(err));
    });
  });

  log.info(`Rendered: ${outputPath}`);
}
