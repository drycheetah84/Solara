# pecore.py -- minimal PE reader + instruction-aligned, displacement-masked byte patterns (capstone).
import struct, bisect, os
import capstone

EXE = {"22": r"C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\SpecEditorBuild\A2\Binaries\Win64\A2-Win64-Shipping.exe",
       "47": r"C:\Users\drych\Downloads\32586841537580874\32586841537580874\A2\Binaries\Win64\A2-Win64-Shipping.exe",
       "50": os.path.join(os.path.dirname(os.path.abspath(__file__)), "A2-50451.exe")}
cs = capstone.Cs(capstone.CS_ARCH_X86, capstone.CS_MODE_64); cs.detail = True

class PE:
    def __init__(s, path):
        s.d = open(path, "rb").read()
        e = struct.unpack_from("<I", s.d, 0x3C)[0]
        nsec = struct.unpack_from("<H", s.d, e + 6)[0]; osz = struct.unpack_from("<H", s.d, e + 20)[0]
        opt = e + 24
        s.size = struct.unpack_from("<I", s.d, opt + 56)[0]
        exc_rva, exc_sz = struct.unpack_from("<II", s.d, opt + 112 + 3 * 8)
        s.secs = []
        for i in range(nsec):
            o = opt + osz + i * 40
            name = s.d[o:o + 8].rstrip(b"\0").decode(errors="ignore")
            vsz, va, rsz, raw = struct.unpack_from("<IIII", s.d, o + 8)
            s.secs.append((name, va, max(vsz, rsz), raw, rsz))
        s.rf = []
        base = s.off(exc_rva)
        for i in range(exc_sz // 12):
            b, en, _ = struct.unpack_from("<III", s.d, base + i * 12)
            s.rf.append((b, en))
        s.rf.sort(); s.rfb = [x[0] for x in s.rf]; s.begins = set(s.rfb)
        s._dc = {}
    def off(s, rva):
        for n, va, sz, raw, rsz in s.secs:
            if va <= rva < va + sz and rva - va < rsz: return raw + rva - va
        return None
    def sec(s, rva):
        for n, va, sz, raw, rsz in s.secs:
            if va <= rva < va + sz: return n
        return None
    def read(s, rva, n):
        o = s.off(rva); return s.d[o:o + n] if o is not None else b""
    def chunk(s, rva):
        i = bisect.bisect_right(s.rfb, rva) - 1
        if i >= 0 and s.rf[i][0] <= rva < s.rf[i][1]: return s.rf[i]
        return None
    def insns(s, lo, hi):
        """instruction list (addr, size, bytes, maskset, insn) decoded linearly from the pdata chunk start."""
        c = s.chunk(lo)
        st = c[0] if c and c[0] <= lo else lo
        if lo - st > 0x2000:                       # huge chunk: resync from a nearby instruction boundary
            st2 = lo - 0x2000
            for i in cs.disasm(s.read(st2, 0x40), st2):
                pass
            st = st2
        key = (st, hi)
        if key in s._dc: return s._dc[key]
        if len(s._dc) > 200: s._dc.clear()
        out = []
        for ins in cs.disasm(s.read(st, hi - st), st):
            m = set()
            try:
                for op in ins.operands:
                    if op.type == capstone.x86.X86_OP_MEM and op.mem.base == capstone.x86.X86_REG_RIP:
                        m |= set(range(ins.disp_offset, ins.disp_offset + ins.disp_size))
                if (ins.group(capstone.CS_GRP_JUMP) or ins.group(capstone.CS_GRP_CALL)) and ins.imm_size == 4:
                    m |= set(range(ins.imm_offset, ins.imm_offset + 4))
            except Exception:
                pass
            out.append((ins.address, ins.size, bytes(ins.bytes), m, ins))
        s._dc[key] = out
        return out
    def window(s, v, before, after):
        """masked pattern covering whole instructions overlapping [v-before, v+after); returns (start, pattern)."""
        ins = s.insns(v - before, v + after + 16)
        sel = [x for x in ins if x[0] + x[1] > v - before and x[0] < v + after]
        if not sel or not any(x[0] == v for x in ins) and not any(x[0] <= v < x[0] + x[1] for x in ins): return None, None
        pat = []
        for a, n, b, m, _ in sel: pat += [None if i in m else b[i] for i in range(n)]
        return sel[0][0], pat
    def ins_at(s, v):
        for a, n, b, m, ins in s.insns(v, v + 16):
            if a == v: return ins
        return None

def find_masked(pe, lo, hi, pat, limit=3):
    lo = max(lo, 0x1000)
    hay = pe.read(lo, hi - lo); n = len(pat); hits = []
    fixed = [(i, v) for i, v in enumerate(pat) if v is not None]
    if not fixed: return hits
    # anchor on the longest run of fixed bytes
    best, cur, bs = (0, 0), None, 0
    runs, i = [], 0
    while i < n:
        if pat[i] is None: i += 1; continue
        j = i
        while j < n and pat[j] is not None: j += 1
        runs.append((j - i, i)); i = j
    ln, at = max(runs)
    needle = bytes(pat[at:at + ln])
    k = hay.find(needle)
    while k != -1:
        st = k - at
        if st >= 0 and st + n <= len(hay) and all(v is None or hay[st + q] == v for q, v in enumerate(pat)):
            hits.append(lo + st)
            if len(hits) >= limit: break
        k = hay.find(needle, k + 1)
    return hits


def call_index(pe):
    """{target_rva: [call_site_rva,...]} for every E8 rel32 / E9 rel32 in .text (built once, numpy)."""
    if getattr(pe, "_ci", None) is not None: return pe._ci
    import numpy as np
    for n, va, sz, raw, rsz in pe.secs:
        if n == ".text": break
    t = np.frombuffer(pe.d, dtype=np.uint8, count=rsz, offset=raw)
    idx = np.nonzero((t[:-5] == 0xE8) | (t[:-5] == 0xE9))[0]
    rel = (t[idx + 1].astype(np.int64) | (t[idx + 2].astype(np.int64) << 8) | (t[idx + 3].astype(np.int64) << 16) | (t[idx + 4].astype(np.int64) << 24))
    rel = np.where(rel >= 0x80000000, rel - 0x100000000, rel)
    tgt = va + idx + 5 + rel
    ok = (tgt >= 0x1000) & (tgt < pe.size)
    ci = {}
    for a, b in zip((va + idx[ok]).tolist(), tgt[ok].tolist()):
        ci.setdefault(b, []).append(a)
    pe._ci = ci
    return ci
