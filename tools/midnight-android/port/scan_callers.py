import sys, struct
from collections import Counter, defaultdict
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
def is_bl(w): return (w&0xFC000000)==0x94000000
def bl_tgt(i):
    w=words[i]; imm=w&0x3FFFFFF
    if imm&0x2000000: imm-=0x4000000
    return i+imm
def movz_w1(w, imm):  # movz w1,#imm lsl0
    return w==(0x52800000 | ((imm&0xffff)<<5) | 1)
# common CURLOPT_* pointer options UE/EOS use
OPTS={10002:"URL",10001:"WRITEDATA",10015:"POSTFIELDS",10023:"HTTPHEADER",
      10018:"USERAGENT",10005:"USERPWD",10004:"PROXY",10009:"READDATA",
      10082:"SSL_CTX_DATA",10052:"COOKIEJAR"}
tally=defaultdict(Counter)   # opt -> Counter(target)
url_targets=Counter()
for i in range(n):
    for opt in OPTS:
        if movz_w1(words[i],opt):
            # next bl within 6 instrs
            for j in range(1,7):
                if i+j<n and is_bl(words[i+j]):
                    t=bl_tgt(i+j); tally[opt][va(t)]+=1
                    if opt==10002: url_targets[va(t)]+=1
                    break
print("=== CURLOPT_URL(10002) call targets ===")
for t,c in url_targets.most_common(8): print("  %#x : %d calls"%(t,c))
# intersection: target that appears for MANY different options = curl_easy_setopt
allt=Counter()
for opt,c in tally.items():
    for t,cnt in c.items(): allt[t]+=cnt
print("=== targets summed across all option constants ===")
for t,c in allt.most_common(8):
    opts=[OPTS[o] for o in OPTS if allt and t in tally[o]]
    print("  %#x : %d total, seen for opts %s"%(t,c,opts))
