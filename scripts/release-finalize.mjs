#!/usr/bin/env node

import { createHash } from "node:crypto";
import { lstat, readFile, readdir, writeFile } from "node:fs/promises";
import path from "node:path";
import Ajv from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const REPOSITORY = "braidenm/home-lab-observer";
const POLICY = "UNSIGNED_PREVIEW_WITH_CHECKSUMS_AND_PROVENANCE";
const SCHEMA_V1 = "observer-release/v1";
const SCHEMA_V2 = "observer-release/v2";
const PLATFORMS = [
  ["linux", "amd64", "tar.gz"],
  ["linux", "arm64", "tar.gz"],
  ["darwin", "amd64", "tar.gz"],
  ["darwin", "arm64", "tar.gz"],
  ["windows", "amd64", "zip"],
  ["windows", "arm64", "zip"],
];

function fail(message) {
  throw new Error(`release finalization rejected: ${message}`);
}

function parseArgs(argv) {
  const values = new Map();
  for (let index = 0; index < argv.length; index += 2) {
    const name = argv[index];
    const value = argv[index + 1];
    if (!name?.startsWith("--") || value === undefined || value.startsWith("--") || values.has(name)) {
      fail(`invalid arguments ${JSON.stringify(argv)}`);
    }
    values.set(name, value);
  }
  for (const name of values.keys()) {
    if (!["--version", "--commit", "--directory", "--schema"].includes(name)) fail(`unknown argument ${name}`);
  }
  return values;
}

function exactKeys(value, expected, context) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) fail(`${context} must be an object`);
  const actual = Object.keys(value).sort();
  const wanted = [...expected].sort();
  if (actual.join("\0") !== wanted.join("\0")) {
    fail(`${context} keys must be exactly ${wanted.join(", ")}; received ${actual.join(", ")}`);
  }
}

