#!/usr/bin/env node
// APCode npm installer — downloads the right binary from GitHub Releases
// Mirrors opencode-ai npm install but for APCode (Go binary).

const fs = require("fs");
const path = require("path");
const https = require("https");
const { execSync } = require("child_process");

const REPO = "apcode/apcode";
const APP = "apcode";
// Version is replaced at publish time; fallback to latest via API if 0.0.0
let VERSION = "0.1.0";
try {
  const pkg = require("./package.json");
  VERSION = pkg.version || VERSION;
} catch {}

const BIN_DIR = path.join(__dirname, "bin");
const BIN_PATH = path.join(BIN_DIR, process.platform === "win32" ? "apcode.exe" : "apcode");

function getPlatform() {
  const plat = process.platform;
  const arch = process.arch;
  let os = null;
  let cpu = null;
  if (plat === "linux") os = "linux";
  else if (plat === "darwin") os = "darwin";
  else if (plat === "win32") os = "windows";
  else throw new Error(`Unsupported OS: ${plat}`);
  if (arch === "x64") cpu = "amd64";
  else if (arch === "arm64") cpu = "arm64";
  else throw new Error(`Unsupported arch: ${arch} (supported: x64, arm64)`);
  return { os, cpu };
}

function getUrl(os, cpu, version) {
  const ext = os === "windows" ? ".zip" : ".tar.gz";
  // Try GoReleaser pattern first, then fallback
  return `https://github.com/${REPO}/releases/download/v${version}/apcode_${version}_${os}_${cpu}${ext}`;
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    const req = https.get(url, { headers: { "User-Agent": "apcode-installer" } }, (res) => {
      if (res.statusCode === 302 || res.statusCode === 301) {
        // follow redirect
        https.get(res.headers.location, (r2) => {
          if (r2.statusCode !== 200) return reject(new Error(`HTTP ${r2.statusCode} for ${url}`));
          r2.pipe(file);
          file.on("finish", () => file.close(resolve));
        }).on("error", reject);
        return;
      }
      if (res.statusCode !== 200) return reject(new Error(`HTTP ${res.statusCode} for ${url}: ${res.statusMessage}`));
      res.pipe(file);
      file.on("finish", () => file.close(resolve));
    });
    req.on("error", reject);
    file.on("error", reject);
  });
}

async function extract(archive, outDir) {
  if (archive.endsWith(".zip")) {
    // Use PowerShell Expand-Archive on Windows, unzip elsewhere
    if (process.platform === "win32") {
      execSync(`powershell -Command "Expand-Archive -Path '${archive}' -DestinationPath '${outDir}' -Force"`, { stdio: "inherit" });
    } else {
      execSync(`unzip -o "${archive}" -d "${outDir}"`, { stdio: "inherit" });
    }
  } else {
    execSync(`tar -xzf "${archive}" -C "${outDir}"`, { stdio: "inherit" });
  }
}

async function main() {
  // Skip download if binary already present (for local dev)
  if (process.env.APCODE_SKIP_DOWNLOAD === "1") {
    console.log("APCODE_SKIP_DOWNLOAD=1, skipping binary download");
    return;
  }

  const { os, cpu } = getPlatform();
  const ext = os === "windows" ? ".zip" : ".tar.gz";
  const url = getUrl(os, cpu, VERSION);
  const fallbackUrl = `https://github.com/${REPO}/releases/download/v${VERSION}/apcode-${os}-${cpu}${ext}`;

  if (!fs.existsSync(BIN_DIR)) fs.mkdirSync(BIN_DIR, { recursive: true });

  // If binary already exists and version matches, skip
  if (fs.existsSync(BIN_PATH)) {
    try {
      const out = execSync(`"${BIN_PATH}" --version`, { encoding: "utf8" }).trim();
      if (out.includes(VERSION)) {
        console.log(`apcode ${VERSION} already installed at ${BIN_PATH}`);
        return;
      }
    } catch {}
  }

  const tmpArchive = path.join(BIN_DIR, `apcode_${VERSION}_${os}_${cpu}${ext}`);
  console.log(`Downloading apcode v${VERSION} for ${os}/${cpu}...`);
  console.log(`  ${url}`);

  let ok = false;
  for (const u of [url, fallbackUrl]) {
    try {
      await download(u, tmpArchive);
      ok = true;
      break;
    } catch (e) {
      console.warn(`  failed: ${e.message}`);
      console.warn(`  trying fallback: ${fallbackUrl === u ? "(already tried fallback)" : fallbackUrl}`);
      if (u === fallbackUrl) throw e;
    }
  }
  if (!ok) throw new Error("Download failed");

  console.log(`Extracting ${tmpArchive}...`);
  const tmpExtract = path.join(BIN_DIR, "_extract");
  if (!fs.existsSync(tmpExtract)) fs.mkdirSync(tmpExtract, { recursive: true });
  await extract(tmpArchive, tmpExtract);

  // Find binary
  const findBin = (dir) => {
    const entries = fs.readdirSync(dir, { withFileTypes: true });
    for (const e of entries) {
      const full = path.join(dir, e.name);
      if (e.isFile() && (e.name === "apcode" || e.name === "apcode.exe")) return full;
      if (e.isDirectory()) {
        const found = findBin(full);
        if (found) return found;
      }
    }
    return null;
  };
  const src = findBin(tmpExtract) || findBin(BIN_DIR);
  if (!src) throw new Error(`Binary not found in archive ${tmpArchive}. Contents: ${fs.readdirSync(tmpExtract)}`);

  fs.copyFileSync(src, BIN_PATH);
  if (process.platform !== "win32") fs.chmodSync(BIN_PATH, 0o755);

  // Cleanup
  try { fs.rmSync(tmpExtract, { recursive: true, force: true }); } catch {}
  try { fs.unlinkSync(tmpArchive); } catch {}

  console.log(`✓ Installed apcode to ${BIN_PATH}`);
  try {
    const ver = execSync(`"${BIN_PATH}" --version`, { encoding: "utf8" }).trim();
    console.log(`✓ Verified: ${ver}`);
  } catch {}
  console.log("\nRun: apcode --help");
}

main().catch((e) => {
  console.error("APCode install failed:", e.message);
  console.error("Try manual install: https://github.com/apcode/apcode#install");
  console.error("Or: curl -fsSL https://raw.githubusercontent.com/apcode/apcode/main/install.sh | bash");
  process.exit(1);
});
