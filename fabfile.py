"""
Fabric release tasks for TraceNexBiz / TraceNex Partner.

Flow:
    local git push
      -> server git fetch/checkout
      -> server podman build partner-api image
      -> server podman push to ACR when configured
      -> server blue-green deploy from the image

Setup:
    pip install fabric

Common usage from the TraceNexBiz repo root:
    fab info
    fab preflight --target=ceshi
    fab check --target=ceshi
    fab network --target=ceshi
    fab sync-code --target=ceshi --ref=origin/main
    fab build --target=ceshi --tag=1.2.1-tracenex --ref=origin/main
    fab push-image --target=ceshi --tag=1.2.1-tracenex
    fab deploy --target=ceshi --tag=1.2.1-tracenex
    fab release --target=ceshi --tag=1.2.1-tracenex --ref=origin/main
    fab logs --target=ceshi --tail=200

Known targets:
    ceshi: ssh -i ~/.ssh/ceshi_cd.pem -p 58422 root@8.156.88.148

Override with environment variables when needed:
    TNBIZ_TARGET
    TNBIZ_HOST, TNBIZ_PORT, TNBIZ_USER, TNBIZ_KEY
    TNBIZ_REPO_URL, TNBIZ_SRC_DIR, TNBIZ_BUILD_DIR
    TNBIZ_REGISTRY, TNBIZ_NAMESPACE, TNBIZ_REPO
"""

from __future__ import annotations

import os
import re
import shlex
import subprocess
import tarfile
import tempfile
from pathlib import Path

from fabric import Connection, task

TARGETS = {
    "ceshi": {
        "host": "8.156.88.148",
        "port": 58422,
        "user": "root",
        "key": "~/.ssh/ceshi_cd.pem",
        "app_dir": "/opt/tracenexbiz",
        "src_dir": "/root/TraceNexBiz",
        "build_dir": "/tmp/tracenexbiz-build",
        "registry": "transnext-acr-ee-registry-vpc.cn-hangzhou.cr.aliyuncs.com",
        "namespace": "tracenexbiz",
        "repo": "partner-api",
        "repo_url": "git@github.com:seraph0017/TraceNexBiz.git",
        "region": "cn-chengdu",
        "internal_ip": "172.24.203.146",
        "rds_host": "rm-2vcnldc9an5s17ho9.mysql.cn-chengdu.rds.aliyuncs.com",
        "rds_port": 3306,
        "redis_host": "r-2vczy7qnmxs5p1ryhi.redis.cn-chengdu.rds.aliyuncs.com",
        "redis_port": 6379,
    },
}

DEFAULT_REPO_URL = "git@github.com:seraph0017/TraceNexBiz.git"
DEFAULT_REF = os.getenv("TNBIZ_DEFAULT_REF", "origin/main")
DEPLOY_LOCK = os.getenv("TNBIZ_DEPLOY_LOCK", "/tmp/tracenexbiz-deploy.lock")
SAFE_ARG_RE = re.compile(r"^[A-Za-z0-9._/@:+-]+$")


def _validate_arg(name: str, value: str) -> str:
    if not value:
        raise ValueError(f"{name} is required")
    if not SAFE_ARG_RE.match(value):
        raise ValueError(f"unsafe {name}: {value!r}")
    return value


def _q(value: str) -> str:
    return shlex.quote(value)


