# Python Harness

## Harness file

`harness/python/crossfuzz.py` — published to PyPI as `crossfuzz`. Install with `pip install crossfuzz`, then `import crossfuzz`.

## Dependency

A venv with the `coverage` library lives at `harness/python/.venv/`. Use its interpreter directly — no activation needed:

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
import crossfuzz

def my_target(data: bytes) -> bytes:
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

## Settings

```python
class Settings:
    instrument: bool   # Enable branch-arc coverage. Default: True.
    transform: bool    # Filter mode: allow output to replace input. Default: False.

crossfuzz.fuzz(my_target, crossfuzz.Settings(instrument=False))
```

`Settings()` with no arguments uses the defaults.

## Filter

```python
def filter(target, settings=None)
```

`target(data: bytes) -> tuple[bytes, bool]` — returns `(output, accepted)`.

When `accepted` is `False` the coordinator discards the input. When `True`:
- `settings.transform=False` (default): original input is forwarded unchanged.
- `settings.transform=True`: `output` replaces the original input for all targets.

**Validity filter:**

```python
import crossfuzz

def only_utf8(data: bytes):
    try:
        data.decode('utf-8')
        return b'', True
    except UnicodeDecodeError:
        return b'', False

crossfuzz.filter(only_utf8)
```

**Transform filter (normalise JSON):**

```python
import json
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

```python
import crossfuzz

def check(input_bytes, names, outputs):
    ref = outputs[0]
    for i, out in enumerate(outputs[1:], 1):
        if out != ref:
            return f'{names[0]} vs {names[i]}: {ref!r} != {out!r}'
    return ''

crossfuzz.compare(check)
```

Configure as `[comparator] type = "harness"`.

## Disabling instrumentation

Set `instrument=False` when the harness is a thin HTTP client and coverage comes from an instrumented server:

```python
crossfuzz.fuzz(
    lambda data: send_http_request(data),
    crossfuzz.Settings(instrument=False),
)
```

## How coverage works

The harness uses `coverage.Coverage(branch=True)` to collect branch arc data via Python's `sys.settrace`. After the target returns, arcs `(from_line, to_line)` are hashed to 16-bit bitmap indices using the same multiplicative hash as the JS harness, then written to the shared bitmap.
