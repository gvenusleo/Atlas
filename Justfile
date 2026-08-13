set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

# Directory the CLI binary is installed into by `just install`.
INSTALL_DIR := "$HOME/.local/bin"

default:
    @just --list

# Resolve the shared workspace lockfile without changing locked versions.
deps:
    mise exec -- flutter pub get --enforce-lockfile

# Refresh the shared workspace lockfile after dependency changes.
deps-update:
    mise exec -- flutter pub get

fmt:
    mise exec -- dart format packages apps/atlas_cli apps/atlas_flutter/lib apps/atlas_flutter/test

fmt-check:
    mise exec -- dart format --output=none --set-exit-if-changed packages apps/atlas_cli apps/atlas_flutter/lib apps/atlas_flutter/test

analyze: deps
    mise exec -- dart analyze

dart-test: deps
    @for package in packages/* apps/atlas_cli; do \
        if [ -d "$package/test" ] && find "$package/test" -name '*_test.dart' -print -quit | grep -q .; then \
            (cd "$package" && mise exec -- dart test); \
        fi; \
    done

[working-directory: 'apps/atlas_flutter']
app-test: deps
    mise exec -- flutter test --no-pub

test: dart-test app-test

ci: fmt-check analyze test

# Run the TUI with the Dart VM service enabled for debugging.
tui-debug:
    mise exec -- dart --enable-vm-service apps/atlas_cli/bin/atlas.dart

# Build the native `atlas` executable (Dart 3.13+ requires `dart build`
# for packages with build hooks; the output lands under build/bundle/).
build-cli: deps
    mise exec -- dart build cli -t apps/atlas_cli/bin/atlas.dart -o build/

# Install the built CLI binary into ~/.local/bin (override with INSTALL_DIR).
install: build-cli
    @mkdir -p {{ INSTALL_DIR }}
    @mv -f build/bundle/bin/atlas {{ INSTALL_DIR }}/atlas
    @chmod +x {{ INSTALL_DIR }}/atlas
    @echo "Installed atlas to {{ INSTALL_DIR }}/atlas"

[working-directory: 'apps/atlas_flutter']
app-run device='macos': deps
    mise exec -- flutter run --no-pub -d {{ quote(device) }}

[working-directory: 'apps/atlas_flutter']
app-build-linux: deps
    mise exec -- flutter build linux --debug --no-pub

[working-directory: 'apps/atlas_flutter']
app-build-macos: deps
    mise exec -- flutter build macos --debug --no-pub

[working-directory: 'apps/atlas_flutter']
app-build-windows: deps
    mise exec -- flutter build windows --debug --no-pub

[working-directory: 'apps/atlas_flutter']
app-build-android: deps
    mise exec -- flutter build apk --debug --no-pub

[working-directory: 'apps/atlas_flutter']
app-build-ios: deps
    mise exec -- flutter build ios --debug --no-codesign --no-pub

[working-directory: 'apps/atlas_flutter']
clean:
    mise exec -- flutter clean
