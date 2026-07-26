{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  name = "energy-tycoon-dev";

  buildInputs = with pkgs; [
    # Go toolchain
    go
    gcc
    pkg-config

    # GLFW (required for Ebiten)
    glfw

    # X11 libraries for Ebiten window management
    xorg.libX11
    xorg.libXcursor
    xorg.libXrandr
    xorg.libXinerama
    xorg.libXi
    xorg.libXxf86vm

    # OpenGL libraries for Ebiten rendering
    libGL
    libglvnd

    # Sparse linear algebra (SuperLU for power flow solver)
    superlu

    # Development tools
    git
    git-lfs
    gnumake
    zsh
    direnv
  ];

  # Set library paths for runtime linking
  LD_LIBRARY_PATH = with pkgs; lib.makeLibraryPath [
    libGL
    libglvnd
    glfw
    xorg.libX11
    xorg.libXcursor
    xorg.libXrandr
    xorg.libXinerama
    xorg.libXi
    xorg.libXxf86vm
    superlu
  ] + ":/run/opengl-driver/lib";

  shellHook = ''
    # Set CGO flags for Ebiten/GLFW/SuperLU
    export CGO_ENABLED=1
    export CGO_CFLAGS="-I${pkgs.glfw}/include -I${pkgs.superlu}/include"
    export CGO_LDFLAGS="-L${pkgs.glfw}/lib -L${pkgs.superlu}/lib -lsuperlu"
    export PKG_CONFIG_PATH="${pkgs.glfw}/lib/pkgconfig:$PKG_CONFIG_PATH"

    # nix-shell forces SHELL=bash; restore zsh so children (Cursor terminals,
    # `cursor` launched from this shell, etc.) load ~/.zshrc + oh-my-zsh.
    export SHELL="${pkgs.zsh}/bin/zsh"

    # nix-shell points TMPDIR at /tmp/nix-shell-<pid>-* and deletes it when that
    # shell exits. Cursor (and its terminals) keep the stale path, so `go run`
    # fails with: creating work dir: stat /tmp/nix-shell-...: no such file.
    export TMPDIR="''${XDG_RUNTIME_DIR:-/tmp}"
    export TMP="$TMPDIR"
    export TEMP="$TMPDIR"
    export TEMPDIR="$TMPDIR"

    # Ensure LFS filters are installed for this clone (idempotent).
    git lfs install --local >/dev/null 2>&1 || true

    # Only show welcome message in interactive shells
    if [ -t 1 ]; then
      echo "=================================================="
      echo "  Energy Tycoon Development Environment"
      echo "=================================================="
      echo ""
      echo "Available commands:"
      echo "  go run ./game          - Run desktop game"
      echo "  go test ./...          - Run unit tests"
      echo "  go build -o game ./game"
      echo ""
      echo "Engine (local replace → ../milo):"
      echo "  ls ../milo/pkg"
      echo ""
      echo "Environment:"
      echo "  Go: $(go version | cut -d' ' -f3-4)"
      echo "  SuperLU: ${pkgs.superlu}/lib/libsuperlu.so"
      echo "  CGO_ENABLED: $CGO_ENABLED"
      echo "  CGO_CFLAGS: $CGO_CFLAGS"
      echo "  CGO_LDFLAGS: $CGO_LDFLAGS"
      echo "  SHELL: $SHELL (oh-my-zsh via ~/.zshrc)"
      echo "=================================================="
      echo ""
    fi

    # Drop into zsh for interactive `nix-shell` only (not `nix-shell --run`,
    # and not direnv's non-interactive eval). Guard avoids exec loops.
    if [[ $- == *i* && -z "$IN_NIX_SHELL_ZSH" ]]; then
      export IN_NIX_SHELL_ZSH=1
      exec "$SHELL"
    fi
  '';
}
