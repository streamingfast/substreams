#!/usr/bin/env python3
"""Turn live-benchmark.sh's results.tsv into a comparison table.

Correctness is checked before any speedup is printed: for a given endpoint and size the
two variants must have produced the same rows, the same blocks and the same fingerprint.
A speedup over data that does not match is meaningless, so it is reported as a mismatch
rather than as a number.
"""

import collections
import pathlib
import sys

Row = collections.namedtuple("Row", "endpoint size variant seconds rows blocks fp rc drain")


def load(path):
    lines = pathlib.Path(path).read_text().splitlines()
    out = []
    for line in lines[1:]:
        parts = line.split("\t")
        if len(parts) < 8:
            continue
        drain = float(parts[8]) if len(parts) > 8 and parts[8] else None
        out.append(Row(parts[0], int(parts[1]), parts[2], float(parts[3]),
                       int(parts[4] or 0), int(parts[5] or 0), parts[6], int(parts[7]), drain))
    return out


def short(endpoint):
    return endpoint.replace("https://", "").replace(".streamingfast.io", "")


def main(argv):
    path = argv[1] if len(argv) > 1 else ".sinkbench/results.tsv"
    rows = load(path)

    failed = [r for r in rows if r.rc != 0]
    good = [r for r in rows if r.rc == 0]
    if failed:
        print("failed runs, excluded:")
        for r in failed:
            print(f"  {short(r.endpoint)} {r.size} {r.variant} rc={r.rc}")
        print()

    pairs = collections.defaultdict(dict)
    for r in good:
        pairs[(r.endpoint, r.size)][r.variant] = r

    print(f"{'endpoint':22s} {'blocks':>8s} {'rows':>12s} "
          f"{'accumulator':>12s} {'cache':>10s} {'speedup':>8s} {'identical':>10s} {'drain a/c':>14s}")
    for (endpoint, size), variants in sorted(pairs.items(), key=lambda kv: (kv[0][0], kv[0][1])):
        acc, cache = variants.get("accumulator"), variants.get("cache")
        if not (acc and cache):
            only = next(iter(variants.values()))
            print(f"{short(endpoint):22s} {size:>8,} {only.rows:>12,} "
                  f"{'(only ' + only.variant + ')':>32s}")
            continue

        identical = acc.rows == cache.rows and acc.blocks == cache.blocks and acc.fp == cache.fp
        drain = "-"
        if acc.drain is not None and cache.drain is not None:
            drain = f"{acc.drain:.1f}s/{cache.drain:.1f}s"
        print(f"{short(endpoint):22s} {size:>8,} {acc.rows:>12,} "
              f"{acc.seconds:>11.1f}s {cache.seconds:>9.1f}s "
              f"{acc.seconds / cache.seconds:>7.2f}x "
              f"{'yes' if identical else 'NO -- MISMATCH':>10s} {drain:>14s}")

    repeats = collections.defaultdict(list)
    for r in good:
        repeats[(r.endpoint, r.size, r.variant)].append(r.seconds)
    dupes = {k: v for k, v in repeats.items() if len(v) > 1}
    if dupes:
        print("\nrepeat measurements:")
        for (endpoint, size, variant), seconds in sorted(dupes.items()):
            spread = (max(seconds) - min(seconds)) / min(seconds) * 100
            print(f"  {short(endpoint)} {size:,} {variant}: "
                  f"{', '.join(f'{s:.1f}s' for s in seconds)}  (spread {spread:.1f}%)")

    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
