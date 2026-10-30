"""Closed preset compatibility tests; no provider, candidate build or execution."""
import dataclasses
import hashlib
import inspect
import unittest

import _connected_candidate_core as core
import connected_candidate as numeric
import connected_docker_candidate as docker
from test_connected_candidate import fixture_metadata


class PresetTests(unittest.TestCase):
    def test_existing_numeric_wire_and_public_signatures_are_unchanged(self):
        raw = numeric.encode_metadata(fixture_metadata())
        self.assertEqual(hashlib.sha256(raw).hexdigest(), 'd53709af7315321a622170312d9e8f7f9c5ed8f4585fd0cfa699ab1dd15604ba')
        self.assertEqual(str(inspect.signature(numeric.validate_metadata)), '(metadata, expected_workflow_id=None)')
        self.assertEqual(str(inspect.signature(numeric.validate_payload_bytes)), '(contents, metadata)')
        self.assertEqual(numeric.WORKFLOW_PATH, '.github/workflows/connected-linux-candidate.yml')
        self.assertEqual(numeric.asset_names('0.1.0-canary.1')[2], 'install-connected-linux.sh')

    def test_only_two_actual_immutable_presets_are_admitted(self):
        for preset in (core._NUMERIC, core._DOCKER):
            core._preset(preset)
            with self.assertRaises(dataclasses.FrozenInstanceError):
                preset.schema = 'changed'
            with self.assertRaises(ValueError):
                core._preset(dataclasses.replace(preset))
        for preset in (None, {}, 'docker'):
            with self.assertRaises(ValueError): core._preset(preset)

    def test_docker_contract_cannot_substitute_for_numeric(self):
        metadata = fixture_metadata()
        with self.assertRaises(ValueError): docker.validate_metadata(metadata)
        metadata['schema_version'] = docker.SCHEMA_VERSION
        metadata['workflow_path'] = docker.WORKFLOW_PATH
        metadata['contract_sha256'] = docker.DEFINITION
        for item, name in zip(metadata['assets'], docker.asset_names(metadata['version'])):
            item['name'] = name
        docker.validate_metadata(metadata)
        with self.assertRaises(ValueError): numeric.validate_metadata(metadata)
        metadata['contract_sha256'] = 'b' * 64
        with self.assertRaises(ValueError): docker.validate_metadata(metadata)


if __name__ == '__main__': unittest.main()
