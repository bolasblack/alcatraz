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

        alca = pkgs.buildGoModule {
          pname = "alca";
          version = "0.1.0";

          src = ./.;

          # First build with empty hash to get the correct one
          vendorHash = "sha256-BqcdENlkvx6l0IBlHi7EZhDnTj9om0sHJbgvtPMViDk=";

          # Build binaries
          subPackages = [
            "cmd/alca"
            "cmd/gendocs"
          ];

          # Generate man pages after build
          postInstall = ''
            $out/bin/gendocs man
            mkdir -p $out/share/man/man1
            mv out/man/*.1 $out/share/man/man1/
            rm -rf out
            rm $out/bin/gendocs
          '';

          meta = with pkgs.lib; {
            description = "Container isolation for AI coding assistants";
            homepage = "https://github.com/bolasblack/alcatraz";
            license = licenses.mit;
            maintainers = [ ];
            mainProgram = "alca";
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