async function hashFile(filename) {
  const bytes = await readFile(filename);
  return { sha256: createHash("sha256").update(bytes).digest("hex"), size: bytes.length };
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const version = args.get("--version") ?? fail("--version is required");
  const commit = args.get("--commit") ?? fail("--commit is required");
  const directory = path.resolve(args.get("--directory") ?? fail("--directory is required"));
  const schemaPath = path.resolve(args.get("--schema") ?? fail("--schema is required"));
  if (!/^0\.1\.0-preview\.[1-9][0-9]*$/.test(version)) fail("invalid preview version");
  if (!/^[a-f0-9]{40}$/.test(commit)) fail("invalid full commit SHA");

  const archiveNames = PLATFORMS.map(
    ([os, arch, format]) => `home-lab-observer_${version}_${os}_${arch}.${format}`,
  );
  const payloadNames = [
    ...archiveNames,
    "release-manifest.json",
    "install.sh",
    "install.ps1",
    "observer.spdx.json",
  ].sort();

  const entries = (await readdir(directory)).sort();
  const entriesWithoutProvisionalSums = entries.filter((entry) => entry !== "SHA256SUMS");
  if (entriesWithoutProvisionalSums.join("\0") !== payloadNames.join("\0")) {
    fail(`payload must contain exactly ${payloadNames.join(", ")}; received ${entries.join(", ")}`);
  }
  for (const name of entriesWithoutProvisionalSums) {
    const stat = await lstat(path.join(directory, name));
    if (!stat.isFile() || stat.isSymbolicLink()) fail(`${name} must be a regular file`);
    const maximum = archiveNames.includes(name) ? 220 * 1024 * 1024 : 1024 * 1024;
    if (stat.size < 1 || stat.size > maximum) fail(`${name} has an invalid size of ${stat.size} bytes`);
  }
  if (entries.includes("SHA256SUMS")) {
    const provisional = await lstat(path.join(directory, "SHA256SUMS"));
    if (!provisional.isFile() || provisional.isSymbolicLink() || provisional.size > 1024 * 1024) {
      fail("provisional SHA256SUMS must be a bounded regular file");
    }
  }

  const manifest = JSON.parse(await readFile(path.join(directory, "release-manifest.json"), "utf8"));
  const schema = JSON.parse(await readFile(schemaPath, "utf8"));
  const ajv = new Ajv({ allErrors: true, strict: false });
  addFormats(ajv);
  if (schema.$id !== "urn:home-lab-observer:schema:release:v1") {
    const v1Schema = JSON.parse(await readFile(new URL("../schemas/release-v1.schema.json", import.meta.url), "utf8"));
    ajv.addSchema(v1Schema);
  }
  const validate = ajv.compile(schema);
  if (!validate(manifest)) fail(`manifest does not satisfy its JSON Schema: ${ajv.errorsText(validate.errors)}`);
  exactKeys(manifest, ["schema_version", "version", "tag", "repository", "commit_sha", "signing_policy", "assets"], "manifest");
  const schemaProfiles = new Map([
    ["urn:home-lab-observer:schema:release:v1", SCHEMA_V1],
    ["urn:home-lab-observer:schema:release:v2", SCHEMA_V2],
  ]);
  if (schemaProfiles.has(schema.$id) && manifest.schema_version !== schemaProfiles.get(schema.$id)) {
    fail("manifest schema_version does not match the selected schema");
  }
  if (![SCHEMA_V1, SCHEMA_V2].includes(manifest.schema_version)) fail("manifest schema_version is unsupported");
  const expectedTopLevel = {
    version,
    tag: `v${version}`,
    repository: REPOSITORY,
    commit_sha: commit,
    signing_policy: POLICY,
  };
  for (const [key, expected] of Object.entries(expectedTopLevel)) {
    if (manifest[key] !== expected) fail(`manifest ${key} must be ${expected}`);
  }
  if (!Array.isArray(manifest.assets) || manifest.assets.length !== PLATFORMS.length) {
    fail("manifest must contain exactly six native assets");
  }

  const seen = new Set();
  for (const asset of manifest.assets) {
    const assetKeys = ["os", "arch", "filename", "sha256", "size_bytes", "format", "download_url"];
    if (manifest.schema_version === SCHEMA_V2) assetKeys.push("content_profile", "journal_helper");
    exactKeys(asset, assetKeys, "manifest asset");
    const platform = PLATFORMS.find(([os, arch]) => os === asset.os && arch === asset.arch);
    if (!platform) fail(`unsupported manifest platform ${asset.os}/${asset.arch}`);
    const format = platform[2];
    const filename = `home-lab-observer_${version}_${asset.os}_${asset.arch}.${format}`;
    const platformKey = `${asset.os}/${asset.arch}`;
    if (seen.has(platformKey)) fail(`duplicate manifest platform ${platformKey}`);
    seen.add(platformKey);
    if (asset.filename !== filename || asset.format !== format) fail(`incorrect filename or format for ${platformKey}`);
    const expectedURL = `https://github.com/${REPOSITORY}/releases/download/v${version}/${filename}`;
    if (asset.download_url !== expectedURL) fail(`download URL for ${platformKey} is not immutable and version-pinned`);
    if (manifest.schema_version === SCHEMA_V2) {
      const linux = asset.os === "linux";
      if (asset.content_profile !== (linux ? "linux-journal-helper-v1" : "native-core-v1")) {
        fail(`incorrect content profile for ${platformKey}`);
      }
      if (linux) {
        exactKeys(asset.journal_helper, ["filename", "sha256", "size_bytes"], "manifest journal helper");
        if (asset.journal_helper.filename !== "observer-journal-helper" ||
            !/^[a-f0-9]{64}$/.test(asset.journal_helper.sha256) ||
            !Number.isSafeInteger(asset.journal_helper.size_bytes) || asset.journal_helper.size_bytes < 1 ||
            asset.journal_helper.size_bytes > 200 * 1024 * 1024) {
          fail(`invalid journal helper metadata for ${platformKey}`);
        }
      } else if (asset.journal_helper !== null) {
        fail(`journal helper metadata is forbidden for ${platformKey}`);
      }
    }
    const actual = await hashFile(path.join(directory, filename));
    if (asset.sha256 !== actual.sha256 || asset.size_bytes !== actual.size) {
      fail(`manifest digest or size does not match final ${filename} bytes`);
    }
  }

  const sbom = JSON.parse(await readFile(path.join(directory, "observer.spdx.json"), "utf8"));
  if (typeof sbom.spdxVersion !== "string" || !sbom.spdxVersion.startsWith("SPDX-2.")) fail("SBOM must be SPDX JSON");
  if (sbom.dataLicense !== "CC0-1.0" || !Array.isArray(sbom.packages)) fail("SBOM is missing SPDX package data");

  const checksumLines = [];
  for (const name of payloadNames) {
    const { sha256 } = await hashFile(path.join(directory, name));
    checksumLines.push(`${sha256}  ${name}`);
  }
  await writeFile(path.join(directory, "SHA256SUMS"), `${checksumLines.join("\n")}\n`, { encoding: "utf8", mode: 0o644 });

  const finalEntries = (await readdir(directory)).sort();
  const expectedFinalEntries = [...payloadNames, "SHA256SUMS"].sort();
  if (finalEntries.join("\0") !== expectedFinalEntries.join("\0")) fail("final release file allowlist changed during finalization");
  console.log(`finalized ${payloadNames.length} downloadable files plus SHA256SUMS`);
}

main().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
