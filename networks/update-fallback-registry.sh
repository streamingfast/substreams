#!/bin/bash
set -euo pipefail

# Directory of this script
cd "$(dirname "$0")"

# Find the current fallback file
old_file=$(ls fallback_TheGraphNetworkRegistry_*.json | head -n1)

# Extract current version from filename
if [[ "$old_file" =~ fallback_TheGraphNetworkRegistry_(.*)\.json ]]; then
  current_version="${BASH_REMATCH[1]}"
else
  echo "Could not determine current fallback file version."
  exit 1
fi

# Fetch the latest registry
tmp_file="TheGraphNetworksRegistry.json.tmp"
curl -sSL https://networks-registry.thegraph.com/TheGraphNetworksRegistry.json -o "$tmp_file"

# Extract version
version=$(jq -r .version "$tmp_file")
if [[ -z "$version" || "$version" == "null" ]]; then
  echo "Failed to extract version from registry JSON."
  rm -f "$tmp_file"
  exit 1
fi

# Stop if version is the same
if [[ "$version" == "$current_version" ]]; then
  echo "Already up to date (version $version)."
  rm -f "$tmp_file"
  exit 0
fi

# Compose new filename
new_file="fallback_TheGraphNetworkRegistry_${version}.json"

# Move to correct name
mv "$tmp_file" "$new_file"
echo "Downloaded and saved as $new_file"

# Delete old file if different
if [[ "$old_file" != "$new_file" ]]; then
  rm -f "$old_file"
  echo "Deleted old fallback file: $old_file"
fi

# Update go:embed line in chainconfig.go
sed -i '' "s|^//go:embed fallback_TheGraphNetworkRegistry_.*\\.json$|//go:embed $new_file|" chainconfig.go

echo "Updated go:embed in chainconfig.go to $new_file" 