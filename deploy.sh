#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
target=${1:?provide existing production directory}
target=$(realpath "$target")
[[ "$target" == /home/fun/projects/chronolog ]] || { echo 'Unexpected production directory'; exit 1; }
[[ -f "$target/chronolog" && -d "$target/data" && -d "$target/config" ]] || { echo 'Production layout mismatch'; exit 1; }
exec 9>"$target/.deploy.lock"
flock -n 9 || { echo 'Another deployment is running'; exit 1; }
cd "$(dirname "$0")"
sha256sum --check SHA256SUMS
gzip -dc chronolog-linux-amd64.gz > "$target/chronolog.next"
chmod 700 "$target/chronolog.next"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
backup="$target/backups/$stamp"
mkdir -p "$backup"
unit=$(systemctl --user show chronolog.service -p FragmentPath --value)
[[ -f "$unit" ]] || { echo 'Service unit not found'; exit 1; }
cp "$unit" "$backup/chronolog.service"
switched=0
stopped=0
rollback() {
  result=$?
  trap - ERR
  if (( switched )); then cp "$backup/chronolog" "$target/chronolog.rollback"; chmod 700 "$target/chronolog.rollback"; mv "$target/chronolog.rollback" "$target/chronolog"; fi
  if (( stopped )); then systemctl --user restart chronolog.service || true; fi
  echo "Deployment failed; rollback attempted. Backup: $backup"
  exit "$result"
}
trap rollback ERR
systemctl --user stop chronolog.service
stopped=1
cp "$target/chronolog" "$backup/chronolog"
tar -czf "$backup/config-data.tar.gz" -C "$target" config data
if [[ -d "$HOME/.config/systemd/user/chronolog.service.d" ]]; then cp -a "$HOME/.config/systemd/user/chronolog.service.d" "$backup/"; fi
mv "$target/chronolog.next" "$target/chronolog"
switched=1
systemctl --user start chronolog.service
healthy=0
for attempt in $(seq 1 30); do
  if curl --fail --silent --max-time 2 http://127.0.0.1:18088/api/v1/health > "$backup/new-health.json"; then healthy=1; break; fi
  sleep 1
done
[[ "$healthy" == 1 ]]
systemctl --user is-active --quiet chronolog.service
cp release.json "$target/release.json"
cp release.json "$backup/new-release.json"
trap - ERR
echo "Deployment healthy. Backup: $backup"
cat release.json
