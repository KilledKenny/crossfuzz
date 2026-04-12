"""cross_fuzz Python harness.

Handles IPC between the cross_fuzz coordinator and a Python target. You write
the target function; the harness manages shared memory, pipes, and coverage.

Usage::

    import sys, os
    sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../harness/python'))
    import crossfuzz

    def my_target(data: bytes) -> bytes:
        return data  # process input, return output

    crossfuzz.fuzz(my_target)

Coverage is collected via the `coverage` library (``pip install coverage``).
Branch arcs are hashed into the 64 KB shared-memory bitmap using the same
multiplicative hash scheme as the JS harness.
"""

import json
import mmap as _mmap
import os
import struct
import sys

# ── Shared memory layout (must match pkg/coverage/shmem.go) ──────────────────
_OFF_INPUT_LEN  = 8
_OFF_OUTPUT_LEN = 12
_OFF_STATUS     = 16
_INPUT_OFFSET   = 0x000040
_INPUT_SIZE     = 1 << 20       # 1 MB
_OUTPUT_OFFSET  = 0x100040
_OUTPUT_SIZE    = 1 << 20       # 1 MB
_COV_OFFSET     = 0x200040
_COV_SIZE       = 1 << 16       # 64 KB
_TOTAL_SIZE     = 0x210040

_STATUS_OK    = 0
_STATUS_ERROR = 1

_CMD_FD  = 3   # coordinator → harness (read)
_RESP_FD = 4   # harness → coordinator (write)

# ── Protocol ──────────────────────────────────────────────────────────────────

def _read_exact(fd, n):
    buf = bytearray(n)
    mv  = memoryview(buf)
    pos = 0
    while pos < n:
        chunk = os.read(fd, n - pos)
        if not chunk:
            return None
        mv[pos:pos + len(chunk)] = chunk
        pos += len(chunk)
    return bytes(buf)


def _read_msg(fd):
    hdr = _read_exact(fd, 4)
    if hdr is None:
        return None
    length = struct.unpack('>I', hdr)[0]
    if length == 0 or length > (1 << 20):
        return None
    return _read_exact(fd, length)


def _write_msg(fd, text):
    if isinstance(text, str):
        text = text.encode()
    os.write(fd, struct.pack('>I', len(text)) + text)


def _escape_json(s):
    return s.replace('\\', '\\\\').replace('"', '\\"').replace('\n', '\\n').replace('\r', '\\r')


# ── Shared memory ─────────────────────────────────────────────────────────────

class _SHM:
    """Read-write mmap of the target's shared memory region."""
    __slots__ = ('_mm',)

    def __init__(self, path):
        fd = os.open(path, os.O_RDWR)
        self._mm = _mmap.mmap(fd, _TOTAL_SIZE)
        os.close(fd)

    def read_input(self):
        n = min(struct.unpack_from('<I', self._mm, _OFF_INPUT_LEN)[0], _INPUT_SIZE)
        self._mm.seek(_INPUT_OFFSET)
        return self._mm.read(n)

    def write_output(self, data):
        n = min(len(data), _OUTPUT_SIZE)
        struct.pack_into('<I', self._mm, _OFF_OUTPUT_LEN, n)
        self._mm.seek(_OUTPUT_OFFSET)
        self._mm.write(data[:n])

    def set_output_len(self, n):
        struct.pack_into('<I', self._mm, _OFF_OUTPUT_LEN, n)

    def set_status(self, s):
        struct.pack_into('<I', self._mm, _OFF_STATUS, s)

    def cov_view(self):
        """Writable memoryview of the 64 KB coverage bitmap."""
        return memoryview(self._mm)[_COV_OFFSET:_COV_OFFSET + _COV_SIZE]


