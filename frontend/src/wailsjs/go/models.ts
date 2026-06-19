// Compatibility shim: re-exposes the Wails v3 generated models under the v2
// per-Go-package namespaces the app still imports (e.g. `polaris.Agent`,
// `main.BackendStatus`). Each namespace maps to its generated module.
export * as main from '../../../bindings/github.com/KevinBonnoron/polaris/models';
export * as polaris from '../../../bindings/github.com/KevinBonnoron/polaris/internal/polaris/models';
export * as terminal from '../../../bindings/github.com/KevinBonnoron/polaris/internal/terminal/models';
export * as docker from '../../../bindings/github.com/KevinBonnoron/polaris/internal/providers/docker/models';
export * as dokploy from '../../../bindings/github.com/KevinBonnoron/polaris/internal/providers/dokploy/models';
export * as git from '../../../bindings/github.com/KevinBonnoron/polaris/internal/providers/git/models';
export * as nodejs from '../../../bindings/github.com/KevinBonnoron/polaris/internal/providers/nodejs/models';
export * as python from '../../../bindings/github.com/KevinBonnoron/polaris/internal/providers/python/models';
export * as repository from '../../../bindings/github.com/KevinBonnoron/polaris/internal/providers/repository/models';
export * as resend from '../../../bindings/github.com/KevinBonnoron/polaris/internal/providers/resend/models';
export * as sentry from '../../../bindings/github.com/KevinBonnoron/polaris/internal/providers/sentry/models';
export * as tickets from '../../../bindings/github.com/KevinBonnoron/polaris/internal/providers/tickets/models';
