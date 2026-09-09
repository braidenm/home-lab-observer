import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import Ajv2020 from "ajv/dist/2020.js";

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
