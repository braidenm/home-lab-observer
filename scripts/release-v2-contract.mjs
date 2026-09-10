// Contract checkpoint only: release publication switches to v2 after helper integration.
import { readFileSync } from "node:fs";
import Ajv2020 from "ajv/dist/2020.js";

const readSchema = (name) => JSON.parse(readFileSync(new URL(`../schemas/${name}.schema.json`, import.meta.url), "utf8"));
const ajv = new Ajv2020({ allErrors: true, strict: false });
ajv.addSchema(readSchema("release-v1"));
const validateShape = ajv.compile(readSchema("release-v2"));

export function validReleaseV2(value) {
  if (!validateShape(value) || value.tag !== `v${value.version}`) return false;
  return value.assets.every((asset) => {
    const filename = `home-lab-observer_${value.version}_${asset.os}_${asset.arch}.${asset.format}`;
    return asset.filename === filename && asset.download_url ===
      `https://github.com/braidenm/home-lab-observer/releases/download/${value.tag}/${filename}`;
  });
}

export function releaseV2ArchiveEntries(asset) {
  // Do not use this with an unvalidated manifest.
  if (!asset || !["linux", "darwin", "windows"].includes(asset.os)) throw new Error("INVALID_CONTENT_PROFILE");
  const expectedProfile = asset.os === "linux" ? "linux-journal-helper-v1" : "native-core-v1";
  if (asset.content_profile !== expectedProfile) throw new Error("INVALID_CONTENT_PROFILE");
  if (asset.os === "linux") return ["observer", "LICENSE", "START-HERE.md", "run-observer.sh", "observer-journal-helper"];
  if (asset.os === "darwin") return ["observer", "LICENSE", "START-HERE.md", "Run-Observer.command"];
  return ["observer.exe", "LICENSE", "START-HERE.md", "Run-Observer.cmd"];
}
