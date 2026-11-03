#!/usr/bin/env python3
"""Update one deployment image without reserializing the surrounding YAML."""

import argparse
from pathlib import Path
import re
import subprocess


def read_image(contents: bytes, deployment: str) -> str:
    # Pass the key as a string, never as executable yq expression text.
    import json

    expression = (
        f'.deployments[{json.dumps(deployment)}]'
        '.spec.template.spec.containers[0].image'
    )
    result = subprocess.run(
        ["yq", "eval", "-e", "-r", expression, "-"],
        input=contents, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True,
    )
    return result.stdout.decode().strip()


def update_image(contents: bytes, deployment: str, repository: str, tag: str) -> bytes:
    if not re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}", tag):
        raise ValueError("Invalid Docker image tag")
    old_image = read_image(contents, deployment)
    if not old_image.startswith(repository + ":") or not re.fullmatch(
        r"[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}", old_image[len(repository) + 1:]
    ):
        raise ValueError(f"Expected a tagged image from {repository}, got {old_image!r}")
    new_image = f"{repository}:{tag}"
    # Match only a whole image scalar. Preserve quoting, comments, newlines,
    # literal config blocks and all other bytes. Ambiguous occurrences fail closed.
    pattern = re.compile(
        rb"(?m)^([ \t]*image:[ \t]*)([\"']?)" + re.escape(old_image.encode())
        + rb"\2(?=[ \t]*(?:#[^\r\n]*)?\r?$)"
    )
    if len(list(pattern.finditer(contents))) != 1:
        raise ValueError("Expected exactly one unambiguous image line")
    updated = pattern.sub(
        lambda match: match[1] + match[2] + new_image.encode() + match[2], contents
    )
    if read_image(updated, deployment) != new_image:
        raise ValueError("Updated YAML does not contain the expected deployment image")
    return updated


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("file", type=Path)
    parser.add_argument("deployment")
    parser.add_argument("repository")
    parser.add_argument("tag")
    args = parser.parse_args()
    original = args.file.read_bytes()
    updated = update_image(original, args.deployment, args.repository, args.tag)
    if updated != original:
        args.file.write_bytes(updated)


if __name__ == "__main__":
    main()
