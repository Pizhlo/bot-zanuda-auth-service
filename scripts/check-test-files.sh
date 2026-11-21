#!/bin/bash

# Скрипт был скопирован отсюда: https://github.com/jaegertracing/jaeger/blob/main/scripts/lint/check-test-files.sh

# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Check that each test matches a source file and packages meet the coverage rules.

set -euo pipefail

NO_TEST_FILE_DIRS=""
total_pkgs=0

invalid_names=0
for dir in "$@"; do
  for test_file in "$dir"/*_test.go; do
    [[ -f "$test_file" ]] || continue
    source_file="${test_file%_test.go}.go"
    if [[ ! -f "$source_file" ]]; then
      echo "error: $test_file requires matching source file $source_file" >&2
      invalid_names=1
    fi
  done

  # Пропускаем директории с моками
  if [[ "${dir}" == */mocks ]] || [[ "${dir}" == */mocks/* ]]; then
    continue
  fi
  # Пропускаем директории с документацией
  if [[ "${dir}" == */docs ]] || [[ "${dir}" == */docs/* ]]; then
    continue
  fi
  ((total_pkgs+=1))
  mainFile=$(find "${dir}" -maxdepth 1 -name 'main.go')
  testFiles=$(find "${dir}" -maxdepth 1 -name '*_test.go')
  if [ -z "${testFiles}" ]; then
    if [ -n "${mainFile}" ]; then
      continue # single main does not require tests
    fi
    if [ -e "${dir}/.nocover" ]; then
      reason=$(cat "${dir}/.nocover")
      if [ "${reason}" == "" ]; then
        echo "error: ${dir}/.nocover must specify reason" >&2
        exit 1
      fi
      echo "Package excluded from coverage: ${dir}"
      echo "  reason: ${reason}" | sed "s/FIXME/🔴 FIXME/"
      continue
    fi
    if [ -z "${NO_TEST_FILE_DIRS}" ]; then
      NO_TEST_FILE_DIRS="${dir}"
    else
      NO_TEST_FILE_DIRS="${NO_TEST_FILE_DIRS} ${dir}"
    fi
  fi
done

if [ -n "${NO_TEST_FILE_DIRS}" ]; then
  echo "*** directories without *_test.go files:" >&2
  echo "${NO_TEST_FILE_DIRS}" | tr ' ' '\n' >&2
  echo "error: at least one *_test.go file must be in all directories with go files so that they are counted for code coverage" >&2
  echo "       if no tests are possible for a package (e.g. it only defines types), add a .nocover file explaining why" >&2
  exit 1
fi

if ((invalid_names)); then
  exit 1
fi

echo "✅ Info(check-test-files): no issues after scanning ${total_pkgs} package(s)."
