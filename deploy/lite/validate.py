#!/usr/bin/env python3
"""Static checks of the lite compose profile (no docker needed): YAML, interpolation, references, env keys.

Usage: python3 deploy/lite/validate.py [--env-file PATH] [--emit-env DIR]
Exit status 0 when every check passes; each check prints PASS/FAIL lines.
"""
import argparse
import base64
import binascii
import json
import pathlib
import re
import shutil
import subprocess
import sys

import yaml

LITE_DIR = pathlib.Path(__file__).resolve().parent
REPO_ROOT = LITE_DIR.parent.parent
COMPOSE_FILES = ("docker-compose.yml", "docker-compose.tls.yml")
TEMPLATE = LITE_DIR / ".env.example"
GO_SOURCE_DIRS = ("internal", "cmd")
ROUTER_PREFIX = "ROUTER_"
METRICS_PROFILE = "metrics"
PUBLIC_SERVICES = {"web"}
LOOPBACK = "127.0.0.1"
VALID_CONDITIONS = {"service_started", "service_healthy", "service_completed_successfully"}
VAR_PATTERN = re.compile(r"\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?[-?])([^}]*))?\}|\$\$")
KEY_LINE = re.compile(r"^\s*#?\s*([A-Z][A-Z0-9_]*)=(.*)$")
ACTIVE_KEY_LINE = re.compile(r"^([A-Z][A-Z0-9_]*)=(.*)$")
GO_KEY = re.compile(r'"(ROUTER_[A-Z0-9_]+)"')
DISTROLESS_DEBUG_TOOLS = {"/busybox/wget", "/app"}
IMAGE_TOOLS = {
    "postgres:16-alpine": {"pg_isready"},
    "redis:7-alpine": {"redis-cli"},
    "prom/prometheus:v2.55.1": {"wget"},
    "event-router-lite/web:latest": {"wget"},
    "event-router-lite/api:latest": DISTROLESS_DEBUG_TOOLS,
    "event-router-lite/worker:latest": DISTROLESS_DEBUG_TOOLS,
    "event-router-lite/preview:latest": DISTROLESS_DEBUG_TOOLS,
}
FILE_ENV_SUFFIX = "_FILE"
SHELL_SCRIPTS = ("init.sh", "smoke.sh", "certs/init-certs.sh")
POSIX_SHELL = "sh"
SHELLCHECK = "shellcheck"
TLS_OVERLAY = COMPOSE_FILES[1]
MASTER_KEY_ENV = ("ROUTER_SECRETS_MASTER_KEY", "ROUTER_SECRETS_MASTER_KEY_FILE")
MASTER_KEY_BYTES = 32


class Report:
    """Collects PASS/FAIL lines and the overall verdict."""

    def __init__(self):
        self.failures = 0

    def check(self, name, problems):
        if problems:
            self.failures += 1
            for p in problems:
                print(f"FAIL {name}: {p}")
        else:
            print(f"PASS {name}")


class EnvFile:
    """Reads KEY=value lines of a compose .env file (active keys) or the template (active + commented)."""

    def __init__(self, path, include_commented=False):
        self.path = path
        pattern = KEY_LINE if include_commented else ACTIVE_KEY_LINE
        self.values = {}
        for line in path.read_text().splitlines():
            m = pattern.match(line)
            if m:
                self.values[m.group(1)] = m.group(2).strip()


class Interpolator:
    """Applies compose ${VAR}, ${VAR:-default}, ${VAR:?error} and $$ rules; records used and missing vars."""

    def __init__(self, env):
        self.env = env
        self.used, self.required, self.missing = set(), set(), set()

    def text(self, value):
        return VAR_PATTERN.sub(self._replace, value)

    def _replace(self, m):
        if m.group(0) == "$$":
            return "$"
        name, op, arg = m.group(1), m.group(2), m.group(3) or ""
        self.used.add(name)
        current = self.env.get(name, "")
        if op in (":?", "?"):
            self.required.add(name)
            if not current:
                self.missing.add(name)
        if op in (":-", "-") and not current:
            return arg
        return current

    def walk(self, node):
        if isinstance(node, dict):
            return {k: self.walk(v) for k, v in node.items()}
        if isinstance(node, list):
            return [self.walk(v) for v in node]
        if isinstance(node, str):
            return self.text(node)
        return node


