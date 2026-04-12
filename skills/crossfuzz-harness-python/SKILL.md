---
name: crossfuzz-harness-python
description: Use this skill when the user is writing a Python target for cross_fuzz, needs to know the Python harness API, wants to understand how Python coverage is collected, or is setting up a Python fuzzing target. Trigger for questions like "how do I write a Python target?", "how does Python coverage work in cross_fuzz?", "how do I add Python to cross_fuzz?", or "my Python target isn't producing coverage".
---

# Python Harness

## Harness file

`harness/python/crossfuzz.py` — add the harness directory to `sys.path` in your target script, then import it.

## Dependency

A venv with the `coverage` library lives at `harness/python/.venv/`. Use its
interpreter directly — no activation needed:

```bash
harness/python/.venv/bin/python3 my_target.py
```

To recreate the venv from scratch:

```bash
python3 -m venv harness/python/.venv
harness/python/.venv/bin/pip install -r harness/python/requirements.txt
```

## Fuzz target

```python
import os, sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../harness/python'))
import crossfuzz

def my_target(data: bytes) -> bytes:
    # Process input bytes, return output bytes.
    # Raise an exception to signal an error — does not crash the process.
    return data

crossfuzz.fuzz(my_target)
```

### Target function signature

```python
(data: bytes) -> bytes
```

## TOML config entry

```toml
[[target]]
name = "python_impl"
language = "python"
binary = "../../harness/python/.venv/bin/python3"
args = ["./python_target.py"]
```

## How coverage works

The harness uses `coverage.Coverage(branch=True)` to collect branch arc data
around each target invocation via Python's `sys.settrace`. After the target
returns:

1. `cov.stop()` is called to stop tracing.
2. `cov.get_data().arcs(filename)` yields `(from_line, to_line)` pairs for
   every branch arc executed.
3. Each arc is hashed to a 16-bit bitmap index using the same multiplicative
   hash as the JS harness: `hash(file_idx, from_line, to_line) & 0xFFFF`.
4. The corresponding bitmap byte is incremented (saturating at 255).
5. `cov.erase()` resets state for the next iteration.

The coordinator resets the bitmap before every fuzz command, so the harness
always writes a fresh snapshot.

For Settings, filter, and compare variants with annotated examples, read
`<skill-dir>/references/python-harness.md`.