def _config(target: str | None = None) -> dict[str, object]:
    name = target or os.getenv("TNBIZ_TARGET", "ceshi")
    if name not in TARGETS:
        raise ValueError(f"unknown target {name!r}; expected one of: {', '.join(TARGETS)}")
    cfg = dict(TARGETS[name])
    cfg["name"] = name

    overrides = {
        "host": "TNBIZ_HOST",
        "port": "TNBIZ_PORT",
        "user": "TNBIZ_USER",
        "key": "TNBIZ_KEY",
        "app_dir": "TNBIZ_APP_DIR",
        "src_dir": "TNBIZ_SRC_DIR",
        "build_dir": "TNBIZ_BUILD_DIR",
        "registry": "TNBIZ_REGISTRY",
        "namespace": "TNBIZ_NAMESPACE",
        "repo": "TNBIZ_REPO",
        "repo_url": "TNBIZ_REPO_URL",
        "rds_host": "TNBIZ_RDS_HOST",
        "rds_port": "TNBIZ_RDS_PORT",
        "redis_host": "TNBIZ_REDIS_HOST",
        "redis_port": "TNBIZ_REDIS_PORT",
    }
    for key, env_name in overrides.items():
        value = os.getenv(env_name)
        if value:
            cfg[key] = int(value) if key == "port" else value

    cfg["key"] = os.path.expanduser(str(cfg.get("key") or ""))
    cfg["env_file"] = os.getenv("TNBIZ_ENV_FILE", f"{cfg['app_dir']}/config/partner-api.env")
    cfg["nginx_conf"] = os.getenv("TNBIZ_NGINX_CONF", "/etc/nginx/conf.d/tracenexbiz.conf")
    cfg["log_dir"] = os.getenv("TNBIZ_LOG_DIR", f"{cfg['app_dir']}/logs")
    cfg["data_dir"] = os.getenv("TNBIZ_DATA_DIR", f"{cfg['app_dir']}/data")
    cfg["health_path"] = os.getenv("TNBIZ_HEALTH_PATH", "/healthz")
    cfg["container_port"] = int(os.getenv("TNBIZ_CONTAINER_PORT", "8080"))
    cfg["blue_port"] = int(os.getenv("TNBIZ_BLUE_PORT", "8080"))
    cfg["green_port"] = int(os.getenv("TNBIZ_GREEN_PORT", "8081"))
    cfg["memory"] = os.getenv("TNBIZ_MEM", "2g")
    cfg["cpus"] = os.getenv("TNBIZ_CPUS", "1")
    return cfg


def _image(cfg: dict[str, object], tag: str) -> str:
    tag = _validate_arg("tag", tag)
    return f"{cfg['registry']}/{cfg['namespace']}/{cfg['repo']}:{tag}"


def _connect(cfg: dict[str, object]) -> Connection:
    connect_kwargs = {}
    key = str(cfg.get("key") or "")
    if key:
        connect_kwargs["key_filename"] = key
    return Connection(
        host=str(cfg["host"]),
        user=str(cfg["user"]),
        port=int(cfg["port"]),
        connect_kwargs=connect_kwargs,
    )


def _run(c: Connection, command: str, *, warn: bool = False, hide: bool = False):
    if not hide:
        print(f"[{c.user}@{c.host}:{c.port}] $ {command}")
    return c.run(command, warn=warn, hide=hide, pty=False)


def _ensure_source_checkout(c: Connection, cfg: dict[str, object]):
    src_dir = str(cfg["src_dir"])
    parent = _q(str(Path(src_dir).parent))
    src = _q(src_dir)
    repo = _q(str(cfg.get("repo_url") or DEFAULT_REPO_URL))
    _run(
        c,
        " && ".join(
            [
                f"mkdir -p {parent}",
                f"if [ ! -d {src}/.git ]; then git clone {repo} {src}; fi",
                f"test -d {src}/.git",
            ]
        ),
    )


def _checkout_ref(c: Connection, cfg: dict[str, object], ref: str):
    ref = _validate_arg("ref", ref)
    src = _q(str(cfg["src_dir"]))
    quoted_ref = _q(ref)
    _run(
        c,
        " && ".join(
            [
                f"cd {src}",
                "git fetch origin --tags --prune",
                f"git checkout -f {quoted_ref}",
                f"git reset --hard {quoted_ref}",
                "git rev-parse --short HEAD",
            ]
        ),
    )


def _create_local_worktree_archive() -> Path:
    repo_root = Path(__file__).resolve().parent
    result = subprocess.run(
        ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
        cwd=repo_root,
        check=True,
        stdout=subprocess.PIPE,
    )
    paths = [p for p in result.stdout.decode("utf-8").split("\0") if p]
    if not paths:
        raise RuntimeError("no files found to archive")

    tmp_dir = Path(tempfile.mkdtemp(prefix="tracenexbiz-worktree-"))
    archive = tmp_dir / "worktree.tar.gz"
    with tarfile.open(archive, "w:gz") as tar:
        for rel in paths:
            path = repo_root / rel
            if path.is_file():
                tar.add(path, arcname=rel)
    return archive