class ComposeModel:
    """Loads and merges the compose files the way `docker compose -f a -f b` does for the checked fields."""

    def __init__(self, interpolators):
        self.files = {}
        for name in COMPOSE_FILES:
            data = yaml.safe_load((LITE_DIR / name).read_text())
            self.files[name] = interpolators[name].walk(data)
        self.base = self.files[COMPOSE_FILES[0]]
        self.services = self.base.get("services", {})

    def overlay_services(self):
        return self.files[COMPOSE_FILES[1]].get("services", {})


class GoKeys:
    """Every ROUTER_* key literal in non-test Go sources of the binaries."""

    def __init__(self):
        self.keys = set()
        for d in GO_SOURCE_DIRS:
            for path in (REPO_ROOT / d).rglob("*.go"):
                if not path.name.endswith("_test.go"):
                    self.keys.update(GO_KEY.findall(path.read_text(errors="ignore")))


class Checks:
    """The individual static checks over the merged model."""

    def __init__(self, model, interps, template, env):
        self.m, self.template, self.env = model, template, env
        self.base, self.overlay = interps[COMPOSE_FILES[0]], interps[TLS_OVERLAY]
        self.overlay_enabled = TLS_OVERLAY in env.values.get("COMPOSE_FILE", "")

    def depends_on(self):
        out = []
        for name, svc in self.m.services.items():
            for target, spec in (svc.get("depends_on") or {}).items():
                if target not in self.m.services:
                    out.append(f"{name} depends on unknown service {target}")
                    continue
                cond = (spec or {}).get("condition", "service_started")
                out += self._dependency_problems(name, target, cond)
        return out

    def _dependency_problems(self, name, target, cond):
        out, dep = [], self.m.services[target]
        if cond not in VALID_CONDITIONS:
            out.append(f"{name} -> {target}: unknown condition {cond}")
        if cond == "service_healthy" and "healthcheck" not in dep:
            out.append(f"{name} waits for {target} healthy but it has no healthcheck")
        if cond == "service_completed_successfully" and dep.get("restart") != "no":
            out.append(f"{name} waits for {target} to complete but it restarts")
        if dep.get("profiles") and not self.m.services[name].get("profiles"):
            out.append(f"{name} (always on) depends on profiled service {target}")
        return out

    def documented_vars(self):
        used = self.base.used | self.overlay.used
        return [f"${{{v}}} not documented in .env.example" for v in sorted(used) if v not in self.template.values]

    def required_vars(self):
        missing = self.base.missing | (self.overlay.missing if self.overlay_enabled else set())
        return [f"required ${{{v}}} empty or missing in {self.env.path}" for v in sorted(missing)]

    def master_key(self):
        env = self._env(self.m.services["api"])
        sources = [k for k in MASTER_KEY_ENV if env.get(k)]
        if len(sources) != 1:
            return [f"api needs exactly one of {' / '.join(MASTER_KEY_ENV)}, has {sources or 'none'}"]
        if sources[0] != MASTER_KEY_ENV[0]:
            return []
        try:
            size = len(base64.b64decode(env[MASTER_KEY_ENV[0]], validate=True))
        except (binascii.Error, ValueError):
            return ["ROUTER_SECRETS_MASTER_KEY is not valid base64"]
        return [] if size == MASTER_KEY_BYTES else [f"ROUTER_SECRETS_MASTER_KEY decodes to {size} bytes, want {MASTER_KEY_BYTES}"]

    def router_keys(self, go_keys):
        out = []
        for name, svc in self.m.services.items():
            for key in self._env(svc):
                if key.startswith(ROUTER_PREFIX) and key not in go_keys.keys:
                    out.append(f"{name}: {key} is not read by any Go binary")
        return out

    def declared_resources(self):
        out, volumes, networks = [], set(self.m.base.get("volumes") or {}), set(self.m.base.get("networks") or {})
        for name, svc in self.m.services.items():
            for vol in svc.get("volumes") or []:
                source = vol.split(":")[0]
                if not source.startswith((".", "/")) and source not in volumes:
                    out.append(f"{name}: volume {source} not declared")
            out += [f"{name}: network {n} not declared" for n in svc.get("networks") or [] if n not in networks]
        return out

    def host_files(self):
        out = []
        for name, svc in list(self.m.services.items()) + list(self.m.overlay_services().items()):
            for vol in svc.get("volumes") or []:
                source = vol.split(":")[0]
                if source.startswith(".") and not (LITE_DIR / source).exists():
                    out.append(f"{name}: host path {source} does not exist")
        return out

    def profiles(self):
        out = []
        for name, svc in self.m.services.items():
            expected = [METRICS_PROFILE] if name == "prometheus" else None
            if svc.get("profiles") != expected:
                out.append(f"{name}: profiles {svc.get('profiles')} (want {expected})")
        return out

    def healthchecks(self):
        out = []
        for name, svc in self.m.services.items():
            test = (svc.get("healthcheck") or {}).get("test")
            if not test or test[0] != "CMD":
                continue
            tools = IMAGE_TOOLS.get(svc.get("image"), set())
            if test[1] not in tools:
                out.append(f"{name}: healthcheck binary {test[1]} not known in image {svc.get('image')}")
        return out

    def ports(self):
        out = []
        for name, svc in self.m.services.items():
            for port in svc.get("ports") or []:
                if name not in PUBLIC_SERVICES and not str(port).startswith(LOOPBACK + ":"):
                    out.append(f"{name}: port {port} is not bound to {LOOPBACK}")
        return out

    def file_env_mounted(self):
        out = []
        for name, svc in self.m.services.items():
            targets = [v.split(":")[1] for v in svc.get("volumes") or []]
            for key, value in self._env(svc).items():
                if key.endswith(FILE_ENV_SUFFIX) and value and value.startswith("/"):
                    if not any(value.startswith(t.rstrip("/") + "/") for t in targets):
                        out.append(f"{name}: {key}={value} is not inside a mounted volume")
        return out

    @staticmethod
    def scripts():
        out = []
        for script in SHELL_SCRIPTS:
            path = LITE_DIR / script
            result = subprocess.run([POSIX_SHELL, "-n", str(path)], capture_output=True, text=True)
            if result.returncode != 0:
                out.append(f"{script}: {result.stderr.strip()}")
            if shutil.which(SHELLCHECK):
                lint = subprocess.run([SHELLCHECK, "-s", POSIX_SHELL, str(path)], capture_output=True, text=True)
                out += [f"{script}: {line}" for line in lint.stdout.splitlines() if line.startswith("In ")]
        return out

    @staticmethod
    def _env(svc):
        env = svc.get("environment") or {}
        return {k: "" if v is None else str(v) for k, v in env.items()}


