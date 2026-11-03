"""Regression coverage for dev deploy image bumps (requires mikefarah/yq v4)."""

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


if __name__ == "__main__":
    unittest.main()
