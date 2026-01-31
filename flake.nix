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
            export PATH="/extra-bin:$PATH"

            echo '
            set -g default-shell "${pkgs.bashInteractive}/bin/bash"
            set -ga terminal-features "*:hyperlinks"
            set -g allow-passthrough on

            # Set the base index for windows pane to 1 instead of 0
            set -g base-index 1
            setw -g pane-base-index 1

            # Set the history length
            set -g history-limit 65535

            # Make C-[ in evil-mode more quickly
            #   https://bitbucket.org/lyro/evil/issues/69/delay-between-esc-or-c-and-modeswitch
            set -s escape-time 0

            # Set the key binding to vi/emacs mode
            set-window-option -g mode-keys vi
            bind -T copy-mode-vi v send -X begin-selection
            bind -T copy-mode-vi C-V send -X begin-selection \; send -X rectangle-toggle
            bind -T copy-mode-vi y send -X copy-selection-and-cancel
            bind -T copy-mode-vi r send -X rectangle-toggle

            # make sure NIX related script will be sourced again
            setenv -g __ETC_PROFILE_NIX_SOURCED ""
            setenv -g __HM_SESS_VARS_SOURCED ""
            setenv -g __HM_ZSH_SESS_VARS_SOURCED ""
            ' > ~/.tmux.conf

            mise trust
          '';
        };
      }
    );
}
