"""Restore a snapshot into a NEW project-local directory; never overwrite live data."""
import argparse
import hashlib
import json
import logging
from pathlib import Path
import shutil
import sqlite3


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("snapshot", type=Path)
    parser.add_argument("destination", type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    snapshot = args.snapshot.resolve()
    destination = args.destination.resolve()
    if not snapshot.is_relative_to(root) or not destination.is_relative_to(root):
        raise ValueError("Snapshot and destination must be inside this project")
    if destination.exists():
        raise ValueError("Destination must be a new directory; live files are never overwritten")
    manifest = json.loads((snapshot / "media.json").read_text(encoding="utf-8"))
    sources = []
    for relative, digest in manifest.items():
        target = (destination / "uploads" / relative).resolve()
        if not target.is_relative_to(destination / "uploads"):
            raise ValueError("Unsafe media path in manifest")
        if len(digest) != 64 or any(c not in "0123456789abcdef" for c in digest):
            raise ValueError("Invalid media digest")
        source = snapshot.parent / "media" / digest
        hasher = hashlib.sha256()
        with source.open("rb") as stream:
            for block in iter(lambda: stream.read(1024 * 1024), b""):
                hasher.update(block)
        if hasher.hexdigest() != digest:
            raise ValueError(f"Backup media checksum failed: {relative}")
        sources.append((source, target))
    destination.mkdir(parents=True)
    shutil.copy2(snapshot / "data.db", destination / "data.db")
    for source, target in sources:
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
    with sqlite3.connect(destination / "data.db") as db:
        if db.execute("PRAGMA integrity_check").fetchone()[0] != "ok":
            raise ValueError("Restored SQLite integrity check failed")
        # Restoring an old snapshot must never dispatch an old pending generation.
        db.execute("UPDATE batch_tasks SET status='cancelled',error='恢复快照后暂停，需人工重新选择执行' WHERE status IN ('pending','review','submitting')")
    logging.info("backup restored", extra={"destination": str(destination), "media_count": len(sources)})


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    main()
