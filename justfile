run:
    #!/usr/bin/env bash
    set -uo pipefail
    # go run spawns the compiled binary as a child, so kill the whole process
    # group rather than the tracked PIDs.
    trap 'trap - INT TERM EXIT; kill 0' INT TERM EXIT
    prefix() { while IFS= read -r line; do printf '[%s] %s\n' "$1" "$line"; done; }
    VEERY_DB=./veery.db go run ./cmd/veery 2>&1 | prefix api &
    (cd web && pnpm dev 2>&1 | prefix web) &
    wait
