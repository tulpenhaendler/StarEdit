#!/usr/bin/env python3
"""Regenerate items.txt from the game's IoStore directory index.

usage: sudo python3 tools/extract_items.py \
    /opt/starrupture/server/StarRupture/Content/Paks/pakchunk0-WindowsServer.utoc > internal/items/items.txt
"""
import re
import struct
import sys

d = open(sys.argv[1], "rb").read()
NONE = 0xFFFFFFFF


def fstring(o):
    n = struct.unpack_from("<i", d, o)[0]
    o += 4
    if n >= 0:
        return d[o:o + max(n - 1, 0)].decode("utf8"), o + n
    return d[o:o - 2 * n - 2].decode("utf-16le"), o - 2 * n


# The directory index starts with its mount point, an FString "../../../".
_, o = fstring(d.index(b"../../../") - 4)
(nd,) = struct.unpack_from("<I", d, o)
o += 4
dirs = [struct.unpack_from("<IIII", d, o + 16 * k) for k in range(nd)]  # name, child, sibling, file
o += 16 * nd
(nf,) = struct.unpack_from("<I", d, o)
o += 4
files = [struct.unpack_from("<III", d, o + 12 * k) for k in range(nf)]  # name, next, userdata
o += 12 * nf
(ns,) = struct.unpack_from("<I", d, o)
o += 4
names = []
for _ in range(ns):
    s, o = fstring(o)
    names.append(s)

paths = []
stack = [(0, "")]
while stack:
    di, parent = stack.pop()
    name, child, sibling, f = dirs[di]
    path = parent + (names[name] + "/" if name != NONE else "")
    while f != NONE:
        fname, f, _ = files[f]
        paths.append(path + names[fname])
    if sibling != NONE:
        stack.append((sibling, parent))
    if child != NONE:
        stack.append((child, path))

for p in sorted(set(paths)):
    m = re.fullmatch(r"StarRupture/Content/(.*)/(I_[^/]+)\.uasset", p)
    if m:
        print(f"/Game/{m[1]}/{m[2]}.{m[2]}_C")
