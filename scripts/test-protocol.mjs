import { spawn } from "node:child_process";
import { createInterface } from "node:readline";
import { fileURLToPath } from "node:url";
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

const executable = fileURLToPath(new URL(`../bin/takebinder-backend${process.platform === "win32" ? ".exe" : ""}`, import.meta.url));
const fixture = mkdtempSync(path.join(tmpdir(), "takebinder-protocol-"));
const tools = path.join(fixture, "managed");
const external = path.join(fixture, "external");
mkdirSync(external);
for (const name of ["ffmpeg", "ffprobe", "mp4_merge"]) {
  const filename = process.platform === "win32" ? `${name}.exe` : name;
  mkdirSync(path.join(tools, name, "old"), { recursive: true });
  writeFileSync(path.join(tools, name, "old", filename), "managed test binary");
  writeFileSync(path.join(external, filename), "external tool must remain", { mode: 0o755 });
}
const child = spawn(executable, [], { stdio: ["pipe", "pipe", "inherit"], shell: false, windowsHide: true, env: { ...process.env, TAKEBINDER_TOOLS_DIR: tools, PATH: `${external}${path.delimiter}${process.env.PATH ?? ""}` } });
const messages = [];
createInterface({ input: child.stdout }).on("line", (line) => messages.push(JSON.parse(line)));

child.stdin.write('{broken json}\n');
child.stdin.write(`${JSON.stringify({ protocolVersion: "2", requestId: "version", type: "status", payload: {} })}\n`);
child.stdin.write(`${JSON.stringify({ protocolVersion: "1", requestId: "unknown", type: "unknown", payload: {} })}\n`);
child.stdin.write(`${JSON.stringify({ protocolVersion: "1", requestId: "remove-path", type: "remove-tools", payload: { path: external } })}\n`);
child.stdin.write(`${JSON.stringify({ protocolVersion: "1", requestId: "remove", type: "remove-tools", payload: {} })}\n`);
child.stdin.end(`${JSON.stringify({ protocolVersion: "1", requestId: "status", type: "status", payload: {} })}\n`);

const exitCode = await new Promise((resolve, reject) => {
  const timeout = setTimeout(() => { child.kill(); reject(new Error("backend protocol test timed out")); }, 5000);
  child.once("error", reject);
  child.once("exit", (code) => { clearTimeout(timeout); resolve(code); });
});

if (exitCode !== 0) throw new Error(`backend exited with ${exitCode}`);
if (messages.length !== 6) throw new Error(`expected 6 responses, got ${messages.length}`);
for (const message of messages) {
  if (message.protocolVersion !== "1" || typeof message.requestId !== "string" || typeof message.type !== "string" || !("payload" in message)) {
    throw new Error(`invalid envelope: ${JSON.stringify(message)}`);
  }
}
const byRequest = new Map(messages.map((message) => [message.requestId, message]));
if (messages[0].type !== "error" || messages[0].payload.code !== "E_BAD_REQUEST") throw new Error("malformed JSON contract failed");
if (byRequest.get("version")?.payload.code !== "E_PROTOCOL") throw new Error("version contract failed");
if (byRequest.get("unknown")?.payload.code !== "E_BAD_REQUEST") throw new Error("unknown command contract failed");
if (byRequest.get("remove-path")?.payload.code !== "E_BAD_REQUEST") throw new Error("arbitrary removal path accepted");
if (byRequest.get("remove")?.type !== "tools.remove.completed") throw new Error("remove command contract failed");
for (const name of ["ffmpeg", "ffprobe", "mp4_merge"]) {
  const status = byRequest.get("remove")?.payload.tools[name];
  if (status?.removable !== false || status?.available !== true) throw new Error("removal did not retain external tools");
  const filename = process.platform === "win32" ? `${name}.exe` : name;
  if (readFileSync(path.join(external, filename), "utf8") !== "external tool must remain") throw new Error("external tool changed");
}
if (byRequest.get("status")?.type !== "status.completed") throw new Error("status contract failed after malformed input");

console.log("JSON Lines protocol contract: OK");
