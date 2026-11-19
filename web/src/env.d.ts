/// <reference types="vite/client" />

declare module 'vue' {
  interface GlobalDirectives {
    vField: typeof import('./directives').vField
    vStack: typeof import('./directives').vStack
  }
}
export {}
