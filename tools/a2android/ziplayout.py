import sys, struct, zipfile
f = open(sys.argv[1], 'rb'); z = zipfile.ZipFile(sys.argv[1])
for i in z.infolist():
    if not (i.filename.endswith(('.utoc', '.ucas', '.pak')) or i.filename.startswith('A2/A2.uproject')): continue
    f.seek(i.header_offset); h = f.read(30)
    sig, ver, flg, meth, t, d, crc, cs, us, nl, xl = struct.unpack('<IHHHHHIIIHH', h)
    data = i.header_offset + 30 + nl + xl
    print(f"{i.filename:45} hdr={i.header_offset:#x} localExtra={xl} cdExtra={len(i.extra)} data={data:#x} align4k={data % 4096} cs={cs:#x}")
print('total', len(z.infolist()), 'first', z.infolist()[0].filename, 'last', z.infolist()[-1].filename)
