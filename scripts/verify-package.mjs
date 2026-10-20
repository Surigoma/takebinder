import { createHash } from "node:crypto";
import { createReadStream, existsSync, lstatSync, readFileSync, readlinkSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
const target = process.platform === "darwin"
  ? path.join(root, "release", "TakeBinder.app")
  : path.join(root, "release", `TakeBinder-${process.platform}-${process.arch}`);
const resources = process.platform === "darwin" ? path.join(target, "Contents", "Resources") : path.join(target, "resources");
for (const name of ["README.md", "README.ja.md"]) {
  if (!existsSync(path.join(resources, name))) throw new Error(`missing packaged documentation: ${name}`);
}
const manifest = JSON.parse(readFileSync(path.join(resources, "release-manifest.json"), "utf8"));
if (manifest.platform !== process.platform || manifest.arch !== process.arch) throw new Error("release manifest platform mismatch");
for (const artifact of manifest.artifacts) {
  const file = path.resolve(target, artifact.path);
  if (!file.startsWith(path.resolve(target) + path.sep) || !existsSync(file)) throw new Error(`missing artifact: ${artifact.path}`);
  const stat = lstatSync(file);
  if (artifact.link !== undefined) {
    if (!stat.isSymbolicLink() || readlinkSync(file) !== artifact.link) throw new Error(`link mismatch: ${artifact.path}`);
  } else if (!stat.isFile() || stat.size !== artifact.size || await hashFile(file) !== artifact.sha256) {
    throw new Error(`artifact mismatch: ${artifact.path}`);
  }
}
for (const name of ["ffmpeg", "ffprobe", "mp4_merge"]) {
  if (!manifest.nativeTools[name]?.version || !/^[a-f0-9]{64}$/.test(manifest.nativeTools[name]?.expectedSha256 ?? "")) throw new Error(`native tool metadata missing: ${name}`);
}
console.log(`Release manifest verified: ${manifest.artifacts.length} artifacts`);

function hashFile(file) {
  return new Promise((resolve, reject) => {
    const hash = createHash("sha256");
    createReadStream(file).on("data", (chunk) => hash.update(chunk)).once("error", reject).once("end", () => resolve(hash.digest("hex")));
  });
}
