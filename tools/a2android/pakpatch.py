# pakpatch.py <orig.pak> <out.pak> <pak path> <replacement file>
# Replaces one file in a UE5 v11 pak: new uncompressed entry appended after the data,
# encoded entry/PHI/FDI/primary index/footer rebuilt with fresh SHA1s.
import sys, struct, hashlib
sys.path.insert(0, __import__('os').path.dirname(__file__))
import pak11

src, dst, target, repl = sys.argv[1:5]
p = pak11.Pak(src)
new = open(repl, 'rb').read()
locs = dict(p.files()); old_loc = locs[target]
print('old entry', p.entry(old_loc))

# layout check: primary index, then PHI, then FDI, then footer
assert p.idxoff + p.idxsize <= p.phi_off and p.phi_off + p.phi_size <= p.fdi_off and p.fdi_off + p.fdi_size == p.foot_start, \
    (p.idxoff, p.idxsize, p.phi_off, p.phi_size, p.fdi_off, p.fdi_size, p.foot_start)

out = bytearray(p.d[:p.idxoff])
entry_off = len(out)
sha = hashlib.sha1(new).digest()
out += pak11.inline_header_uncompressed(len(new), sha) + new
assert pak11.hdr_size(0, 0) == 53

# encoded entry appended to the encoded-entries blob
new_loc = len(p.enc_entries)
p.enc_entries += pak11.encode_uncompressed(entry_off, len(new))
chk = p.entry(new_loc); assert chk['offset'] == entry_off and chk['usize'] == len(new) and chk['method'] == 0

# FDI
n = 0
for dn, fl in p.dirs:
    for f in fl:
        if dn + f[0] == target: f[1] = new_loc; n += 1
assert n == 1
fdi = p.build_fdi()

# PHI: int32 count, (u64 hash, i32 loc)*count, then remainder kept verbatim
r = pak11.R(p.phi); cnt = r.i32(); phi = bytearray(p.phi); hits = 0
for k in range(cnt):
    pos = 4 + k * 12
    h, l = struct.unpack_from('<Qi', phi, pos)
    if l == old_loc: struct.pack_into('<Qi', phi, pos, h, new_loc); hits += 1
assert hits == 1, hits
phi = bytes(phi)

# primary index
def primary(phi_off, fdi_off):
    b = bytearray(pak11.wfstr(p.mount) + struct.pack('<iQ', p.num, p.seed))
    b += struct.pack('<iqq', 1, phi_off, len(phi)) + hashlib.sha1(phi).digest()
    b += struct.pack('<iqq', 1, fdi_off, len(fdi)) + hashlib.sha1(fdi).digest()
    b += struct.pack('<i', len(p.enc_entries)) + bytes(p.enc_entries) + struct.pack('<i', p.nfiles) + p.files_raw
    return bytes(b)

idx_off = len(out)
size0 = len(primary(0, 0))
phi_off = idx_off + size0; fdi_off = phi_off + len(phi)
idx = primary(phi_off, fdi_off); assert len(idx) == size0
out += idx + phi + fdi
out += p.guid + b'\0' + struct.pack('<IIQQ', pak11.MAGIC, p.ver, idx_off, len(idx)) + hashlib.sha1(idx).digest() + p.methods + p.foot_tail
open(dst, 'wb').write(out)

# verify by re-reading every file
q = pak11.Pak(dst)
assert q.read(dict(q.files())[target]) == new
bad = 0
for path, loc in q.files():
    if path == target: continue
    if q.entry(loc) != p.entry(locs[path]): bad += 1
print('entries', q.num, 'unchanged-entry mismatches', bad, 'size', len(out))
