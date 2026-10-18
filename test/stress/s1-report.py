"""Turns S1 go test benchmark output and pprof top-15 summaries into results/s1-engine.json."""
import argparse
import datetime
import json
import pathlib
import re
import statistics

BENCH_LINE = re.compile(r"^(Benchmark\S+?)(?:-\d+)?\s+(\d+)\s+(.*)$")
METRIC = re.compile(r"([\d.]+)\s+(\S+)")
TOP_LINE = re.compile(r"^\s*([\d.]+\w*)\s+([\d.]+)%\s+([\d.]+)%\s+([\d.]+\w*)\s+([\d.]+)%\s+(.+)$")
UNIT_KEYS = {"ns/op": "nsPerOp", "B/op": "bytesPerOp", "allocs/op": "allocsPerOp", "MB/s": "mbPerSec", "events/s": "eventsPerSec"}


class BenchSamples:
    """Collects repeated benchmark runs and summarises each metric (mean/min/max/stddev)."""

    def __init__(self):
        self.runs = {}

    def add_file(self, layer, path):
        for line in path.read_text().splitlines():
            m = BENCH_LINE.match(line.strip())
            if m:
                self.add(layer, m.group(1), int(m.group(2)), m.group(3))

    def add(self, layer, name, iterations, rest):
        sample = {"iterations": iterations}
        for value, unit in METRIC.findall(rest):
            sample[UNIT_KEYS.get(unit, unit)] = float(value)
        self.runs.setdefault((layer, name), []).append(sample)

    def summary(self):
        return [self.cell(layer, name, samples) for (layer, name), samples in self.runs.items()]

    def cell(self, layer, name, samples):
        parts = dict(p.split("=", 1) for p in name.split("/")[1:] if "=" in p)
        out = {"layer": layer, "benchmark": name, "shape": parts.get("shape", "js"), "payload": parts.get("payload"), "runs": len(samples)}
        for key in sorted({k for s in samples for k in s}):
            vals = [s[key] for s in samples if key in s]
            out[key] = {"mean": statistics.fmean(vals), "min": min(vals), "max": max(vals),
                        "stddevPct": (statistics.pstdev(vals) / statistics.fmean(vals) * 100) if statistics.fmean(vals) else 0.0}
        return out


class ProfileTop:
    """Parses a pprof -top listing into rows."""

    def __init__(self, path):
        self.path = path

    def rows(self):
        if not self.path.exists():
            return []
        rows = []
        for line in self.path.read_text().splitlines():
            m = TOP_LINE.match(line)
            if m:
                rows.append({"flat": m.group(1), "flatPct": float(m.group(2)), "sumPct": float(m.group(3)),
                             "cum": m.group(4), "cumPct": float(m.group(5)), "function": m.group(6).strip()})
        return rows


class Report:
    """Assembles the S1 result document."""

    def __init__(self, args):
        self.args = args

    def cpu_model(self):
        for line in pathlib.Path("/proc/cpuinfo").read_text().splitlines():
            if line.startswith("model name"):
                return line.split(":", 1)[1].strip()
        return "unknown"

    def profiles(self):
        out = {}
        for top in sorted(pathlib.Path(self.args.profiles).glob("*.top15.txt")):
            name = top.name[: -len(".top15.txt")]
            out[name] = {"profile": str(top.with_name(name + ".cpu.pprof")), "topFlat": ProfileTop(top).rows(),
                         "topCum": ProfileTop(top.with_name(name + ".top15-cum.txt")).rows()}
        return out

    def build(self):
        samples = BenchSamples()
        for raw in sorted(pathlib.Path(self.args.raw).glob("*.txt")):
            samples.add_file(raw.stem, raw)
        return {"scenario": "S1", "title": "engine-only step type x payload matrix",
                "generatedAt": datetime.datetime.now(datetime.timezone.utc).isoformat(),
                "env": {"go": self.args.go, "cpuModel": self.cpu_model(), "pinnedCpu": self.args.cpu, "gomaxprocs": 1,
                        "benchtime": self.args.benchtime, "count": self.args.count},
                "results": samples.summary(), "profiles": self.profiles()}


if __name__ == "__main__":
    p = argparse.ArgumentParser()
    for flag in ("--raw", "--profiles", "--out", "--benchtime", "--count", "--cpu", "--go"):
        p.add_argument(flag, required=True)
    a = p.parse_args()
    pathlib.Path(a.out).write_text(json.dumps(Report(a).build(), indent=2) + "\n")
