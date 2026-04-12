# Python Harness — Full Reference

## Settings

```python
class Settings:
    instrument: bool   # Enable branch-arc coverage. Default: True.
    warmup: int        # Pre-heat iterations before main loop. Default: 0.
    transform: bool    # Filter mode: allow output to replace input. Default: False.

# Example
crossfuzz.fuzz(my_target, crossfuzz.Settings(instrument=True, warmup=50))
```

`Settings()` with no arguments uses the defaults above.

## Fuzz

```python
def fuzz(target, settings=None)
```

`target(data: bytes) -> bytes`

Raise an exception to signal a target error. The coordinator records the input
but does not treat it as a crash; the process continues running.

**Example:**

```python
import os, sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../harness/python'))
import crossfuzz

def parse_json(data: bytes) -> bytes:
    import json
    obj = json.loads(data)                 # raises on invalid JSON
    return json.dumps(obj).encode()        # canonical re-serialisation

crossfuzz.fuzz(parse_json)
```

## Filter

```python
def filter(target, settings=None)
```

`target(data: bytes) -> tuple[bytes, bool]` — returns `(output, accepted)`.

When `accepted` is `False`, the coordinator discards the input. When `True`:
- `settings.transform=False` (default): the original input is forwarded unchanged.
- `settings.transform=True`: `output` replaces the original input for all targets.

**Example (validity filter):**

```python
import os, sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../harness/python'))
import crossfuzz

def only_utf8(data: bytes):
    try:
        data.decode('utf-8')
        return b'', True
    except UnicodeDecodeError:
        return b'', False

crossfuzz.filter(only_utf8)
```

**Example (transform filter — normalise JSON):**

```python
import json, os, sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../harness/python'))
import crossfuzz

def normalise(data: bytes):
    try:
        obj = json.loads(data)
        return json.dumps(obj, sort_keys=True).encode(), True
    except Exception:
        return b'', False

crossfuzz.filter(normalise, crossfuzz.Settings(transform=True))
```

## Compare

```python
def compare(target, settings=None)
```

`target(input: bytes, names: list[str], outputs: list[bytes]) -> str`

Return `''` if all outputs agree, or a non-empty mismatch description.

The compare process reads `CROSSFUZZ_SHM_TARGETS` (set by the coordinator)
instead of `CROSSFUZZ_SHM`. It maps each target's SHM read-only.

**Example:**

```python
import os, sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../harness/python'))
import crossfuzz

def check(input_bytes, names, outputs):
    ref = outputs[0]
    for i, out in enumerate(outputs[1:], 1):
        if out != ref:
            return f'{names[0]} vs {names[i]}: {ref!r} != {out!r}'
    return ''

crossfuzz.compare(check)
```

**TOML config for a custom comparator:**

```toml
[[target]]
name = "python_compare"
language = "python"
binary = "python3"
args = ["./python_compare.py"]

[comparator]
type = "custom"
binary = "python3"
args = ["./python_compare.py"]
```

## Disabling instrumentation

Set `instrument=False` when the harness is a thin HTTP client and coverage
comes from an instrumented server (`type = "server"` in the TOML):

```python
crossfuzz.fuzz(
    lambda data: send_http_request(data),
    crossfuzz.Settings(instrument=False),
)
```
