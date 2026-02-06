{
  description = "Alcatraz - Container isolation for AI coding assistants";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
        };

        # Single source of truth for supported systems
        supportedSystems = {
          "x86_64-linux" = {
            target = "linux:amd64";
            bin = "alca-linux-amd64";
          };
          "aarch64-linux" = {
            target = "linux:arm64";
            bin = "alca-linux-arm64";
          };
          "x86_64-darwin" = {
            target = "darwin:amd64";
            bin = "alca-darwin-amd64";
          };
          "aarch64-darwin" = {
            target = "darwin:arm64";
            bin = "alca-darwin-arm64";
          };
        };

        # Derive values from supportedSystems
        systemInfo = supportedSystems.${system} or (throw "Unsupported system: ${system}");
        makeTarget = systemInfo.target;
        binaryName = systemInfo.bin;

        alca = pkgs.buildGoModule {
          pname = "alca";
          version = "0.1.0";

          src = ./.;

          # First build with empty hash to get the correct one
          vendorHash = "sha256-BqcdENlkvx6l0IBlHi7EZhDnTj9om0sHJbgvtPMViDk=";

          # Disable default build, use Makefile instead
          buildPhase = ''
            runHook preBuild
            make build:${makeTarget}
            runHook postBuild
          '';

          # Install binary and generate man pages
          installPhase = ''
            runHook preInstall
            mkdir -p $out/bin
            cp out/bin/${binaryName} $out/bin/alca

            # Build gendocs for man page generation
            go build -o gendocs ./cmd/gendocs
            ./gendocs man
            mkdir -p $out/share/man/man1
            mv out/man/*.1 $out/share/man/man1/
            runHook postInstall
          '';

          meta = with pkgs.lib; {
            description = "Container isolation for AI coding assistants";
            homepage = "https://github.com/bolasblack/alcatraz";
            license = licenses.mit;
            maintainers = [ ];
            mainProgram = "alca";
            platforms = builtins.attrNames supportedSystems;
          };
        };
      in
      {
        packages = {
          default = alca;
          alca = alca;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            bashInteractive
            ncurses
            coreutils
            findutils
            iputils
            iproute2
            traceroute
            tcpdump
            gnugrep
            python3Minimal
            nodejs-slim
            tmux
            mise
          ];

          shellHook = ''
            echo "Alcatraz development environment"

            export LANG=C.UTF-8
            export LC_ALL=C.UTF-8
            export PATH="/root/.local/share/mise/shims:$PATH"
            [ -x /extra-scripts/source.sh ] && source /extra-scripts/source.sh

            if [ ! -f /.inited ]; then
              mise trust
              mise install

              export BIN_PATH__BASH="${pkgs.bashInteractive}/bin/bash"
              [ -x /extra-scripts/init.sh ] && /extra-scripts/init.sh

              touch /.inited
            fi
          '';
        };
      }
    );
}