@task(help={"target": "target name: ceshi"})
def info(ctx, target="ceshi"):
    """Print local Fabric deployment configuration."""
    cfg = _config(target)
    print(f"target:    {cfg['name']}")
    print(f"region:    {cfg.get('region', '-')}")
    print(f"host:      {cfg['user']}@{cfg['host']}:{cfg['port']}")
    print(f"internal:  {cfg.get('internal_ip', '-')}")
    print(f"key:       {cfg['key']}")
    print(f"app_dir:   {cfg['app_dir']}")
    print(f"src:       {cfg['src_dir']}")
    print(f"build_dir: {cfg['build_dir']}")
    print(f"repo_url:  {cfg.get('repo_url') or DEFAULT_REPO_URL}")
    print(f"image:     {cfg['registry']}/{cfg['namespace']}/{cfg['repo']}:<tag>")
    print(f"env_file:  {cfg['env_file']}")
    print(f"nginx:     {cfg['nginx_conf']}")
    print(f"ports:     blue={cfg['blue_port']} green={cfg['green_port']} container={cfg['container_port']}")
    print(f"rds:       {cfg.get('rds_host', '-')}:{cfg.get('rds_port', '-')}")
    print(f"redis:     {cfg.get('redis_host', '-')}:{cfg.get('redis_port', '-')}")


@task(help={"target": "target name: ceshi"})
def preflight(ctx, target="ceshi"):
    """Check SSH connectivity and OS basics without requiring app setup."""
    cfg = _config(target)
    key = str(cfg.get("key") or "")
    if key and not Path(key).exists():
        raise FileNotFoundError(f"SSH key not found: {key}")
    c = _connect(cfg)
    _run(c, "hostnamectl || uname -a")
    _run(c, "id && pwd && df -h / | awk 'NR==1 || NR==2'")
    _run(c, "command -v apt-get || command -v dnf || true")


@task(help={"target": "target name: ceshi"})
def check(ctx, target="ceshi"):
    """Check remote prerequisites for deployed partner-api service."""
    cfg = _config(target)
    key = str(cfg.get("key") or "")
    if key and not Path(key).exists():
        raise FileNotFoundError(f"SSH key not found: {key}")

    c = _connect(cfg)
    _run(c, "command -v git && command -v podman && command -v curl && command -v flock")
    _run(c, f"test -f {_q(str(cfg['env_file']))} && test -f {_q(str(cfg['nginx_conf']))}")
    _run(c, "podman info >/dev/null")
    _run(c, f"test -d {_q(str(cfg['app_dir']))}")


@task(help={"target": "target name: ceshi"})
def network(ctx, target="ceshi"):
    """Check ECS -> RDS/Redis intranet TCP reachability without credentials."""
    cfg = _config(target)
    c = _connect(cfg)
    rds = _q(str(cfg["rds_host"]))
    redis = _q(str(cfg["redis_host"]))
    rds_port = int(cfg["rds_port"])
    redis_port = int(cfg["redis_port"])
    _run(c, f"(nc -vz -w 5 {rds} {rds_port} || timeout 5 bash -c '</dev/tcp/{rds}/{rds_port}')")
    _run(c, f"(nc -vz -w 5 {redis} {redis_port} || timeout 5 bash -c '</dev/tcp/{redis}/{redis_port}')")


@task(help={"target": "target name: ceshi", "ref": "git ref to checkout"})
def sync_code(ctx, target="ceshi", ref=DEFAULT_REF):
    """Fetch and checkout code on the server source directory."""
    cfg = _config(target)
    c = _connect(cfg)
    _ensure_source_checkout(c, cfg)
    _checkout_ref(c, cfg, ref)


@task(
    help={
        "target": "target name: ceshi",
        "tag": "image tag to build, e.g. 1.2.1-tracenex",
        "ref": "git ref to checkout before build; defaults to the same value as tag",
        "pull": "pass --pull to podman build",
        "no_cache": "pass --no-cache to podman build",
    }
)
def build(ctx, target="ceshi", tag="", ref="", pull=True, no_cache=False):
    """Build the partner-api image on the server."""
    tag = _validate_arg("tag", tag)
    ref = ref or tag
    cfg = _config(target)
    c = _connect(cfg)
    _ensure_source_checkout(c, cfg)
    _checkout_ref(c, cfg, ref)

    flags = []
    if pull:
        flags.append("--pull")
    if no_cache:
        flags.append("--no-cache")

    image = _q(_image(cfg, tag))
    flag_str = " ".join(flags)
    build_dir = _q(str(cfg["build_dir"]))
    src_dir = _q(str(cfg["src_dir"]))
    _run(
        c,
        " && ".join(
            [
                f"rm -rf {build_dir}",
                f"mkdir -p {build_dir}",
                f"cd {src_dir}",
                f"git archive --format=tar HEAD | tar -x -C {build_dir}",
                f"cd {build_dir}",
                f"podman build {flag_str} -t {image} -f apps/partner-api/Dockerfile apps/partner-api",
                f"rm -rf {build_dir}",
            ]
        ),
    )


