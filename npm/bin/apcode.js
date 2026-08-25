#!/usr/bin/env node
// Shim that execs the real apcode binary downloaded by install.js
const { spawn } = require("child_process");
const path = require("path");
const fs = require("fs");

const BIN = path.join(__dirname, process.platform === "win32" ? "apcode.exe" : "apcode");
const FALLBACK_BIN = path.resolve(__dirname, "..", "..", "apcode" + (process.platform === "win32" ? ".exe" : ""));

let bin = BIN;
if (!fs.existsSync(bin) && fs.existsSync(FALLBACK_BIN)) bin = FALLBACK_BIN;

if (!fs.existsSync(bin)) {
  console.error(`APCode binary not found at ${bin}`);
  console.error("Try reinstall: npm install -g apcode-ai --force");
  console.error("Or install via: curl -fsSL https://raw.githubusercontent.com/anshulchikhale30-p/APCode/main/install.sh | bash");
  process.exit(1);
}

const child = spawn(bin, process.argv.slice(2), { stdio: "inherit" });
child.on("exit", (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  else process.exit(code ?? 1);
});
child.on("error", (err) => {
  console.error("Failed to launch apcode:", err.message);
  process.exit(1);
});
