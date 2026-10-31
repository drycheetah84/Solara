# obbclone.py <orig.obb> <out.obb> <entry name> <replacement>
# Rewrites the OBB copying every original local header / central record byte-for-byte;
# only the replaced entry's CRC/sizes and the shifted header offsets change. Stored entries only, no zip64.
import sys, struct, zlib, os

src, dst, name, repl = sys.argv[1:5]
f = open(src, 'rb'); total = os.path.getsize(src)
f.seek(max(0, total - 70000)); tail = f.read(); e = tail.rfind(b'PK\x05\x06'); assert e >= 0
eocd = bytearray(tail[e:e + 22]); cmt = tail[e + 22:]
_, disk, cdisk, n1, n, cdsize, cdoff, clen = struct.unpack('<IHHHHIIH', eocd)
assert n1 == n and tail.rfind(b'PK\x06\x06') < 0 and tail.rfind(b'PK\x06\x07') < 0, 'zip64 not handled'
f.seek(cdoff); cd = f.read(cdsize)

new = open(repl, 'rb').read(); newcrc = zlib.crc32(new) & 0xffffffff
out = open(dst, 'wb'); recs = []; p = 0; hit = 0
f.seek(0); prefix_end = None
for _ in range(n):
    rec = bytearray(cd[p:p + 46]); assert rec[:4] == b'PK\x01\x02'
    meth, = struct.unpack_from('<H', rec, 10); crc, cs, us = struct.unpack_from('<III', rec, 16)
    nl, xl, cl = struct.unpack_from('<HHH', rec, 28); hoff, = struct.unpack_from('<I', rec, 42)
    fname = cd[p + 46:p + 46 + nl].decode('utf-8'); tailrec = cd[p + 46:p + 46 + nl + xl + cl]
    p += 46 + nl + xl + cl
    assert meth == 0, (fname, meth)
    f.seek(hoff); lh = bytearray(f.read(30)); assert lh[:4] == b'PK\x03\x04'
    lnl, lxl = struct.unpack_from('<HH', lh, 26); lh += f.read(lnl + lxl)
    flags, = struct.unpack_from('<H', lh, 6); assert not flags & 8, 'data descriptor'
    newoff = out.tell()
    if fname == name:
        hit += 1; data = new
        struct.pack_into('<III', lh, 14, newcrc, len(new), len(new))
        struct.pack_into('<III', rec, 16, newcrc, len(new), len(new))
        out.write(lh); out.write(new)
    else:
        out.write(lh); left = cs
        while left:
            b = f.read(min(left, 16 << 20)); out.write(b); left -= len(b)
    struct.pack_into('<I', rec, 42, newoff)
    recs.append(bytes(rec) + tailrec)
assert hit == 1
cdo = out.tell(); cdb = b''.join(recs); out.write(cdb)
assert out.tell() < 2**32
struct.pack_into('<II', eocd, 12, len(cdb), cdo); out.write(eocd); out.write(cmt); out.close()
print('size', os.path.getsize(dst), hex(os.path.getsize(dst)))
