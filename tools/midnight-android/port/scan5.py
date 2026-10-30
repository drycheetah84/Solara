import sys, struct
path=sys.argv[1]
data=open(path,"rb").read()
e_phoff=struct.unpack_from("<Q",data,0x20)[0]
e_phnum=struct.unpack_from("<H",data,0x38)[0]; e_phent=struct.unpack_from("<H",data,0x36)[0]
loads=[]
for i in range(e_phnum):
    o=e_phoff+i*e_phent
    pt,pf=struct.unpack_from("<II",data,o); poff,pva=struct.unpack_from("<QQ",data,o+8); pfs=struct.unpack_from("<Q",data,o+0x20)[0]
    if pt==1: loads.append((poff,pva,pfs,pf))
po,pv,fs,_=[l for l in loads if l[3]&1][0]
seg=data[po:po+fs]; n=len(seg)//4
words=list(struct.unpack_from("<%dI"%n,seg,0))
def va(i): return pv+i*4
def is_sub_sp(w): return (w&0xFF0003FF)==0xD10003FF
def is_bl(w): return (w&0xFC000000)==0x94000000
def bl_tgt(i):
    w=words[i]; imm=w&0x3FFFFFF
    if imm&0x2000000: imm-=0x4000000
    return i+imm
MOVZ2B=lambda w:(w&0xFFFFFFE0)==0x52800560
# stp xPAIR,[Rn,#imm] with Rn in {sp=31, x29=29}; identify pair by (Rt,Rt2)
def pair(w):
    if (w&0xFFC00000)!=0xA9000000: return None
    Rt=w&0x1f; Rn=(w>>5)&0x1f; Rt2=(w>>10)&0x1f
    if Rn not in (29,31): return None
    if (Rt,Rt2)==(2,3): return 23
    if (Rt,Rt2)==(4,5): return 45
    if (Rt,Rt2)==(6,7): return 67
    return None
# sliding 8-window containing all 3 pairs
found={}
for i in range(n-8):
    s=set()
    for j in range(8):
        p=pair(words[i+j])
        if p: s.add(p)
    if s=={23,45,67}:
        # function start
        fstart=None
        for b in range(0,16):
            if i-b<0: break
            if is_sub_sp(words[i-b]): fstart=i-b; break
        if fstart is None: fstart=i
        if fstart in found: continue
        body=words[fstart:min(fstart+0x400,n)]
        has43=any(MOVZ2B(x) for x in body)
        bls=[bl_tgt(fstart+j) for j,x in enumerate(body) if is_bl(x)]
        found[fstart]={"func_va":hex(va(fstart)),"save_va":hex(va(i)),"has43":has43,
                       "nbl":len(bls),"bls":[hex(va(b)) for b in dict.fromkeys(bls)][:8]}
cands=list(found.values())
print("variadic(x2-x7) functions:",len(cands))
for c in cands:
    if c["has43"]: print("  [RET43]",c)
