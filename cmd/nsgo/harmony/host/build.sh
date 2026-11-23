#!/bin/sh
# Rebuilds this directory's host skeleton from ../templates: the ArkTS host, compiled by hvigor. These are the parts NSGo cannot build with Go, so they are committed. Needs hvigorw 6.26.x on PATH and OHOS_BASE_SDK_HOME set to an OpenHarmony SDK with API 26 (26.0.0), matching the templates' compile and target SDK versions. The working toolchain is the 7.0-Release x86_64 SDK from https://repo.huaweicloud.com/openharmony/os/7.0-Release/ and @ohos/hvigor@6.26.20 with @ohos/hvigor-ohos-plugin@6.26.20. hvigor reads the SDK by its full API directory, so the extracted components belong in "$OHOS_BASE_SDK_HOME/26.0.0/". On Apple silicon, use an x86_64 Linux builder or install Rosetta for x86_64 SDK tools.
#
# The bundle name is the first argument, a fixed-length placeholder by
# default. The compiled bytecode names each module <bundle>/<module>/<path>;
# harmony.Package replaces this placeholder byte for byte with an equally
# long name for the app, so one host serves every app. A build machine with
# the SDK does not use this host: harmony.CompileHost builds one for the
# app's own bundle from these templates. Keep this in step with hostBundle
# in ../harmony.go (docs/architecture/harmonyos.md).
set -eu
if [ $# -gt 0 ]; then
	bundle=$1
else
	bundle=com.nsgo.host.placeholder000000000000000000000000000000000000000
fi
dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
project="$work/templates"
cp -R "$dir/../templates" "$project"
# Hash the snapshot before replacing any of its placeholders. Match the
# files go:embed includes and the order TestCommittedHostMatchesTemplates uses.
(
	cd "$work"
	find templates -type f ! -path '*/.*' ! -path '*/_*' > files
	LC_ALL=C sort files > sorted
	while IFS= read -r source; do
		printf '%s\000' "$source"
		cat "$source"
		printf '\000'
	done < sorted > template-inputs
	shasum -a 256 template-inputs > digest
	cut -d ' ' -f 1 digest > host-templates.sha256
)
find "$project" -type f \( -name '*.json5' -o -name '*.json' -o -name '*.ts' -o -name '*.ets' \) \
	-exec perl -pi -e "s/\{\{name\}\}/NSGo/g; s/\{\{bundle\}\}/$bundle/g; s/\{\{version\}\}/0.0.0/g; s/\{\{code\}\}/1/g" {} +
mkdir -p "$project/entry/src/main/resources/rawfile/web"
printf '%s\n' '<!doctype html>' > "$project/entry/src/main/resources/rawfile/web/index.html"
( cd "$project" && hvigorw --mode module -p product=default -p module=entry@default -p buildMode=debug assembleHap --no-daemon >/dev/null )
hap="$project/entry/build/default/outputs/default/entry-default-unsigned.hap"
unzip -q "$hap" -d "$work/host"
test -s "$work/host/ets/modules.abc"
rm -f "$work/host/ets/sourceMaps.map"
rm -rf "$work/host/resources/rawfile"
rm -f "$dir/../host-templates.sha256"
rm -rf "$dir/ets" "$dir/resources" "$dir/resources.index" "$dir/pkgSdkInfo.json" "$dir/rpcid.sc" "$dir/module.json" "$dir/pack.info"
cp -R "$work/host/." "$dir/"
cp "$work/host-templates.sha256" "$dir/../host-templates.sha256"
echo "wrote $dir for $bundle"
