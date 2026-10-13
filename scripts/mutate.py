#!/usr/bin/env python3
"""Break the code on purpose, and require the suite to notice.

A gate nobody has seen fail is a gate nobody can trust. Each mutation below is a plausible mistake
somebody could make on a Friday, and each one has to turn the suite red. The file is restored from a
copy and the copy is compared afterwards, because "the tests pass again" is not the same as "the
file is back".

The mutations live in mutations.tsv rather than in shell quoting, because a sed expression that
contains the delimiter of the loop that runs it is a mutation that never applies - and a mutation
that never applies is a gate that is blind in a way nobody notices.
"""
import pathlib
import shutil
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
TABLE = ROOT / "scripts" / "mutations.tsv"


def failures() -> int:
    out = subprocess.run(["go", "test", "./...", "-count=1"], cwd=ROOT, capture_output=True, text=True)
    return sum(1 for line in (out.stdout + out.stderr).splitlines()
               if line.startswith("--- FAIL") or line.strip().startswith("--- FAIL"))


def main() -> int:
    rows = [r for r in TABLE.read_text().splitlines() if r.strip() and not r.startswith("#")]
    if not rows:
        print("no mutations: the table is empty, which is not the same as a codebase that cannot break")
        return 1
    for row in rows:
        target, old, new, what = row.split("\t")
        old, new = old.replace("\\t", "\t"), new.replace("\\t", "\t")
        path = ROOT / target
        with tempfile.NamedTemporaryFile(delete=False) as tmp:
            shutil.copyfile(path, tmp.name)
        text = path.read_text()
        if text.count(old) != 1:
            print(f"mutation does not apply exactly once ({text.count(old)}x): {what}")
            return 1
        path.write_text(text.replace(old, new))
        n = failures()
        shutil.copyfile(tmp.name, path)
        if path.read_text() != text:
            print(f"restore failed: {what}")
            return 1
        if n == 0:
            print(f"GATE IS BLIND: {what} - the suite passed with the mistake in place")
            return 1
        print(f"caught ({n} failing tests): {what}")
    print(f"{len(rows)}/{len(rows)} mutations caught")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
