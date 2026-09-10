import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import Ajv2020 from "ajv/dist/2020.js";
import { validReleaseV2, releaseV2ArchiveEntries } from "./release-v2-contract.mjs";

const schema = JSON.parse(readFileSync(new URL("../schemas/release-v1.schema.json", import.meta.url), "utf8"));
const validate = new Ajv2020({ allErrors: true, strict: false }).compile(schema);

function manifest(version = "0.1.0-preview.1") {
  return {
    schema_version: "observer-release/v1", version, tag: `v${version}`,
    repository: "braidenm/home-lab-observer", commit_sha: "a".repeat(40),
    signing_policy: "UNSIGNED_PREVIEW_WITH_CHECKSUMS_AND_PROVENANCE",
    assets: ["linux", "darwin", "windows"].flatMap((os) => ["amd64", "arm64"].map((arch) => {
      const format = os === "windows" ? "zip" : "tar.gz";
      const filename = `home-lab-observer_${version}_${os}_${arch}.${format}`;
      return { os, arch, filename, sha256: "b".repeat(64), size_bytes: 1234, format,
        download_url: `https://github.com/braidenm/home-lab-observer/releases/download/v${version}/${filename}` };
    })),
  };
}

test("closed six-platform release schema accepts supported prerelease identities", () => {
  for (const version of ["0.1.0-preview.1", "1.2.3-rc.0", "1.2.3-01alpha"]) {
    assert.equal(validate(manifest(version)), true, JSON.stringify(validate.errors));
  }
});

test("release schema rejects unsupported identities and archive bounds", () => {
  for (const version of ["v0.1.0-preview.1", "0.1.0", "0.1.0-preview.01", "00.1.0-preview.1", "0.1.0-preview.1+build", "../escape"]) {
    assert.equal(validate(manifest(version)), false, version);
  }
  const mutations = [
    (value) => { value.secret = "not-a-real-secret"; },
    (value) => { value.assets[0].unknown = true; },
    (value) => { value.assets[0] = structuredClone(value.assets[1]); },
    (value) => { value.assets.pop(); },
    (value) => { value.repository = "example/unapproved"; },
    (value) => { value.signing_policy = "SIGNED"; },
    (value) => { value.commit_sha = "short"; },
    (value) => { value.assets[0].size_bytes = 220 * 1024 * 1024 + 1; },
    (value) => { value.assets[0].size_bytes = 0; },
    (value) => { value.assets[0].sha256 = "x".repeat(64); },
    (value) => { value.assets[0].download_url = "https://example.invalid/observer.tar.gz"; },
    (value) => { value.assets[4].format = "tar.gz"; },
    (value) => { value.assets[0].filename = "../observer"; },
  ];
  for (const mutate of mutations) {
    const value = manifest();
    mutate(value);
    assert.equal(validate(value), false, mutate.toString());
  }
});

function manifestV2() {
  const value = manifest("0.1.0-preview.3");
  value.schema_version = "observer-release/v2";
  for (const asset of value.assets) {
    asset.content_profile = asset.os === "linux" ? "linux-journal-helper-v1" : "native-core-v1";
    asset.journal_helper = asset.os === "linux" ? {
      filename: "observer-journal-helper", sha256: "c".repeat(64), size_bytes: 4096,
    } : null;
  }
  return value;
}

test("v2 freezes exact platform content without relaxing v1 rollback", () => {
  const value = manifestV2();
  assert.equal(validReleaseV2(value), true);
  assert.equal(validate(value), false, "old schema must not silently accept helper-bearing content");
  assert.equal(validReleaseV2(manifest()), false);
  assert.equal(validate(manifest()), true, "legacy rollback contract remains valid");
  assert.deepEqual(releaseV2ArchiveEntries(value.assets[0]), ["observer", "LICENSE", "START-HERE.md", "run-observer.sh", "observer-journal-helper"]);
  assert.equal(releaseV2ArchiveEntries(value.assets[2]).length, 4);
  assert.equal(releaseV2ArchiveEntries(value.assets[4]).length, 4);
});

test("v2 rejects extra authority, missing helper, mismatched identity and bounds", () => {
  const mutations = [
    (v) => { v.assets[0].journal_helper = null; },
    (v) => { delete v.assets[0].journal_helper; },
    (v) => { v.assets[2].journal_helper = structuredClone(v.assets[0].journal_helper); },
    (v) => { v.assets[0].content_profile = "native-core-v1"; },
    (v) => { v.assets[4].content_profile = "linux-journal-helper-v1"; },
    (v) => { v.assets[0].journal_helper.filename = "../observer-journal-helper"; },
    (v) => { v.assets[0].journal_helper.command = "sh"; },
    (v) => { v.assets[0].journal_helper.sha256 = "X".repeat(64); },
    (v) => { v.assets[0].journal_helper.size_bytes = 0; },
    (v) => { v.assets[0].journal_helper.size_bytes = 200 * 1024 * 1024 + 1; },
    (v) => { v.assets[0].size_bytes = 220 * 1024 * 1024 + 1; },
    (v) => { v.assets[0].filename = v.assets[1].filename; },
    (v) => { v.assets[0].download_url = v.assets[1].download_url; },
    (v) => { v.tag = "v0.1.0-preview.4"; },
    (v) => { v.version = "0.1.0-preview.4"; },
    (v) => { v.assets[0].format = "zip"; },
    (v) => { v.assets[2] = structuredClone(v.assets[0]); },
    (v) => { v.assets.push(structuredClone(v.assets[0])); },
    (v) => { v.unknown = "PRIVATE_CANARY"; },
  ];
  for (const mutate of mutations) {
    const value = manifestV2();
    mutate(value);
    assert.equal(validReleaseV2(value), false, mutate.toString());
  }
  assert.throws(() => releaseV2ArchiveEntries({ os: "linux", content_profile: "unknown" }), /INVALID_CONTENT_PROFILE/);
});
