#!/usr/bin/env python3

from __future__ import annotations

import copy
import importlib.util
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("compatibility.py")
SPEC = importlib.util.spec_from_file_location("fisco_compatibility", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
compatibility = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(compatibility)


class CompatibilityBaselineTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.baseline = compatibility.load_baseline(compatibility.DEFAULT_BASELINE)

    def test_baseline_is_valid(self) -> None:
        compatibility.validate_baseline(self.baseline)

    def test_air_artifacts_are_admissible_but_runtime_is_not(self) -> None:
        row = compatibility.check_profile(
            self.baseline, "air", "standard", "linux/amd64", "artifact", "native"
        )
        self.assertEqual(row["artifact_status"], "verified")
        with self.assertRaisesRegex(compatibility.BaselineError, "runtime admission denied"):
            compatibility.check_profile(
                self.baseline, "air", "standard", "linux/amd64", "runtime", "native"
            )

    def test_pro_and_max_fail_closed(self) -> None:
        for deployment in ("pro", "max"):
            with self.assertRaisesRegex(compatibility.BaselineError, "artifact admission denied"):
                compatibility.check_profile(
                    self.baseline, deployment, "guomi", "linux/arm64", "artifact", "native"
                )

    def test_container_without_digest_fails_closed(self) -> None:
        with self.assertRaisesRegex(compatibility.BaselineError, "container admission denied"):
            compatibility.check_profile(
                self.baseline, "air", "standard", "linux/amd64", "documented", "container"
            )

    def test_unavailable_artifact_cannot_be_runtime_candidate(self) -> None:
        invalid = copy.deepcopy(self.baseline)
        row = next(item for item in invalid["matrix"] if item["deployment"] == "pro")
        row["runtime_status"] = "partial"
        with self.assertRaisesRegex(compatibility.BaselineError, "must be unsupported"):
            compatibility.validate_baseline(invalid)


if __name__ == "__main__":
    unittest.main()
