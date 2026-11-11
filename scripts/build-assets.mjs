import { createHash } from "node:crypto";
import { copyFile, mkdir, readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

process.chdir(fileURLToPath(new URL("../", import.meta.url)));
await mkdir("static/vendor/licenses", { recursive: true });

// Preserve the existing Chart.js version and omit the development source map URL.
const chart = await readFile("node_modules/chart.js/dist/chart.umd.js", "utf8");
await writeFile("static/vendor/chart.umd.js", chart.replace(/^\/\/# sourceMappingURL=.*\r?\n?/gm, ""));

for (const [name, source] of Object.entries({
  "tailwindcss.txt": "node_modules/tailwindcss/LICENSE",
  "daisyui.txt": "node_modules/daisyui/LICENSE",
  "chart.js.txt": "node_modules/chart.js/LICENSE.md",
  "kurkle-color.txt": "node_modules/@kurkle/color/LICENSE.md",
})) {
  await copyFile(source, `static/vendor/licenses/${name}`);
}

const files = ["chart.umd.js", "ui.css", "licenses/chart.js.txt", "licenses/daisyui.txt", "licenses/kurkle-color.txt", "licenses/tailwindcss.txt"];
const sums = await Promise.all(files.map(async (file) => {
  const hash = createHash("sha256").update(await readFile(`static/vendor/${file}`)).digest("hex");
  return `${hash}  ${file}\n`;
}));
await writeFile("static/vendor/SHA256SUMS", sums.join(""));
