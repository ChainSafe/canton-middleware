"""Regression coverage for dev deploy image bumps (requires mikefarah/yq v4)."""

import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

from update_image_tag import update_image


SERVICES = (
    ("canton-middleware-api", "ghcr.io/chainsafe/canton-erc20-api"),
    ("canton-indexer", "ghcr.io/chainsafe/canton-indexer"),
    ("canton-bridge-relayer", "ghcr.io/chainsafe/canton-middleware"),
)


def fixture(deployment, repository, quote="", newline="\n"):
    return (f'''deployments:
  {deployment}:
    spec:
      template:
        spec:
          containers:
            - name: service
              image: {quote}{repository}:v0.9.1{quote} # keep comment
              imagePullPolicy: IfNotPresent
externalSecrets:
  config:
    template: |
      # Keep this embedded config verbatim.
      url: "https://example.com"

      enabled: true

# Run migrations using the release image before updating application pods.
jobs:
  migrate:
    image: '{{{{ deployment_image }}}}'
''').replace("\n", newline).encode()


class UpdateImageTagTests(unittest.TestCase):
    def test_all_services_preserve_every_other_byte(self):
        for deployment, repository in SERVICES:
            for quote in ("", '"', "'"):
                for newline in ("\n", "\r\n"):
                    with self.subTest(deployment=deployment, quote=quote, newline=newline):
                        original = fixture(deployment, repository, quote, newline)
                        expected = original.replace(
                            f"{repository}:v0.9.1".encode(),
                            f"{repository}:main-13b07ac".encode(),
                        )
                        self.assertEqual(
                            update_image(original, deployment, repository, "main-13b07ac"),
                            expected,
                        )
                        self.assertEqual(len(original.splitlines()), len(expected.splitlines()))

    def test_image_as_first_container_key(self):
        deployment, repository = SERVICES[0]
        original = fixture(deployment, repository).replace(
            b"            - name: service\n              image:",
            b"            - image:",
        )
        expected = original.replace(b":v0.9.1", b":main-13b07ac")
        self.assertEqual(update_image(original, deployment, repository, "main-13b07ac"), expected)

    def test_yq_error_is_visible_and_file_unchanged(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "values.yaml"
            original = b"deployments: [invalid yaml\n"
            path.write_bytes(original)
            result = subprocess.run(
                ["python3", str(Path(__file__).with_name("update_image_tag.py")),
                 str(path), SERVICES[0][0], SERVICES[0][1], "main-13b07ac"],
                capture_output=True,
            )
            self.assertNotEqual(result.returncode, 0)
            diagnostic = subprocess.run(
                ["yq", "eval", ".", "-"], input=original, capture_output=True,
            )
            self.assertNotEqual(diagnostic.returncode, 0)
            self.assertTrue(diagnostic.stderr)
            self.assertIn(diagnostic.stderr, result.stderr)
            self.assertEqual(path.read_bytes(), original)

    def test_already_current_is_noop(self):
        deployment, repository = SERVICES[0]
        original = fixture(deployment, repository)
        self.assertEqual(update_image(original, deployment, repository, "v0.9.1"), original)

    def test_wrong_repository_rejected(self):
        deployment, repository = SERVICES[0]
        with self.assertRaises(ValueError):
            update_image(fixture(deployment, repository), deployment, "ghcr.io/other/api", "main-abc1234")

    def test_missing_deployment_rejected(self):
        deployment, repository = SERVICES[0]
        with self.assertRaises(subprocess.CalledProcessError):
            update_image(fixture(deployment, repository), "missing", repository, "main-abc1234")

    def test_missing_or_ambiguous_image_line_rejected(self):
        deployment, repository = SERVICES[0]
        original = fixture(deployment, repository)
        ambiguous = original + f"other:\n  image: {repository}:v0.9.1\n".encode()
        inline = original.replace(
            f"image: {repository}:v0.9.1 # keep comment".encode(),
            f"image: >-\n                {repository}:v0.9.1".encode(),
        )
        for contents in (ambiguous, inline):
            with self.subTest(contents=contents), self.assertRaises(ValueError):
                update_image(contents, deployment, repository, "main-abc1234")

    def test_invalid_tags_rejected(self):
        deployment, repository = SERVICES[0]
        for tag in ("", "-bad", "a/b", "tag\nother", "a" * 129):
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                update_image(fixture(deployment, repository), deployment, repository, tag)

    def test_untagged_and_digest_images_rejected(self):
        deployment, repository = SERVICES[0]
        original = fixture(deployment, repository)
        for image in (repository, repository + "@sha256:" + "a" * 64):
            with self.subTest(image=image), self.assertRaises(ValueError):
                update_image(original.replace(f"{repository}:v0.9.1".encode(), image.encode()),
                             deployment, repository, "main-abc1234")

    def test_cli_failure_does_not_write_file(self):
        deployment, repository = SERVICES[0]
        original = fixture(deployment, repository)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "values.yaml"
            path.write_bytes(original)
            result = subprocess.run(
                ["python3", str(Path(__file__).with_name("update_image_tag.py")),
                 str(path), deployment, repository, "invalid/tag"],
                capture_output=True,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(path.read_bytes(), original)


class WorkflowRerunTests(unittest.TestCase):
    """Execute the actual workflow update step against an isolated infra git repo."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        source = Path(__file__).resolve().parents[2]
        (self.root / ".src").symlink_to(source, target_is_directory=True)
        workflow = source / ".github/workflows/docker-build-release.yml"
        steps = json.loads(subprocess.check_output([
            "yq", "eval", "-o=json", ".jobs.open-devnet-pr.steps", str(workflow),
        ]))
        self.update_step = next(step for step in steps if step.get("id") == "images")
        self.pr_step = next(step for step in steps if step["name"] == "Create signed commit and open PR")
        self.env = dict(os.environ, DIR="clusters/dev/apps/canton", VERSION="main-13b07ac",
                        GITHUB_OUTPUT=str(self.root / "outputs"))
        self.git("init", "-q")
        self.git("config", "user.name", "Updater tests")
        self.git("config", "user.email", "updater-tests@example.com")
        self.git("config", "commit.gpgsign", "false")

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.root, capture_output=True, check=True)

    def prepare(self, current_services=0):
        for index, (deployment, repository) in enumerate(SERVICES):
            folder = "canton-middleware" if deployment == "canton-bridge-relayer" else deployment
            path = self.root / self.env["DIR"] / folder / "values.yaml"
            path.parent.mkdir(parents=True)
            contents = fixture(deployment, repository)
            if index < current_services:
                contents = contents.replace(b":v0.9.1", b":main-13b07ac")
            path.write_bytes(contents)
        self.git("add", "clusters")
        self.git("commit", "-qm", "baseline")

    def run_update(self):
        output = Path(self.env["GITHUB_OUTPUT"])
        output.write_text("")
        result = subprocess.run(["bash", "-e", "-c", self.update_step["run"]],
                                cwd=self.root, env=self.env, capture_output=True, text=True)
        return result, output.read_text()

    def test_fresh_deploy_then_rerun_after_merge(self):
        self.prepare()
        result, output = self.run_update()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(output, "changed=true\n")
        self.assertEqual(len(self.git("diff", "--numstat").stdout.splitlines()), 3)
        # Simulate main incorporating the merged deploy PR; no new PR is needed.
        self.git("add", "clusters")
        self.git("commit", "-qm", "deploy merged")
        result, output = self.run_update()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(output, "changed=false\n")
        self.assertIn("devnet already at main-13b07ac", result.stdout)
        self.assertEqual(self.git("diff").stdout, b"")
        self.assertEqual(self.pr_step["if"], "steps.images.outputs.changed == 'true'")

    def test_rerun_with_open_pr_still_reports_changes(self):
        self.prepare()
        for _ in range(2):
            result, output = self.run_update()
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(output, "changed=true\n")

    def test_partial_service_update_still_opens_pr(self):
        self.prepare(current_services=1)
        result, output = self.run_update()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(output, "changed=true\n")
        self.assertEqual(len(self.git("diff", "--numstat").stdout.splitlines()), 2)

    def test_unrelated_diff_still_fails_closed(self):
        self.prepare()
        path = self.root / self.env["DIR"] / "canton-middleware-api/values.yaml"
        path.write_bytes(path.read_bytes().replace(b"enabled: true", b"enabled: false"))
        result, output = self.run_update()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("expected exactly one line changed", result.stdout)
        self.assertEqual(output, "")


if __name__ == "__main__":
    unittest.main()
