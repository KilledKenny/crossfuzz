import os
import sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../../harness/python'))
import crossfuzz


def encode_base64(data: bytes) -> bytes:
    alphabet = b'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
    out = bytearray()
    for i in range(0, len(data), 3):
        chunk = data[i:i + 3]
        a = chunk[0]
        b = chunk[1] if len(chunk) > 1 else 0
        c = chunk[2] if len(chunk) > 2 else 0
        triple = (a << 16) | (b << 8) | c
        out.append(alphabet[(triple >> 18) & 0x3F])
        out.append(alphabet[(triple >> 12) & 0x3F])
        out.append(alphabet[(triple >> 6)  & 0x3F])
        out.append(alphabet[ triple        & 0x3F])
    rem = len(data) % 3
    if rem == 1:
        out[-2] = ord('=')
        out[-1] = ord('=')
    elif rem == 2:
        out[-1] = ord('=')
    return bytes(out)


crossfuzz.fuzz(encode_base64)