@task(
    help={
        "target": "target name: ceshi",
        "tag": "image tag to build, e.g. 1.2.1-tracenex",
        "pull": "pass --pull to podman build",
        "no_cache": "pass --no-cache to podman build",
    }
)
def build_local(ctx, target="ceshi", tag="", pull=True, no_cache=False):
    """Build the partner-api image on the server from the local worktree, including uncommitted files."""
    tag = _validate_arg("tag", tag)
    cfg = _config(target)
    c = _connect(cfg)

    flags = []
    if pull:
        flags.append("--pull")
    if no_cache:
        flags.append("--no-cache")

    archive = _create_local_worktree_archive()
    remote_archive = f"/tmp/tracenexbiz-worktree-{tag}.tar.gz"
    c.put(str(archive), remote_archive)

    image = _q(_image(cfg, tag))
    flag_str = " ".join(flags)
    build_dir = _q(str(cfg["build_dir"]))
    _run(
        c,
        " && ".join(
            [
                f"rm -rf {build_dir}",
                f"mkdir -p {build_dir}",
                f"tar -xzf {_q(remote_archive)} -C {build_dir}",
                f"cd {build_dir}",
                f"podman build {flag_str} -t {image} -f apps/partner-api/Dockerfile apps/partner-api",
                f"rm -rf {build_dir} {_q(remote_archive)}",
            ]
        ),
    )


@task(help={"target": "target name: ceshi", "tag": "image tag to push to ACR"})
def push_image(ctx, target="ceshi", tag=""):
    """Push a previously built image from the server to ACR."""
    cfg = _config(target)
    c = _connect(cfg)
    _run(c, f"podman push {_q(_image(cfg, tag))}")


@task(help={"target": "target name: ceshi", "tag": "image tag already present locally or in ACR"})
def deploy(ctx, target="ceshi", tag=""):
    """Blue-green deploy partner-api from an image tag."""
    tag = _validate_arg("tag", tag)
    cfg = _config(target)
    c = _connect(cfg)
    image = _image(cfg, tag)
    deploy_script = f"""
set -euo pipefail
IMAGE={_q(image)}
ENV_FILE={_q(str(cfg['env_file']))}
APP_DIR={_q(str(cfg['app_dir']))}
LOG_DIR={_q(str(cfg['log_dir']))}
DATA_DIR={_q(str(cfg['data_dir']))}
NGINX_CONF={_q(str(cfg['nginx_conf']))}
HEALTH_PATH={_q(str(cfg['health_path']))}
CONTAINER_PORT={int(cfg['container_port'])}
BLUE_PORT={int(cfg['blue_port'])}
GREEN_PORT={int(cfg['green_port'])}
MEM={_q(str(cfg['memory']))}
CPUS={_q(str(cfg['cpus']))}
test -f "$ENV_FILE"
test -f "$NGINX_CONF"
mkdir -p "$APP_DIR" "$LOG_DIR" "$DATA_DIR"
if podman ps --format '{{{{.Names}}}}' | grep -qx tracenexbiz-blue; then
  CUR=blue; CUR_PORT=$BLUE_PORT; NEXT=green; NEXT_PORT=$GREEN_PORT
elif podman ps --format '{{{{.Names}}}}' | grep -qx tracenexbiz-green; then
  CUR=green; CUR_PORT=$GREEN_PORT; NEXT=blue; NEXT_PORT=$BLUE_PORT
else
  CUR=none; CUR_PORT=0; NEXT=blue; NEXT_PORT=$BLUE_PORT
fi
echo "current=$CUR port=$CUR_PORT next=$NEXT port=$NEXT_PORT image=$IMAGE"
podman pull "$IMAGE" || podman image exists "$IMAGE"
podman rm -f "tracenexbiz-$NEXT" 2>/dev/null || true
podman run -d --name "tracenexbiz-$NEXT" \\
  --restart=unless-stopped \\
  -p "127.0.0.1:$NEXT_PORT:$CONTAINER_PORT" \\
  -v "$LOG_DIR:/app/logs:Z" \\
  -v "$DATA_DIR:/data:Z" \\
  --env-file "$ENV_FILE" \\
  --log-driver=k8s-file \\
  --log-opt max-size=100m --log-opt max-file=5 \\
  --memory="$MEM" --memory-swap="$MEM" \\
  --cpus="$CPUS" \\
  "$IMAGE"
for i in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:$NEXT_PORT$HEALTH_PATH" >/dev/null; then
    echo "tracenexbiz-$NEXT healthy"
    break
  fi
  sleep 1
  if [ "$i" -eq 60 ]; then
    podman logs --tail 120 "tracenexbiz-$NEXT" || true
    exit 1
  fi
done
if [ "$CUR" != "none" ]; then
  sed -i -E "s|(server[[:space:]]+127\\.0\\.0\\.1:)$CUR_PORT([[:space:];])|\\1$NEXT_PORT\\2|" "$NGINX_CONF"
  nginx -t
  systemctl reload nginx
else
  echo "first deploy: skipping nginx upstream switch"
fi
if [ "$CUR" != "none" ]; then
  sleep 15
  podman stop -t 30 "tracenexbiz-$CUR" || true
fi
echo "$(date -u +%F-%T) deploy $IMAGE: $CUR -> $NEXT (port $NEXT_PORT)" >> "$APP_DIR/deploy-log.md"
"""
    _run(c, f"flock {_q(DEPLOY_LOCK)} -c {_q(deploy_script)}")


