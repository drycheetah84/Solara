# pak11.py -- minimal UE5 pak v11 reader/writer (unencrypted index, Oodle).
import struct, hashlib, ctypes

MAGIC = 0x5A6F12E1
OODLE = __import__("os").environ.get("OODLE_DLL", r"C:SERSDRYCHA7_RECOVERYTOOLSOODLEOODLE-DATA-SHARED.DLL")


class R:
    def __init__(s, b, p=0): s.b, s.p = b, p
    def u8(s): v = s.b[s.p]; s.p += 1; return v
    def u32(s): v, = struct.unpack_from('<I', s.b, s.p); s.p += 4; return v
    def i32(s): v, = struct.unpack_from('<i', s.b, s.p); s.p += 4; return v
    def u64(s): v, = struct.unpack_from('<Q', s.b, s.p); s.p += 8; return v
    def i64(s): v, = struct.unpack_from('<q', s.b, s.p); s.p += 8; return v
    def raw(s, n): v = s.b[s.p:s.p + n]; s.p += n; return v
    def fstr(s):
        n = s.i32()
        if n == 0: return ''
        if n < 0:
            v = s.raw(-n * 2).decode('utf-16-le'); return v[:-1]
        return s.raw(n)[:-1].decode('latin-1')


def wfstr(v):
    if v == '': return struct.pack('<i', 0)
    try:
        b = v.encode('ascii') + b'\0'; return struct.pack('<i', len(b)) + b
    except UnicodeEncodeError:
        b = (v + '\0').encode('utf-16-le'); return struct.pack('<i', -(len(b) // 2)) + b


def hdr_size(method, nblocks):
    return 8 + 8 + 8 + 4 + 20 + ((4 + 16 * nblocks) if method else 0) + 1 + 4


def decode_entry(b, p):
    r = R(b, p)
    v = r.u32()
    bs = r.u32() if (v & 0x3f) == 0x3f else (v & 0x3f) << 11
    method = (v >> 23) & 0x3f
    enc = (v >> 22) & 1
    nb = (v >> 6) & 0xffff
    off = r.u32() if v & (1 << 31) else r.u64()
    usz = r.u32() if v & (1 << 30) else r.u64()
    sz = (r.u32() if v & (1 << 29) else r.u64()) if method else usz
    blocks = []
    if nb:
        start = hdr_size(method, nb)  # relative chunk offsets (v>=5)
        if nb == 1 and not enc:
            blocks.append((start, start + sz))
        else:
            for _ in range(nb):
                n = r.u32(); blocks.append((start, start + n)); start += (n + 15) & ~15 if enc else n
    return dict(offset=off, size=sz, usize=usz, method=method, enc=enc, bsize=bs, blocks=blocks), r.p - p


def encode_uncompressed(offset, size):
    assert offset < 2**32 and size < 2**32
    v = (1 << 31) | (1 << 30) | (1 << 29)
    return struct.pack('<III', v, offset, size)


def inline_header_uncompressed(size, sha1):
    # FPakEntry::Serialize: Offset(0 in-data), Size, UncompressedSize, CompressionMethodIndex, Hash, Flags, CompressionBlockSize
    return struct.pack('<qqqI', 0, size, size, 0) + sha1 + struct.pack('<BI', 0, 0)


class Pak:
    def __init__(s, path):
        s.path = path
        s.d = open(path, 'rb').read()
        d = s.d
        i = d.rfind(struct.pack('<I', MAGIC))
        s.foot_start = i - 17
        s.guid = d[i - 17:i - 1]; assert d[i - 1] == 0, 'encrypted index'
        s.ver, = struct.unpack_from('<I', d, i + 4); assert s.ver == 11
        s.idxoff, s.idxsize = struct.unpack_from('<QQ', d, i + 8)
        s.idxhash = d[i + 24:i + 44]
        s.methods = d[i + 44:i + 44 + 5 * 32]
        s.foot_tail = d[i + 44 + 5 * 32:]
        idx = d[s.idxoff:s.idxoff + s.idxsize]
        assert hashlib.sha1(idx).digest() == s.idxhash, 'primary index hash mismatch'
        r = R(idx)
        s.mount = r.fstr(); s.num = r.i32(); s.seed = r.u64()
        s.has_phi = r.i32()
        if s.has_phi: s.phi_off, s.phi_size = r.i64(), r.i64(); s.phi_hash = r.raw(20)
        s.has_fdi = r.i32()
        if s.has_fdi: s.fdi_off, s.fdi_size = r.i64(), r.i64(); s.fdi_hash = r.raw(20)
        n = r.i32(); s.enc_entries = bytearray(r.raw(n))
        s.nfiles = r.i32(); s.files_raw = idx[r.p:]  # non-encodable FPakEntry list (kept verbatim)
        s.phi = d[s.phi_off:s.phi_off + s.phi_size] if s.has_phi else b''
        s.fdi = d[s.fdi_off:s.fdi_off + s.fdi_size] if s.has_fdi else b''
        if s.has_phi: assert hashlib.sha1(s.phi).digest() == s.phi_hash
        if s.has_fdi: assert hashlib.sha1(s.fdi).digest() == s.fdi_hash
        s.dirs = s.parse_fdi()

    def parse_fdi(s):
        r = R(s.fdi); out = []
        for _ in range(r.i32()):
            dn = r.fstr(); files = []
            for _ in range(r.i32()):
                fn = r.fstr(); files.append([fn, r.i32()])
            out.append([dn, files])
        assert r.p == len(s.fdi)
        return out

    def build_fdi(s):
        b = bytearray(struct.pack('<i', len(s.dirs)))
        for dn, files in s.dirs:
            b += wfstr(dn) + struct.pack('<i', len(files))
            for fn, off in files: b += wfstr(fn) + struct.pack('<i', off)
        return bytes(b)

    def files(s):
        for dn, fl in s.dirs:
            for fn, off in fl: yield dn + fn, off

    def entry(s, loc):
        assert loc >= 0, 'non-encodable entry'
        return decode_entry(s.enc_entries, loc)[0]

    def read(s, loc):
        e = s.entry(loc); assert not e['enc']
        base = e['offset']
        if e['method'] == 0:
            h = hdr_size(0, 0); return s.d[base + h:base + h + e['usize']]
        name = s.methods[(e['method'] - 1) * 32:e['method'] * 32].split(b'\0')[0]
        assert name == b'Oodle', name
        dll = ctypes.WinDLL(OODLE)
        dll.OodleLZ_Decompress.restype = ctypes.c_int64
        out = bytearray(); left = e['usize']
        for a, z in e['blocks']:
            # bsize 0 = a single block holding the whole file (encoded-entry shorthand), not a 0-byte block
            src = s.d[base + a:base + z]; n = min(e['bsize'] or e['usize'], left)
            dst = ctypes.create_string_buffer(n)
            got = dll.OodleLZ_Decompress(src, ctypes.c_int64(len(src)), dst, ctypes.c_int64(n),
                                         1, 0, 0, None, ctypes.c_int64(0), None, None, None, ctypes.c_int64(0), 3)
            assert got == n, (got, n)
            out += dst.raw; left -= n
        return bytes(out)
