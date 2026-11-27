#!/usr/bin/env bash
# Проверяем свои компоненты на отдельном проекте уже используемого стенда.
set -euo pipefail
cd "$(dirname "$0")/.."
testkit_dir=$(realpath "${TESTKIT_DIR:-testkit}")
work=$(mktemp -d)
export RUN_ID="components-$(cat /proc/sys/kernel/random/uuid)"
export CI=true AUTH_MODE=container TEST_MODE=container
rev=$(git rev-parse HEAD)
cleanup() {
    local code=$?
    if [[ $started == true ]]; then
        if (( code != 0 )); then make integration-logs TESTKIT_DIR="$testkit_dir" RUN_ID="$RUN_ID" || true; fi
        make integration-down TESTKIT_DIR="$testkit_dir" RUN_ID="$RUN_ID" || code=1
    fi
    rm -rf -- "$work"
    exit "$code"
}
started=false
trap cleanup EXIT
if [[ -z ${COMPONENT_AUTH_IMAGE:-} ]]; then
    # Только файлы исходников, также учитываем подготовленные, ещё не закоммиченные изменения.
    git ls-files -z -- Dockerfile go.mod go.sum cmd internal pkg docs fga migration casbin_model.conf |
      tar --null -T - -cf - | docker build --iidfile "$work/image" --build-arg VERSION="$rev" \
      --build-arg GIT_COMMIT="$rev" --build-arg BUILD_DATE="$(git show -s --format=%cI HEAD)" - >/dev/null
    COMPONENT_AUTH_IMAGE=$(cat "$work/image")
fi
export AUTH_IMAGE
AUTH_IMAGE=$(docker image inspect --format '{{.Id}}' "$COMPONENT_AUTH_IMAGE")
export CGO_ENABLED=0 GOOS=linux
go test -tags=integration -c -o "$work/component-tests" ./tests/components
go build -o "$work/space-types" ./cmd/space-types
# Меняется только вход go:embed. Go переиспользует общий результат компиляции
# и перелинковывает приложение, не создавая копий checkout и не меняя исходники.
pkg=auth-service/internal/service/enforcer
flags="-X main.Version=components -X main.GitCommit=$rev -X main.BuildDate=$(git show -s --format=%cI HEAD)"
go build -o "$work/empty" -ldflags "$flags -X $pkg.fieldPolicy=" ./cmd/app
rules=$(cat internal/service/enforcer/fields/policy.csv)
go build -o "$work/changed" -ldflags "$flags -X '$pkg.fieldPolicy=$rules
p, note, update, revision'" ./cmd/app
matcher=$(sed 's/^m = .*/m = unavailableFunction(r.field)/' internal/service/enforcer/fields/model.conf)
go build -o "$work/unavailable" -ldflags "$flags -X '$pkg.fieldModel=$matcher'" ./cmd/app
started=true
make integration-up TESTKIT_DIR="$testkit_dir" AUTH_IMAGE="$AUTH_IMAGE" RUN_ID="$RUN_ID"
container=$(docker ps -q --filter "label=com.docker.compose.project=$RUN_ID" --filter 'label=com.docker.compose.service=auth-service')
test -n "$container"
docker run --rm --network "container:$container" --user "$(id -u):$(id -g)" \
    --mount "type=bind,source=$work,target=/components,readonly" \
    --mount "type=bind,source=$PWD/fga/auth-model.fga,target=/app/fga/auth-model.fga,readonly" \
    --mount "type=bind,source=$testkit_dir/configs/auth-service/config.yaml,target=/components.yaml,readonly" \
    "$AUTH_IMAGE" /components/component-tests -test.v -test.timeout=4m
