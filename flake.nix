{
  description = "Polaris";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};
      lib = pkgs.lib;

      version = "0.0.0";

      # Wails v3 CLI, packaged so the build is self-contained (offline) and
      # `wails3 generate bindings` can run inside the derivation.
      wails3 = pkgs.buildGoModule {
        pname = "wails3";
        version = "3.0.0-alpha.102";
        src = pkgs.fetchFromGitHub {
          owner = "wailsapp";
          repo = "wails";
          rev = "e9afa58bb3cc0372123a5a4bf876f9caf77d4625";
          hash = "sha256-os19NyyBhyVpPgUDkHLWIkd8u8b6747MiGKlVV4p2Es=";
        };
        modRoot = "v3";
        subPackages = [ "cmd/wails3" ];
        vendorHash = "sha256-cFAwRPI10xk0AcjJ7aqrm65c4Wy+WQpUV/CEB2Ll2eo=";
        proxyVendor = true;
        env.GOWORK = "off";
        nativeBuildInputs = [ pkgs.pkg-config ];
        buildInputs = guiDeps;
        doCheck = false;
      };

      guiDeps = with pkgs; [
        gtk4
        webkitgtk_6_0
        libsoup_3
        glib
        cairo
        pango
        gdk-pixbuf
        graphene
        harfbuzz
      ];

      bunDeps = pkgs.stdenv.mkDerivation {
        pname = "polaris-bun-deps";
        inherit version;

        src = lib.fileset.toSource {
          root = ./.;
          fileset = lib.fileset.unions [
            ./frontend/package.json
            ./frontend/bun.lock
          ];
        };

        nativeBuildInputs = [ pkgs.bun pkgs.cacert ];

        dontConfigure = true;
        dontFixup = true;

        buildPhase = ''
          export HOME=$TMPDIR
          cd frontend
          bun install --frozen-lockfile --no-progress --ignore-scripts
        '';

        installPhase = ''
          mkdir -p $out
          cp -R node_modules $out/node_modules
        '';

        outputHashMode = "recursive";
        outputHashAlgo = "sha256";
        outputHash = "sha256-f0IsVR04C4bBUHjUnqzGo5ZJUmKDVjPAUXKMniEAms4=";
      };

      # Under GTK4 the window/taskbar icon comes from desktop integration, not
      # the embedded bytes (GTK4 dropped gtk_window_set_icon). Wails sets the GTK
      # application id to "org.wails.<lowercased name>", so the desktop file, its
      # Icon= and the WMClass must all match org.wails.polaris for the desktop
      # environment to associate the running window with the installed icon.
      desktopItem = pkgs.makeDesktopItem {
        name = "org.wails.polaris";
        desktopName = "Polaris";
        exec = "polaris";
        icon = "org.wails.polaris";
        comment = "Polaris";
        categories = [ "Development" ];
        startupWMClass = "org.wails.polaris";
      };
    in
    {
      packages.${system} = {
        wails3 = wails3;
        default = pkgs.buildGoModule {
          pname = "polaris";
          inherit version;

          src = ./.;

          vendorHash = "sha256-8tmyLK8NJGjhueOF4owc4Hthel945PDXdP0Y23jorGQ=";
          proxyVendor = true;
          env.GOWORK = "off";

          subPackages = [ "." ];

          # Without the production tag, Wails v3 compiles the dev variant
          # (devtools, dev logging). production = standalone embedded build.
          tags = [ "production" ];

          ldflags = [ "-s" "-w" ];

          nativeBuildInputs = with pkgs; [
            pkg-config
            wrapGAppsHook4
            copyDesktopItems
            wails3
            bun
            nodejs_22
          ];
          buildInputs = guiDeps;

          desktopItems = [ desktopItem ];

          # Generate the v3 TypeScript bindings, then build the frontend, before
          # the Go build embeds frontend/dist via //go:embed.
          preBuild = ''
            export HOME=$TMPDIR
            cp -R ${bunDeps}/node_modules frontend/node_modules
            chmod -R u+w frontend/node_modules
            patchShebangs frontend/node_modules
            wails3 generate bindings -ts -clean -d frontend/bindings
            (cd frontend && bun run vite:build)
          '';

          postInstall = ''
            for size in 16 24 32 48 64 128 256 512; do
              install -Dm644 build/appicon.png \
                $out/share/icons/hicolor/''${size}x''${size}/apps/org.wails.polaris.png
            done
          '';

          meta = {
            description = "Polaris";
            mainProgram = "polaris";
            platforms = [ "x86_64-linux" ];
          };
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

          wails3
          go-task
          pkg-config
          wl-clipboard
          nss
        ] ++ guiDeps;

        shellHook = ''
          export WEBKIT_DISABLE_DMABUF_RENDERER=1
        '';
      };
    };
}
