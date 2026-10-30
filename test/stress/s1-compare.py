"""Merges two S1 result documents (tree codec, fast codec) into results/s1-fast-codec.json with per-cell speed-ups."""
import argparse
import datetime
import json
import pathlib

METRICS = ("eventsPerSec", "eventsPerCpuSec", "nsPerOp", "allocsPerOp", "bytesPerOp")


class Cells:
    """Indexes the result cells of one S1 document by (layer, shape, payload)."""

    def __init__(self, doc):
        self.doc = doc
        self.by_key = {(c["layer"], c["shape"], c["payload"]): c for c in doc["results"]}

    def mean(self, key, metric):
        cell = self.by_key.get(key, {})
        return cell.get(metric, {}).get("mean")


class Comparison:
    """Builds the comparison document: per cell tree vs fast means and the fast/tree ratio."""

    def __init__(self, tree, fast):
        self.tree, self.fast = Cells(tree), Cells(fast)

    def row(self, key):
        layer, shape, payload = key
        row = {"layer": layer, "shape": shape, "payload": payload}
        for m in METRICS:
            t, f = self.tree.mean(key, m), self.fast.mean(key, m)
            row[m] = {"tree": t, "fast": f, "ratio": (f / t) if t and f else None}
        return row

    def build(self):
        keys = sorted(set(self.tree.by_key) | set(self.fast.by_key))
        return {"scenario": "S1-fast-codec", "title": "S1 engine + runner matrix, tree codec vs fast codec",
                "generatedAt": datetime.datetime.now(datetime.timezone.utc).isoformat(),
                "env": self.fast.doc["env"], "cells": [self.row(k) for k in keys],
                "profiles": {"tree": self.tree.doc.get("profiles", {}), "fast": self.fast.doc.get("profiles", {})}}


if __name__ == "__main__":
    p = argparse.ArgumentParser()
    for flag in ("--tree", "--fast", "--out"):
        p.add_argument(flag, required=True)
    a = p.parse_args()
    tree = json.loads(pathlib.Path(a.tree).read_text())
    fast = json.loads(pathlib.Path(a.fast).read_text())
    pathlib.Path(a.out).write_text(json.dumps(Comparison(tree, fast).build(), indent=2) + "\n")
