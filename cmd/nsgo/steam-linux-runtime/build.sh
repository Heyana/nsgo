#!/bin/sh
# Rebuild the bundled Linux x64 GTK runtime with Valve's sniper SDK.
set -eu

sdk=registry.gitlab.steamos.cloud/steamrt/sniper/sdk@sha256:bb30053d3cd19d35c76ae69b2790dbaa1430448006f8240cb6d7e723f2d1940e
recipe=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)

if [ "${1:-}" != inside ]; then
    work=$(mktemp -d)
    trap 'rm -rf "$work"' EXIT
    docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" \
        -v "$recipe:/recipe:ro" -v "$work:/work" "$sdk" /bin/sh /recipe/build.sh inside
    cp "$work/amd64.tar.gz" "$recipe/amd64.tar.gz"
    exit
fi

export LC_ALL=C TZ=UTC SOURCE_DATE_EPOCH=1 CCACHE_DISABLE=1
work=/work
src="$work/build/source"
prefix="$work/build/install"
mkdir -p "$src"
while read -r checksum url; do
    archive="$work/${url##*/}"
    curl -fsSL --retry 3 "$url" -o "$archive"
    printf '%s  %s\n' "$checksum" "$archive" | sha256sum -c -
    tar -xf "$archive" -C "$src"
done < "$recipe/sources.lock"

meson="$src/meson-1.5.2/meson.py"
export PATH="$prefix/bin:$PATH"
export PKG_CONFIG_PATH="$prefix/lib/pkgconfig:$prefix/share/pkgconfig"
export LD_LIBRARY_PATH="$prefix/lib"
export PYTHONPATH="$src/packaging-24.1/src"

build() {
    name=$1
    shift
    python3 "$meson" setup "$src/$name/build" "$src/$name" \
        --prefix="$prefix" --libdir=lib --buildtype=release --wrap-mode=nodownload "$@"
    python3 "$meson" compile -C "$src/$name/build" -j 8
    python3 "$meson" install -C "$src/$name/build" --strip
}

build glib-2.80.5 -Dtests=false -Dintrospection=disabled -Ddocumentation=false \
    -Dman-pages=disabled -Dnls=disabled -Dselinux=disabled -Dlibmount=disabled -Dxattr=false
build wayland-1.22.0 -Ddocumentation=false -Dtests=false -Ddtd_validation=false
build wayland-protocols-1.34 -Dtests=false
build graphene-1.10.8 -Dintrospection=disabled -Dtests=false -Dinstalled_tests=false
build pango-1.52.2 -Dintrospection=disabled -Dlibthai=disabled
# The Cairo script interpreter reads serialized debug nodes. Exclude it
# so the runtime does not carry its separate compression dependencies.
patch -d "$src/gtk-4.14.5" -p1 < "$recipe/cairo-script.patch"
build gtk-4.14.5 -Dcairo-script=disabled -Dbuild-demos=false -Dbuild-examples=false -Dbuild-tests=false \
    -Dbuild-testsuite=false -Dmedia-gstreamer=disabled -Dintrospection=disabled \
    -Ddocumentation=false -Dvulkan=disabled -Dprint-cups=disabled -Dbroadway-backend=false \
    -Dcolord=disabled -Dtracker=disabled -Dcloudproviders=disabled

runtime="$work/runtime"
mkdir -p "$runtime/lib" "$runtime/share/glib-2.0" "$runtime/licenses"
# Install each library once, using the name that dependent libraries load.
for lib in "$prefix"/lib/*.so.*; do
    [ ! -L "$lib" ] || continue
    soname=$(readelf -d "$lib" | sed -n 's/.*(SONAME).*\[\(.*\)\]/\1/p')
    [ -n "$soname" ] || continue
    install -m644 "$lib" "$runtime/lib/$soname"
done
cp -a "$prefix/share/glib-2.0/schemas" "$runtime/share/glib-2.0"
for project in glib-2.80.5 wayland-1.22.0 wayland-protocols-1.34 graphene-1.10.8 pango-1.52.2 gtk-4.14.5; do
    for license in COPYING LICENSE LICENSE.txt; do
        if [ -f "$src/$project/$license" ]; then
            install -m644 "$src/$project/$license" "$runtime/licenses/$project-$license"
        fi
    done
done
# The two client runtimes omit some libraries present in the SDK.
# Keep their SDK versions and notices with the libraries we built.
printf 'SDK %s\nSource mirrors https://repo.steampowered.com/steamrt3/apt/ https://snapshot.debian.org/\n' \
    "$sdk" > "$runtime/licenses/sdk-packages.txt"
for entry in \
    'libepoxy0 /usr/lib/x86_64-linux-gnu/libepoxy.so.0' \
    'libffi7 /usr/lib/x86_64-linux-gnu/libffi.so.7' \
    'libtiff5 /usr/lib/x86_64-linux-gnu/libtiff.so.5' \
    'libwebp6 /usr/lib/x86_64-linux-gnu/libwebp.so.6'; do
    set -- $entry
    install -m644 "$2" "$runtime/lib/${2##*/}"
    install -m644 "/usr/share/doc/$1/copyright" "$runtime/licenses/$1-copyright"
    dpkg-query -W -f='${binary:Package} ${source:Package} ${source:Version}\n' "$1:amd64"
done >> "$runtime/licenses/sdk-packages.txt"
install -m644 "$recipe/sources.lock" "$runtime/licenses/sources.lock"
install -m644 "$recipe/cairo-script.patch" "$runtime/licenses/cairo-script.patch"
tar -C "$runtime" --sort=name --mtime=@1 --owner=0 --group=0 --numeric-owner \
    -czf "$work/amd64.tar.gz" lib share licenses