class _ROShm:
    """Read-only mmap of a target's shared memory (used by the compare harness)."""
    __slots__ = ('_mm',)

    def __init__(self, path):
        fd = os.open(path, os.O_RDONLY)
        self._mm = _mmap.mmap(fd, _TOTAL_SIZE, access=_mmap.ACCESS_READ)
        os.close(fd)

    def read_input(self):
        n = min(struct.unpack_from('<I', self._mm, _OFF_INPUT_LEN)[0], _INPUT_SIZE)
        self._mm.seek(_INPUT_OFFSET)
        return self._mm.read(n)

    def read_output(self):
        n = min(struct.unpack_from('<I', self._mm, _OFF_OUTPUT_LEN)[0], _OUTPUT_SIZE)
        self._mm.seek(_OUTPUT_OFFSET)
        return self._mm.read(n)


# ── Coverage ──────────────────────────────────────────────────────────────────

def _hash_slot(file_idx, from_line, to_line):
    """Map a coverage arc to a 16-bit bitmap index."""
    h = (file_idx * 0x9E3779B9) & 0xFFFFFFFF
    h = ((h ^ (from_line & 0xFFFFFFFF)) * 0xBF58476D) & 0xFFFFFFFF
    h = ((h ^ (to_line   & 0xFFFFFFFF)) * 0x94D049BB) & 0xFFFFFFFF
    return (h ^ (h >> 16)) & 0xFFFF


def _collect_into(cov_data, file_index, bv):
    """Accumulate coverage arc counts from cov_data into bytearray bv.

    file_index is a dict mapping absolute filename → stable integer index that
    grows monotonically across iterations so bitmap slots stay consistent.
    """
    for filename in cov_data.measured_files():
        fi = file_index.setdefault(filename, len(file_index))
        arcs = cov_data.arcs(filename)
        if not arcs:
            continue
        for from_line, to_line in arcs:
            slot = _hash_slot(fi, from_line, to_line)
            v = bv[slot]
            if v < 255:
                bv[slot] = v + 1


# ── Settings ──────────────────────────────────────────────────────────────────

class Settings:
    """Harness configuration. All fields have safe defaults."""

    def __init__(self, *, instrument=True, warmup=0, transform=False):
        # Enable branch-arc coverage collection via the `coverage` library.
        # Default: True. Set False when the harness is a thin HTTP client and
        # coverage comes from an instrumented server process.
        self.instrument = instrument

        # Run the target this many extra times on the first input before
        # entering the main loop (pre-heating the interpreter). Default: 0.
        self.warmup = warmup

        # Filter mode only: when True, the bytes returned by the filter replace
        # the original input for downstream targets. Default: False.
        self.transform = transform


# ── Entry points ──────────────────────────────────────────────────────────────

def fuzz(target, settings=None):
    """Enter the persistent fuzzing loop.

    ``target(data: bytes) -> bytes`` — raise an exception to signal an error.
    """
    if settings is None:
        settings = Settings()

    shm_path = os.environ.get('CROSSFUZZ_SHM')
    if not shm_path:
        sys.stderr.write('crossfuzz: CROSSFUZZ_SHM not set\n')
        sys.exit(1)
    shm = _SHM(shm_path)

    cov        = None
    file_index = {}   # filename → stable integer index
    warmed_up  = False

    if settings.instrument:
        try:
            import coverage as _cov_lib
        except ImportError:
            sys.stderr.write(
                'crossfuzz: coverage library not installed (pip install coverage)\n'
            )
            sys.exit(1)
        cov = _cov_lib.Coverage(
            branch=True,
            data_file=None,
            omit=[os.path.abspath(__file__)],
        )

    _write_msg(_RESP_FD, '{"type":"ready"}')

    while True:
        raw = _read_msg(_CMD_FD)
        if raw is None:
            return

        try:
            msg = json.loads(raw)
        except Exception:
            continue

        t = msg.get('type')

        if t == 'shutdown':
            return

        elif t == 'fuzz':
            inp = shm.read_input()

            # Warmup: pre-heat the interpreter on the first real input.
            if settings.warmup > 0 and not warmed_up:
                for _ in range(settings.warmup):
                    try:
                        target(inp)
                    except Exception:
                        pass
                warmed_up = True

            if cov is not None:
                cov.start()

            try:
                out = target(inp)
                if isinstance(out, str):
                    out = out.encode()
                shm.write_output(bytes(out))
                shm.set_status(_STATUS_OK)
                resp = '{"type":"fuzz_result","ok":true}'
            except Exception as e:
                shm.set_output_len(0)
                shm.set_status(_STATUS_ERROR)
                resp = f'{{"type":"fuzz_result","error":"{_escape_json(str(e))}"}}'

            if cov is not None:
                cov.stop()
                bv = bytearray(_COV_SIZE)
                _collect_into(cov.get_data(), file_index, bv)
                shm.cov_view()[:] = bv
                cov.erase()

            _write_msg(_RESP_FD, resp)

        elif t == 'ping':
            _write_msg(_RESP_FD, '{"type":"pong"}')


