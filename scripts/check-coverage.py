"""Check statement coverage from a Go module-wide coverage profile."""
import re
import sys
from functools import lru_cache
from pathlib import Path

MINIMUM_PERCENT = 80
TEST_HELPERS = (Path("pkg/audit/testaudit"), Path("internal/testfga"))
GENERATED_HEADER = re.compile(r"^// Code generated .* DO NOT EDIT\.$")


@lru_cache(maxsize=None)
def counts_towards_coverage(source):
    if source.name.endswith("_test.go") or "mocks" in source.parts[:-1]:
        return False
    if any(directory in source.parents for directory in TEST_HELPERS):
        return False
    with source.open(encoding="utf-8") as go_file:
        for line in go_file:
            if GENERATED_HEADER.fullmatch(line.rstrip("\r\n")):
                return False
            if line.startswith(("package ", "package\t")):
                break
    return True


def statement_counts(profile, module):
    # With -coverpkg, several test binaries can report the same source block.
    blocks = {}
    for line in profile:
        location, statements, executions = line.rsplit(maxsplit=2)
        source, _ = location.rsplit(":", 1)
        if not counts_towards_coverage(Path(source).relative_to(module)):
            continue
        key = (location, int(statements))
        blocks[key] = blocks.get(key, False) or int(executions) > 0
    covered = sum(size for (_, size), executed in blocks.items() if executed)
    return covered, sum(size for _, size in blocks)


def main():
    profile_path, module = sys.argv[1:]
    with Path(profile_path).open(encoding="utf-8") as profile:
        if next(profile, "").strip() not in {"mode: set", "mode: count", "mode: atomic"}:
            sys.exit("Invalid Go coverage profile")
        covered, total = statement_counts(profile, module)

    if total == 0:
        sys.exit("No production Go statements in the coverage profile")
    meets_threshold = 100 * covered >= MINIMUM_PERCENT * total
    outcome = "PASS" if meets_threshold else "FAIL"
    print(f"{outcome} — Production Go coverage: {covered}/{total} statements "
          f"({100 * covered / total:.2f}%), minimum {MINIMUM_PERCENT}%")
    return 0 if meets_threshold else 1


if __name__ == "__main__":
    sys.exit(main())
