{
  description = "Polaris";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};
      lib = pkgs.lib;

      version = "0.0.0";

      bunDeps = pkgs.stdenv.mkDerivation {
        pname = "polaris-bun-deps";
        inherit version;

        src = lib.fileset.toSource {
          root = ./.;
          fileset = lib.fileset.unions [
            ./package.json
            ./bun.lock
          ];
        };

        nativeBuildInputs = [ pkgs.bun pkgs.cacert ];

        dontConfigure = true;
        dontFixup = true;

        buildPhase = ''
          export HOME=$TMPDIR
          bun install --frozen-lockfile --no-progress --ignore-scripts
        '';

        installPhase = ''
          mkdir -p $out
          cp -R node_modules $out/node_modules
        '';

        outputHashMode = "recursive";
        outputHashAlgo = "sha256";
        outputHash = "sha256-d13jpRkJ4QyE3H3iq+ppVrB4VXMzLdm9KGiVjkUlHbU=";
      };

      frontend = pkgs.stdenv.mkDerivation {
        pname = "polaris-frontend";
        inherit version;

        src = ./.;

        nativeBuildInputs = [ pkgs.bun pkgs.nodejs_22 ];

        configurePhase = ''
          runHook preConfigure
          cp -R ${bunDeps}/node_modules ./node_modules
          chmod -R u+w node_modules
          patchShebangs node_modules
          runHook postConfigure
        '';

        buildPhase = ''
          runHook preBuild
          bun run vite:build
          runHook postBuild
        '';

        installPhase = ''
          runHook preInstall
          mkdir -p $out
          cp -R frontend/dist $out/dist
          runHook postInstall
        '';
      };
      desktopItem = pkgs.makeDesktopItem {
        name = "polaris";
        desktopName = "Polaris";
        exec = "polaris";
        icon = "polaris";
        comment = "Polaris";
        categories = [ "Development" ];
        startupWMClass = "Polaris";
      };
    in
    {
      packages.${system}.default = pkgs.buildGoModule {
        pname = "polaris";
        inherit version;

        src = ./.;

        vendorHash = "sha256-jKMO1SmqRjoBfh7fQXeYx0P15iaZRfgHxgqX9ZBnkPM=";

        subPackages = [ "." ];

        tags = [ "desktop" "production" "webkit2_41" ];

        ldflags = [ "-s" "-w" ];

        nativeBuildInputs = with pkgs; [ pkg-config wrapGAppsHook3 copyDesktopItems ];
        buildInputs = with pkgs; [ gtk3 webkitgtk_4_1 nss ];

        desktopItems = [ desktopItem ];

        preBuild = ''
          mkdir -p frontend/dist
          cp -R ${frontend}/dist/. frontend/dist/
        '';

        postInstall = ''
          for size in 16 24 32 48 64 128 256 512; do
            install -Dm644 build/appicon.png \
              $out/share/icons/hicolor/''${size}x''${size}/apps/polaris.png
          done
        '';

        meta = {
          description = "Polaris";
          mainProgram = "polaris";
          platforms = [ "x86_64-linux" ];
        };
      };

      devShells.${system}.default = pkgs.mkShell {
        packages = with pkgs; [
          bun
          nodejs_22

          go
          gopls
          delve
          gotools

          wails
          pkg-config
          gtk3
          webkitgtk_4_1
          nss
          wl-clipboard
        ];
      };
    };
}