def filter(target, settings=None):
    """Enter the persistent filter loop.

    ``target(data: bytes) -> tuple[bytes, bool]`` — returns ``(output, accepted)``.
    """
    if settings is None:
        settings = Settings()

    shm_path = os.environ.get('CROSSFUZZ_SHM')
    if not shm_path:
        sys.stderr.write('crossfuzz: CROSSFUZZ_SHM not set\n')
        sys.exit(1)
    shm = _SHM(shm_path)

    _write_msg(_RESP_FD, '{"type":"ready"}')

    while True:
        raw = _read_msg(_CMD_FD)
        if raw is None:
            return

        try:
            msg = json.loads(raw)
        except Exception:
            continue

        t = msg.get('type')

        if t == 'shutdown':
            return

        elif t == 'filter':
            inp = shm.read_input()
            try:
                out, accepted = target(inp)
            except Exception:
                out, accepted = b'', False

            if accepted:
                if settings.transform and out:
                    shm.write_output(bytes(out))
                else:
                    shm.write_output(inp)
                _write_msg(_RESP_FD, '{"type":"filter_result","ok":true}')
            else:
                shm.set_output_len(0)
                _write_msg(_RESP_FD, '{"type":"filter_result"}')

        elif t == 'ping':
            _write_msg(_RESP_FD, '{"type":"pong"}')


def compare(target, settings=None):
    """Enter the persistent compare loop.

    ``target(input: bytes, names: list[str], outputs: list[bytes]) -> str``
    — returns ``''`` if all outputs agree, or a non-empty mismatch description.

    The compare process reads ``CROSSFUZZ_SHM_TARGETS`` (a JSON map of
    ``{"name": "/path/to/shm", ...}``) set by the coordinator.
    """
    targets_json = os.environ.get('CROSSFUZZ_SHM_TARGETS')
    if not targets_json:
        sys.stderr.write('crossfuzz: CROSSFUZZ_SHM_TARGETS not set\n')
        sys.exit(1)

    try:
        target_paths = json.loads(targets_json)
    except Exception as e:
        sys.stderr.write(f'crossfuzz: parse CROSSFUZZ_SHM_TARGETS: {e}\n')
        sys.exit(1)

    target_shms = {}
    for name, path in target_paths.items():
        try:
            target_shms[name] = _ROShm(path)
        except OSError as e:
            sys.stderr.write(f'crossfuzz: open target SHM {name} ({path}): {e}\n')
            sys.exit(1)

    _write_msg(_RESP_FD, '{"type":"ready"}')

    while True:
        raw = _read_msg(_CMD_FD)
        if raw is None:
            return

        try:
            msg = json.loads(raw)
        except Exception:
            continue

        t = msg.get('type')

        if t == 'shutdown':
            return

        elif t == 'compare':
            names   = msg.get('targets', [])
            inp     = None
            outputs = []

            for name in names:
                ts = target_shms.get(name)
                if ts is None:
                    outputs.append(b'')
                    continue
                if inp is None:
                    inp = ts.read_input()
                outputs.append(ts.read_output())

            if inp is None:
                inp = b''

            try:
                mismatch = target(inp, names, outputs)
            except Exception as e:
                mismatch = f'compare exception: {e}'

            if mismatch:
                _write_msg(
                    _RESP_FD,
                    f'{{"type":"compare_result","error":"{_escape_json(str(mismatch))}"}}',
                )
            else:
                _write_msg(_RESP_FD, '{"type":"compare_result"}')

        elif t == 'ping':
            _write_msg(_RESP_FD, '{"type":"pong"}')
