# 3. Vue 3 with PrimeVue 4 as the client framework

Date: 2026-11-19

## Status

Accepted

## Context

The client is a data-heavy business application in the style of HP ALM:
trees, sortable and filterable tables, drawers with forms, dialogs,
drag and drop, charts. It is developed by a small team, must be strictly
typed, and must work under a CSP without `unsafe-eval` because it is
served by the hardened backend.

## Decision

* Vue 3 (Composition API, `<script setup>`) with TypeScript, Vite,
  vue-router and Pinia (`web/`).
* PrimeVue 4 with `@primevue/themes` as the component library
  (DataTable, Tree, Drawer, Dialog, MultiSelect, ...), PrimeIcons, and
  Chart.js for dashboard charts.
* Templates are compiled at build time (no runtime template compiler), so
  `script-src 'self'` suffices.
* Logic that does not need a component (API client, query composition,
  tree building, highlighting, permissions, formatting, drag and drop
  payloads) lives in plain TypeScript modules in `web/src/lib` and is unit
  tested with Vitest.

## Consequences

* A large set of ready components keeps the UI code small.
* PrimeVue injects styles at runtime, so the CSP needs
  `style-src 'self' 'unsafe-inline'` (see `contentSecurityPolicy` in
  `internal/api/server.go`).
* The bundle is larger than a hand-rolled UI; views are lazy-loaded per
  route.
* Component behaviour is covered by manual testing and API integration
  tests rather than component tests; the coverage floor applies to
  `web/src/lib` only ([ADR 0013](0013-coverage-floor-85-percent.md)).
