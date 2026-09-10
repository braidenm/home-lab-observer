package releasepack

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

// DecodeManifest validates the closed, bounded v1/v2 wire shape. It does not
// authenticate the manifest or inspect archives; callers must do both.
func DecodeManifest(payload []byte) (Manifest, error) {
	invalid := errors.New("MANIFEST_INVALID")
	if len(payload) == 0 || len(payload) > 1<<20 || !utf8.Valid(payload) {
		return Manifest{}, invalid
	}
	d := json.NewDecoder(bytes.NewReader(payload))
	d.UseNumber()
	if strictJSONValue(d, 0) != nil {
		return Manifest{}, invalid
	}
	if _, err := d.Token(); err != io.EOF {
		return Manifest{}, invalid
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(payload, &root) != nil || !exactKeys(root, "schema_version", "version", "tag", "repository", "commit_sha", "signing_policy", "assets") {
		return Manifest{}, invalid
	}
	var schema string
	if json.Unmarshal(root["schema_version"], &schema) != nil || (schema != SchemaVersion && schema != SchemaVersionV2) {
		return Manifest{}, invalid
	}
	var assets []map[string]json.RawMessage
	if json.Unmarshal(root["assets"], &assets) != nil || len(assets) != 6 {
		return Manifest{}, invalid
	}
	for _, asset := range assets {
		keys := []string{"os", "arch", "filename", "sha256", "size_bytes", "format", "download_url"}
		if schema == SchemaVersionV2 {
			keys = append(keys, "content_profile", "journal_helper")
		}
		if !exactKeys(asset, keys...) {
			return Manifest{}, invalid
		}
		if schema == SchemaVersionV2 && !bytes.Equal(bytes.TrimSpace(asset["journal_helper"]), []byte("null")) {
			var helper map[string]json.RawMessage
			if json.Unmarshal(asset["journal_helper"], &helper) != nil || !exactKeys(helper, "filename", "sha256", "size_bytes") {
				return Manifest{}, invalid
			}
		}
	}
	var manifest Manifest
	if json.Unmarshal(payload, &manifest) != nil || validateManifestShape(manifest) != nil {
		return Manifest{}, invalid
	}
	return manifest, nil
}

func exactKeys(value map[string]json.RawMessage, names ...string) bool {
	if len(value) != len(names) {
		return false
	}
	for _, name := range names {
		if _, ok := value[name]; !ok {
			return false
		}
	}
	return true
}

func strictJSONValue(d *json.Decoder, depth int) error {
	if depth > 6 {
		return errors.New("depth")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("key")
			}
			seen[name] = true
			if err := strictJSONValue(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("object")
		}
	case '[':
		for d.More() {
			if err := strictJSONValue(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("array")
		}
	default:
		return errors.New("delimiter")
	}
	return nil
}

func validateManifestShape(manifest Manifest) error {
	invalid := errors.New("MANIFEST_INVALID")
	if (manifest.SchemaVersion != SchemaVersion && manifest.SchemaVersion != SchemaVersionV2) || !validVersion(manifest.Version) || manifest.Tag != "v"+manifest.Version || manifest.Repository != Repository || !commitPattern.MatchString(manifest.CommitSHA) || manifest.SigningPolicy != SigningPolicy || len(manifest.Assets) != len(targets) {
		return invalid
	}
	for index, target := range targets {
		asset := manifest.Assets[index]
		filename := "home-lab-observer_" + manifest.Version + "_" + target.os + "_" + target.arch + "." + target.format
		url := "https://github.com/" + Repository + "/releases/download/" + manifest.Tag + "/" + filename
		if asset.OS != target.os || asset.Arch != target.arch || asset.Format != target.format || asset.Filename != filename || asset.DownloadURL != url || !shaPattern(asset.SHA256) || asset.SizeBytes <= 0 || asset.SizeBytes > maxArchiveSize {
			return invalid
		}
		if manifest.SchemaVersion == SchemaVersion {
			if asset.ContentProfile != "" || asset.JournalHelper != nil {
				return invalid
			}
		} else if target.os == "linux" {
			h := asset.JournalHelper
			if asset.ContentProfile != "linux-journal-helper-v1" || h == nil || h.Filename != "observer-journal-helper" || !shaPattern(h.SHA256) || h.SizeBytes <= 0 || h.SizeBytes > maxBinarySize {
				return invalid
			}
		} else if asset.ContentProfile != "native-core-v1" || asset.JournalHelper != nil {
			return invalid
		}
	}
	return nil
}