@task(
    help={
        "target": "target name: ceshi",
        "tag": "image tag to build/push/deploy, e.g. 1.2.1-tracenex",
        "ref": "git ref to checkout; defaults to the same value as tag",
        "skip_build": "skip build step",
        "skip_push": "skip ACR push step",
    }
)
def release(ctx, target="ceshi", tag="", ref="", skip_build=False, skip_push=False):
    """Full release: checkout, build, push to ACR, blue-green deploy, health check."""
    tag = _validate_arg("tag", tag)
    ref = ref or tag
    if not skip_build:
        build(ctx, target=target, tag=tag, ref=ref)
    if not skip_push:
        push_image(ctx, target=target, tag=tag)
    deploy(ctx, target=target, tag=tag)
    health(ctx, target=target)


@task(
    help={
        "target": "target name: ceshi",
        "tag": "image tag to build/push/deploy, e.g. 1.2.1-tracenex",
        "skip_build": "skip build step",
        "skip_push": "skip ACR push step",
    }
)
def release_local(ctx, target="ceshi", tag="", skip_build=False, skip_push=False):
    """Full release from local worktree: build, push to ACR, blue-green deploy, health check."""
    tag = _validate_arg("tag", tag)
    if not skip_build:
        build_local(ctx, target=target, tag=tag)
    if not skip_push:
        push_image(ctx, target=target, tag=tag)
    deploy(ctx, target=target, tag=tag)
    health(ctx, target=target)


@task(help={"target": "target name: ceshi", "tag": "older image tag to deploy"})
def rollback(ctx, target="ceshi", tag=""):
    """Rollback by deploying an older image tag."""
    deploy(ctx, target=target, tag=tag)
    health(ctx, target=target)


@task(help={"target": "target name: ceshi"})
def status(ctx, target="ceshi"):
    """Show git ref, containers, nginx status, and disk usage."""
    cfg = _config(target)
    c = _connect(cfg)
    _run(c, f"cd {_q(str(cfg['src_dir']))} && git log -1 --oneline", warn=True)
    _run(c, "podman ps -a --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}' | grep -E 'NAMES|tracenexbiz' || true", warn=True)
    _run(c, "systemctl is-active nginx || true", warn=True)
    _run(c, f"df -h {_q(str(cfg['app_dir']))} /var/log/nginx 2>/dev/null | awk 'NR>1'", warn=True)


@task(help={"target": "target name: ceshi"})
def health(ctx, target="ceshi"):
    """Check the active local blue/green partner-api health endpoint."""
    cfg = _config(target)
    c = _connect(cfg)
    health_path = _q(str(cfg["health_path"]))
    _run(
        c,
        f"curl -fsS http://127.0.0.1:{int(cfg['blue_port'])}{health_path} || "
        f"curl -fsS http://127.0.0.1:{int(cfg['green_port'])}{health_path}",
        warn=True,
    )


@task(help={"target": "target name: ceshi", "tail": "number of container log lines"})
def logs(ctx, target="ceshi", tail=100):
    """Show logs from the active TraceNexBiz blue/green container."""
    cfg = _config(target)
    c = _connect(cfg)
    tail = int(tail)
    _run(
        c,
        "ACTIVE=$(podman ps --format '{{.Names}}' | grep -E '^tracenexbiz-(blue|green)$' | head -1); "
        f"test -n \"$ACTIVE\" && podman logs --tail {tail} \"$ACTIVE\"",
        warn=True,
    )
