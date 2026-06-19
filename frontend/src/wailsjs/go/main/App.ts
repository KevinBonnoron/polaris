// Compatibility shim: re-exports the Wails v3 generated App service bindings
// under the v2 import path the app still uses. Method names are identical, so
// every `import { Foo } from '@/wailsjs/go/main/App'` keeps working.
export * from '../../../../bindings/github.com/KevinBonnoron/polaris/app';