def run(args):
    env_path = pathlib.Path(args.env_file).resolve() if args.env_file else (LITE_DIR / ".env" if (LITE_DIR / ".env").exists() else TEMPLATE)
    report, env = Report(), EnvFile(env_path)
    print(f"validate: env file {env_path}")
    interps = {name: Interpolator(env.values) for name in COMPOSE_FILES}
    model = ComposeModel(interps)
    yaml.safe_load((LITE_DIR / "prometheus" / "prometheus.yml").read_text())
    report.check("yaml parse (compose files, prometheus.yml)", [])
    checks = Checks(model, interps, EnvFile(TEMPLATE, include_commented=True), env)
    report.check("depends_on targets and conditions", checks.depends_on())
    report.check("compose variables documented in .env.example", checks.documented_vars())
    if env_path == TEMPLATE:
        print("SKIP required variables / master key: validating the template (secrets are filled by init.sh)")
    else:
        report.check("required variables set", checks.required_vars())
        report.check("api master key (exactly one source, 32 bytes)", checks.master_key())
    report.check("ROUTER_* keys read by the Go binaries", checks.router_keys(GoKeys()))
    report.check("volumes and networks declared", checks.declared_resources())
    report.check("bind-mounted host files exist", checks.host_files())
    report.check("profiles (prometheus only under 'metrics')", checks.profiles())
    report.check("healthcheck binaries exist in images", checks.healthchecks())
    report.check("host ports loopback-only except web", checks.ports())
    report.check("*_FILE paths inside mounted volumes", checks.file_env_mounted())
    lint = "sh -n + shellcheck" if shutil.which(SHELLCHECK) else "sh -n (shellcheck not installed)"
    report.check(f"shell scripts ({lint})", Checks.scripts())
    if args.emit_env:
        emit_env(model, pathlib.Path(args.emit_env))
    print("validate: OK" if report.failures == 0 else f"validate: {report.failures} check(s) failed")
    return 0 if report.failures == 0 else 1


def emit_env(model, out_dir):
    """Writes each service's resolved environment as JSON (input for deeper config-loader checks)."""
    out_dir.mkdir(parents=True, exist_ok=True)
    for name, svc in model.services.items():
        (out_dir / f"{name}.json").write_text(json.dumps(Checks._env(svc), indent=1, sort_keys=True))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--env-file", help="env file to interpolate with (default: .env, else .env.example)")
    parser.add_argument("--emit-env", help="directory to write each service's resolved environment as JSON")
    sys.exit(run(parser.parse_args()))
