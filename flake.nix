{
  description = "Development environment for cert-manager-webhook-bunny";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { nixpkgs, ... }:
    let
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    in
    {
      devShells = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go_1_26
              gopls
              gotools
              golangci-lint
              setup-envtest

              gnumake
              git
              kubernetes-helm
              kubectl
              docker-client
            ];

            shellHook = ''
              echo "cert-manager-webhook-bunny development shell"
              echo "Go $(go version | cut -d' ' -f3), Helm $(helm version --short)"

              ENVTEST_BIN="$(setup-envtest use "''${KUBE_VERSION:-1.36}" --bin-dir "$PWD/_test/envtest" -p path)"
              export TEST_ASSET_ETCD="$ENVTEST_BIN/etcd"
              export TEST_ASSET_KUBE_APISERVER="$ENVTEST_BIN/kube-apiserver"
              export TEST_ASSET_KUBECTL="$ENVTEST_BIN/kubectl"
            '';
          };
        }
      );

      formatter = forAllSystems (system: nixpkgs.legacyPackages.${system}.nixfmt);
    };
}
