#!/usr/bin/env python3
import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
import hashlib
import time
from collections import deque
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Optional


NPM_REGISTRY = "https://registry.npmjs.org"
DEFAULT_MAX_CONCURRENT = 4
NODE_MODULES = Path("node_modules")
FETCH_TIMEOUT = 300
DOWNLOAD_TIMEOUT = 600


class Package:
    def __init__(self, name: str, version: str, tarball: str = ""):
        self.name = name
        self.version = version
        self.tarball = tarball
        self.dependencies = {}
        self.installed = False


class Installer:
    def __init__(self, package_json_path: str, max_concurrent: int):
        self.package_json_path = Path(package_json_path)
        self.max_concurrent = max_concurrent
        self.packages: dict[str, Package] = {}
        self.resolved: set[str] = set()
        self.resolving: set[str] = set()
        self.pending: list[tuple[str, str]] = []
        self.node_modules = NODE_MODULES

    def run(self):
        print(f"Installing dependencies from {self.package_json_path}...")
        deps = self.read_package_json()
        if not deps:
            print("No dependencies found")
            return

        print(f"Found {len(deps)} top-level dependencies")
        self.resolve_and_install(deps)
        self.run_postinstall()
        print("Installation complete!")

    def read_package_json(self) -> dict:
        try:
            with open(self.package_json_path) as f:
                data = json.load(f)
            deps = data.get("dependencies", {})
            dev_deps = data.get("devDependencies", {})
            opt_deps = data.get("optionalDependencies", {})
            # Merge all dependency types
            all_deps = {**opt_deps, **dev_deps, **deps}
            return all_deps
        except (FileNotFoundError, json.JSONDecodeError) as e:
            print(f"Error reading package.json: {e}")
            return {}

    def resolve_and_install(self, deps: dict):
        queue = deque(deps.items())
        enqueued = set(deps.keys())

        while queue:
            batch = []
            processed = 0

            while queue and processed < self.max_concurrent:
                name, version_spec = queue.popleft()
                if name not in self.resolved:
                    batch.append((name, version_spec))
                    processed += 1

            if not batch:
                break

            with ThreadPoolExecutor(max_workers=self.max_concurrent) as executor:
                futures = {
                    executor.submit(self.resolve_package, name, version_spec): (
                        name,
                        version_spec,
                    )
                    for name, version_spec in batch
                }

                for future in as_completed(futures):
                    name, version_spec = futures[future]
                    try:
                        pkg = future.result()
                        if pkg:
                            self.install_package(pkg)
                            for dep_name, dep_ver in pkg.dependencies.items():
                                if (
                                    dep_name not in enqueued
                                    and dep_name not in self.resolved
                                ):
                                    enqueued.add(dep_name)
                                    queue.append((dep_name, dep_ver))
                        else:
                            print(f"Failed to resolve {name}@{version_spec}")
                    except Exception as e:
                        print(f"Failed to process {name}: {e}")

    def resolve_package(
        self, name: str, version_spec: str, retries=3
    ) -> Optional[Package]:
        if name in self.resolving:
            return None

        self.resolving.add(name)

        try:
            url = f"{NPM_REGISTRY}/{name}"
            result = None
            for i in range(retries):
                try:
                    result = subprocess.run(
                        ["curl", "-s", "-f", "-L", url],
                        capture_output=True,
                        text=True,
                        timeout=FETCH_TIMEOUT,
                    )
                    if result.returncode == 0:
                        break
                except subprocess.TimeoutExpired:
                    print(f"Timeout fetching {name} ({i + 1}/{retries})...")
                except Exception as e:
                    print(f"Error fetching {name} ({i + 1}/{retries}): {e}")

                if i == retries - 1:
                    print(f"Failed to fetch {name} after {retries} attempts")
                    return None
                time.sleep(1)

            if result is None or result.returncode != 0:
                return None

            metadata = json.loads(result.stdout)
            version = self.resolve_version(metadata, version_spec)

            if not version:
                print(f"No matching version for {name}@{version_spec}")
                return None

            tarball = (
                metadata.get("versions", {})
                .get(version, {})
                .get("dist", {})
                .get("tarball")
            )
            if not tarball:
                print(f"No tarball for {name}@{version}")
                return None

            v_data = metadata.get("versions", {}).get(version, {})
            deps = v_data.get("dependencies", {})
            opt_deps = v_data.get("optionalDependencies", {})

            pkg = Package(name, version, tarball)
            pkg.dependencies = {**opt_deps, **deps}
            return pkg

        except Exception as e:
            print(f"Error resolving {name}: {e}")
            return None
        finally:
            self.resolving.discard(name)

    def resolve_version(self, metadata: dict, version_spec: str) -> Optional[str]:
        versions = metadata.get("versions", {})
        if not versions:
            return None

        if version_spec == "*" or version_spec == "latest":
            return self.get_latest(versions.keys())
        elif version_spec.startswith("^"):
            base = version_spec[1:]
            return self.match_caret(versions.keys(), base)
        elif version_spec.startswith("~"):
            base = version_spec[1:]
            return self.match_tilde(versions.keys(), base)
        elif version_spec.startswith(">="):
            return self.match_range(versions.keys(), version_spec)
        elif version_spec in versions:
            return version_spec

        return self.match_caret(versions.keys(), version_spec)

    def get_latest(self, available_versions) -> Optional[str]:
        valid = [v for v in available_versions if not self.is_prerelease(v)]
        if valid:
            return sorted(valid, key=lambda x: self.version_key(x))[-1]
        if available_versions:
            return sorted(available_versions, key=lambda x: self.version_key(x))[-1]
        return None

    def is_prerelease(self, version: str) -> bool:
        return bool(
            re.search(r"[a-zA-Z]", version.split(".")[0] if "." in version else version)
        )

    def match_caret(self, available_versions, base: str) -> Optional[str]:
        base_parts = base.split(".")
        try:
            major = int(base_parts[0]) if base_parts else 0
        except ValueError:
            major = 0

        matching = []
        for v in available_versions:
            try:
                v_parts = v.split(".")
                if len(v_parts) >= 2:
                    v_major = int(v_parts[0])
                    if v_major == major:
                        matching.append(v)
            except ValueError:
                continue

        if matching:
            return sorted(matching, key=lambda x: self.version_key(x))[-1]
        return None

    def match_tilde(self, available_versions, base: str) -> Optional[str]:
        base_parts = base.split(".")
        if len(base_parts) >= 2:
            try:
                major, minor = int(base_parts[0]), int(base_parts[1])
            except ValueError:
                return None
            matching = []
            for v in available_versions:
                try:
                    v_parts = v.split(".")
                    if len(v_parts) >= 2:
                        if int(v_parts[0]) == major and int(v_parts[1]) == minor:
                            matching.append(v)
                except ValueError:
                    continue

            if matching:
                return sorted(matching, key=lambda x: self.version_key(x))[-1]
        return None

    def match_range(self, available_versions, range_spec: str) -> Optional[str]:
        m = re.match(r">=(\d+\.\d+\.\d+)", range_spec)
        if m:
            min_ver = m.group(1)
            matching = [
                v for v in available_versions if self.compare_versions(v, min_ver) >= 0
            ]
            if matching:
                return sorted(matching, key=lambda x: self.version_key(x))[-1]
        return None

    def version_key(self, v: str) -> list:
        parts = []
        for p in v.split("."):
            num = re.split(r"[-+]", p)[0]
            try:
                parts.append(int(num))
            except ValueError:
                parts.append(0)
        return parts

    def compare_versions(self, v1: str, v2: str) -> int:
        parts1 = self.version_key(v1)
        parts2 = self.version_key(v2)

        for i in range(max(len(parts1), len(parts2))):
            p1 = parts1[i] if i < len(parts1) else 0
            p2 = parts2[i] if i < len(parts2) else 0

            if p1 > p2:
                return 1
            elif p1 < p2:
                return -1
        return 0

    def install_package(self, pkg: Package, retries=3):
        if pkg.name in self.resolved and pkg.installed:
            return

        print(f"Installing {pkg.name}@{pkg.version}")

        self.node_modules.mkdir(parents=True, exist_ok=True)

        try:
            if not pkg.tarball:
                print(f"No tarball for {pkg.name}")
                return

            with tempfile.NamedTemporaryFile(suffix=".tgz", delete=False) as tmp:
                tmp_path = tmp.name

            success = False
            for i in range(retries):
                try:
                    result = subprocess.run(
                        ["curl", "-s", "-f", "-L", "-o", tmp_path, pkg.tarball],
                        capture_output=True,
                        timeout=DOWNLOAD_TIMEOUT,
                    )
                    if result.returncode == 0:
                        success = True
                        break
                except subprocess.TimeoutExpired:
                    print(f"Timeout downloading {pkg.name} ({i + 1}/{retries})...")
                except Exception as e:
                    print(f"Error downloading {pkg.name} ({i + 1}/{retries}): {e}")

                if i < retries - 1:
                    import time

                    time.sleep(1)

            if not success:
                print(f"Failed to download {pkg.name} after {retries} attempts")
                if os.path.exists(tmp_path):
                    os.unlink(tmp_path)
                return

            pkg_dir = self.node_modules / pkg.name
            pkg_dir.mkdir(parents=True, exist_ok=True)

            subprocess.run(
                ["tar", "-xzf", tmp_path, "-C", str(pkg_dir), "--strip-components=1"],
                check=True,
            )

            if os.path.exists(tmp_path):
                os.unlink(tmp_path)

            self.create_bin_links(pkg)

            pkg.installed = True
            self.resolved.add(pkg.name)

        except Exception as e:
            print(f"Failed to install {pkg.name}: {e}")

    def create_bin_links(self, pkg: Package):
        pkg_dir = self.node_modules / pkg.name
        pkg_json_path = pkg_dir / "package.json"

        if not pkg_json_path.exists():
            return

        try:
            with open(pkg_json_path) as f:
                pkg_data = json.load(f)

            bin_entry = pkg_data.get("bin")
            if not bin_entry:
                return

            bin_dir = self.node_modules / ".bin"
            bin_dir.mkdir(parents=True, exist_ok=True)

            if isinstance(bin_entry, dict):
                for name, path in bin_entry.items():
                    self.create_symlink(bin_dir, name, pkg_dir / path)
            else:
                name = pkg.name.split("/")[-1]
                self.create_symlink(bin_dir, name, pkg_dir / bin_entry)

        except Exception as e:
            print(f"Error reading package.json for {pkg.name}: {e}")

    def create_symlink(self, bin_dir: Path, name: str, target: Path):
        link_path = bin_dir / name
        try:
            if link_path.exists() or link_path.is_symlink():
                link_path.unlink()

            abs_target = target.resolve()

            if abs_target.is_file():
                abs_target.chmod(0o755)
                link_path.symlink_to(abs_target)
                print(f"Linked {name} -> {abs_target}")
            else:
                # Some packages have bins that don't exist in all versions or platforms
                pass
        except Exception as e:
            print(f"Failed to create symlink {name}: {e}")

    def run_postinstall(self):
        print("Running postinstall scripts...")

        for pkg_dir in self.node_modules.iterdir():
            if not pkg_dir.is_dir() or pkg_dir.name == ".bin":
                continue

            pkg_json_path = pkg_dir / "package.json"
            if not pkg_json_path.exists():
                continue

            try:
                with open(pkg_json_path) as f:
                    pkg_data = json.load(f)

                scripts = pkg_data.get("scripts", {})
                postinstall = scripts.get("postinstall")

                if postinstall:
                    print(f"Running postinstall for {pkg_data['name']}")
                    env = os.environ.copy()
                    env["PATH"] = (
                        f"/usr/local/bin:/usr/bin:/bin:{self.node_modules.resolve()}/.bin"
                    )

                    subprocess.run(
                        ["bash", "-c", postinstall],
                        cwd=str(pkg_dir),
                        env=env,
                        capture_output=True,
                    )
            except Exception as e:
                print(f"Postinstall error for {pkg_dir.name}: {e}")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("-c", "--concurrent", type=int, default=DEFAULT_MAX_CONCURRENT)
    args = parser.parse_args()

    start_time = time.perf_counter()
    installer = Installer("package.json", args.concurrent)
    installer.run()
    end_time = time.perf_counter()
    print(f"Done in: {(end_time - start_time):.4f} seconds")


if __name__ == "__main__":
    main()
