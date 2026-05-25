#!/usr/bin/env python3
"""Case-insensitive equality custom comparator.

The crossfuzz custom comparator protocol passes a JSON object on stdin:

    {"input": "<base64>", "outputs": {"<target>": "<base64>", ...}}

Go marshals []byte as base64 strings. Exit 0 = no discrepancy; any non-zero
exit with optional stdout text = finding.
"""

import base64
import json
import sys


def main() -> int:
    payload = json.load(sys.stdin)
    outputs = payload.get("outputs", {})
    normalized = []
    for name, b64 in outputs.items():
        b = base64.b64decode(b64)
        normalized.append((name, b.decode("utf-8", errors="replace").lower()))
    if not normalized:
        return 0
    ref = normalized[0][1]
    for name, n in normalized[1:]:
        if n != ref:
            print(f"case-insensitive mismatch: {normalized[0][0]} vs {name}")
            return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
