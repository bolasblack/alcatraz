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
        pkgs = nixpkgs.legacyPackages.${system};

        alca = pkgs.buildGoModule {
          pname = "alca";
          version = "0.1.0";

          src = ./.;

          # First build with empty hash to get the correct one
          vendorHash = "sha256-u1bQu1In9hX+1bmUMJcUc/x/ZBJnVNymCNLXUk3YwAU=";

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
            go
            gopls
            golangci-lint
            alca
          ];

          shellHook = ''
            echo "Alcatraz development environment"
            echo "Go version: $(go version)"
          '';
        };
      }
    );
}
