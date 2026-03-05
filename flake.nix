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

        alcaVersion = "0.2.1";

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

        # Pin mutagen to 0.18.1+ (nixpkgs has 0.18.0 which has a protocol handshake bug).
        # Must override postInstall because the original uses `rec {}` — the agents
        # store path is baked into postInstall at definition time, not at override time.
        mutagen =
          (pkgs.mutagen.override {
            buildGoModule =
              args:
              pkgs.buildGoModule (
                args
                // {
                  vendorHash = "sha256-RVVUeNfp/HWd3/5uCyaDGw6bXFJvfomhu//829jO+qE=";
                }
              );
          }).overrideAttrs
            (old: rec {
              version = "0.18.1";
              src = pkgs.fetchFromGitHub {
                owner = "mutagen-io";
                repo = "mutagen";
                rev = "v${version}";
                hash = "sha256-eT1B2ifs1BA2wcVyz9C9F8YoSbGcpGghu5Z3UrjfBOc=";
              };
              agents = pkgs.fetchzip {
                name = "mutagen-agents-${version}";
                url = "https://github.com/mutagen-io/mutagen/releases/download/v${version}/mutagen_linux_amd64_v${version}.tar.gz";
                stripRoot = false;
                postFetch = ''
                  rm $out/mutagen
                '';
                hash = "sha256-ltObD3MCSYE7IJaEDyB35CqmtUKintsaD0sMQdFAfYY=";
              };
              postInstall = ''
                install -d $out/libexec
                ln -s ${agents}/mutagen-agents.tar.gz $out/libexec/

                $out/bin/mutagen generate \
                  --bash-completion-script mutagen.bash \
                  --fish-completion-script mutagen.fish \
                  --zsh-completion-script mutagen.zsh

                installShellCompletion \
                  --cmd mutagen \
                  --bash mutagen.bash \
                  --fish mutagen.fish \
                  --zsh mutagen.zsh
              '';
            });

        alca = pkgs.buildGoModule {
          pname = "alca";
          version = alcaVersion;

          src = ./.;

          # First build with empty hash to get the correct one
          vendorHash = "sha256-BqcdENlkvx6l0IBlHi7EZhDnTj9om0sHJbgvtPMViDk=";

          # Disable default build, use Makefile instead
          buildPhase = ''
            runHook preBuild
            make build:${makeTarget}
            runHook postBuild
          '';

          # Install binary, man pages, and shell completions
          installPhase = ''
            runHook preInstall
            mkdir -p $out/bin
            cp out/bin/${binaryName} $out/bin/alca

            make docs-man
            mkdir -p $out/share/man/man1
            cp out/man/*.1 $out/share/man/man1/

            make docs-completions
            mkdir -p $out/share/bash-completion/completions
            mkdir -p $out/share/zsh/site-functions
            mkdir -p $out/share/fish/vendor_completions.d
            cp out/completions/alca.bash $out/share/bash-completion/completions/alca
            cp out/completions/alca.zsh $out/share/zsh/site-functions/_alca
            cp out/completions/alca.fish $out/share/fish/vendor_completions.d/alca.fish
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
            python312
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

        devShells.integration = pkgs.mkShell {
          buildInputs = [
            pkgs.bashInteractive
            pkgs.wget
            pkgs.python312
            mutagen
            alca
          ];

          shellHook = ''
            export ALCA_BIN="${alca}/bin/alca"
          '';
        };
      }
    );
}
