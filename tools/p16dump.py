#!/usr/bin/env python3
"""Décode les trames Photon TCP (Protocol16) d'une capture : p16dump.py <pcap> <tcp.stream> [max]

S'appuie sur tshark pour réassembler le flux. Les messages chiffrés (0x80) sont signalés, pas décodés.
"""
import struct, subprocess, sys

MSG = {0: "Init", 1: "InitResponse", 2: "OpRequest", 3: "OpResponse", 4: "Event",
       6: "InternalOpRequest", 7: "InternalOpResponse", 8: "Message", 9: "RawMessage"}


class R:
    def __init__(s, b): s.b, s.i = b, 0
    def take(s, n):
        v = s.b[s.i:s.i + n]; s.i += n
        if len(v) < n: raise EOFError
        return v
    def u8(s): return s.take(1)[0]
    def i16(s): return struct.unpack(">h", s.take(2))[0]
    def i32(s): return struct.unpack(">i", s.take(4))[0]


def val(r, t=None):
    if t is None: t = r.u8()
    c = chr(t)
    if t == 0 or c == '*': return None
    if c == 'b': return r.u8()
    if c == 'o': return bool(r.u8())
    if c == 'k': return r.i16()
    if c == 'i': return r.i32()
    if c == 'l': return struct.unpack(">q", r.take(8))[0]
    if c == 'f': return struct.unpack(">f", r.take(4))[0]
    if c == 'd': return struct.unpack(">d", r.take(8))[0]
    if c == 's':
        n = struct.unpack(">H", r.take(2))[0]; return r.take(n).decode("utf-8", "replace")
    if c == 'x':
        n = r.i32(); b = r.take(n); return f"bytes[{n}]:{b[:24].hex()}{'…' if n > 24 else ''}"
    if c == 'n':
        n = r.i32(); return [r.i32() for _ in range(n)]
    if c == 'a':
        n = struct.unpack(">H", r.take(2))[0]; return [val(r, ord('s')) for _ in range(n)]
    if c == 'y':
        n = struct.unpack(">H", r.take(2))[0]; et = r.u8(); return [val(r, et) for _ in range(n)]
    if c == 'z':
        n = struct.unpack(">H", r.take(2))[0]; return [val(r) for _ in range(n)]
    if c == 'h':
        n = struct.unpack(">H", r.take(2))[0]; return {repr(val(r)): val(r) for _ in range(n)}
    if c == 'D':
        kt, vt = r.u8(), r.u8(); n = struct.unpack(">H", r.take(2))[0]
        return {repr(val(r, kt if kt else None)): val(r, vt if vt else None) for _ in range(n)}
    if c == 'c':
        ct = r.u8(); n = struct.unpack(">H", r.take(2))[0]; return f"custom{ct}[{n}]:{r.take(n)[:16].hex()}"
    raise ValueError(f"type inconnu {t:#x} ({c!r}) à {r.i}")


def params(r):
    n = struct.unpack(">H", r.take(2))[0]
    return {r.u8(): val(r) for _ in range(n)}


def decode(p):
    r = R(p)
    if r.u8() != 0xF3: return f"non-F3 {p[:16].hex()}"
    t = r.u8(); enc = t & 0x80; t &= 0x7F
    name = MSG.get(t, f"type{t}")
    if enc: return f"{name} CHIFFRÉ ({len(p) - 2} o)"
    if t == 0: return f"Init {p[2:].hex()}"
    if t == 1: return "InitResponse"
    if t in (2, 6):
        op = r.u8(); return f"{name} op={op} {params(r)}"
    if t in (3, 7):
        op = r.u8(); rc = r.i16(); dbg = val(r); return f"{name} op={op} rc={rc} dbg={dbg!r} {params(r)}"
    if t == 4:
        code = r.u8(); return f"Event code={code} {params(r)}"
    return f"{name} {p[2:40].hex()}"


def frames(data):
    i = 0
    while i < len(data):
        if data[i] == 0xF0:  # ping
            i += 9 if False else 5; continue
        if data[i] != 0xFB: yield None, data[i:i + 16]; return
        ln = struct.unpack(">I", data[i + 1:i + 5])[0]
        yield data[i + 5:i + 7], data[i + 7:i + ln]; i += ln


def stream(pcap, sid):
    out = subprocess.run(["tshark", "-r", pcap, "-Y", f"tcp.stream=={sid} && tcp.len>0", "-T", "fields",
                          "-e", "frame.time_relative", "-e", "tcp.srcport", "-e", "tcp.payload"],
                         capture_output=True, text=True).stdout
    bufs = {}
    for line in out.splitlines():
        t, sport, hx = line.split("\t")
        d = "S" if int(sport) in (4530, 4531, 4533, 5055, 5056, 5058) else "C"
        b = bufs.get(d, b"") + bytes.fromhex(hx)
        # serveur : les pings font 9 octets (F0 + 2 horodatages)
        res, i = [], 0
        while i < len(b):
            if b[i] == 0xF0:
                n = 5 if d == "C" else 9
                if len(b) - i < n: break
                i += n; continue
            if b[i] != 0xFB or len(b) - i < 5: break
            ln = struct.unpack(">I", b[i + 1:i + 5])[0]
            if len(b) - i < ln: break
            res.append(b[i + 7:i + ln]); i += ln
        bufs[d] = b[i:]
        for p in res:
            yield float(t), d, p


if __name__ == "__main__":
    pcap, sid = sys.argv[1], sys.argv[2]
    mx = int(sys.argv[3]) if len(sys.argv) > 3 else 10**9
    for k, (t, d, p) in enumerate(stream(pcap, sid)):
        if k >= mx: break
        try: s = decode(p)
        except Exception as e: s = f"ERREUR {e} {p[:32].hex()}"
        print(f"{t:9.3f} {d}> {s[:600]}")
