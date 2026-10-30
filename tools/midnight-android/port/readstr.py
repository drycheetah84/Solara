import sys
d=open(sys.argv[1],'rb').read()
o=int(sys.argv[2],16)
print("string @ %#x:"%o, repr(d[o:o+80].split(b'\x00')[0]))
