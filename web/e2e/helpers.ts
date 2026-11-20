// Shared fixtures for the end-to-end tests.
//
// `api` is an API client signed in as the administrator (the session cookie
// from global-setup plus the CSRF token from GET /auth/me) for setting up
// data where the UI is not what a test is about. `project` is a fresh,
// empty project per test, so specs never share data.
import { test as base, expect, type APIRequestContext, type Page } from '@playwright/test'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const ADMIN = { email: 'admin@example.com', password: 'Change-me-1234' }
export const STORAGE_STATE = resolve(dirname(fileURLToPath(import.meta.url)), '.auth/admin.json')

export interface Project {
  id: string
  key: string
  name: string
}

export class Api {
  constructor(
    readonly request: APIRequestContext,
    private csrf: string,
  ) {}

  static async create(request: APIRequestContext): Promise<Api> {
    const res = await request.get('/api/v1/auth/me')
    expect(res.ok(), 'signed in as the administrator').toBeTruthy()
    return new Api(request, (await res.json()).csrf_token)
  }

  async call<T = any>(method: string, path: string, data?: unknown): Promise<T> {
    const res = await this.request.fetch(`/api/v1${path}`, {
      method,
      data,
      headers: method === 'GET' ? {} : { 'X-CSRF-Token': this.csrf },
    })
    if (!res.ok()) throw new Error(`${method} ${path}: HTTP ${res.status()} ${(await res.text()).slice(0, 300)}`)
    const text = await res.text()
    return (text ? JSON.parse(text) : null) as T
  }
  get<T = any>(path: string) {
    return this.call<T>('GET', path)
  }
  post<T = any>(path: string, data?: unknown) {
    return this.call<T>('POST', path, data ?? {})
  }
  patch<T = any>(path: string, data: unknown) {
    return this.call<T>('PATCH', path, data)
  }
  put<T = any>(path: string, data: unknown) {
    return this.call<T>('PUT', path, data)
  }
  del<T = any>(path: string, data?: unknown) {
    return this.call<T>('DELETE', path, data)
  }
}

/** list responses are either arrays or {items: [...]} */
export const items = <T = any>(x: unknown): T[] => (Array.isArray(x) ? x : ((x as { items?: T[] })?.items ?? []))

let seq = 0
/** A project key unique within this test run (2–16 of A–Z0–9). */
export function uniqueKey(prefix: string): string {
  const n = (Date.now() % 1e6).toString(36).toUpperCase() + (process.pid % 1296).toString(36).toUpperCase() + (seq++).toString(36).toUpperCase()
  return (prefix + n).replace(/[^A-Z0-9]/g, '').slice(0, 16)
}

export const test = base.extend<{ api: Api; project: Project }>({
  // its own request context: independent of the browser's session, so it
  // also works in tests that start signed out or sign in as someone else
  api: async ({ playwright, baseURL }, use) => {
    const request = await playwright.request.newContext({ baseURL, storageState: STORAGE_STATE })
    await use(await Api.create(request))
    await request.dispose()
  },
  project: async ({ api }, use, info) => {
    const key = uniqueKey('E' + info.titlePath[0].replace(/\W/g, '').slice(0, 3).toUpperCase())
    const p = await api.post<Project>('/projects', { key, name: `E2E ${info.title}`.slice(0, 80), description: 'created by the end-to-end tests' })
    await use(p)
  },
})
export { expect }

/** Waits until a page has loaded its data (no PrimeVue loading mask or busy region). */
export async function settled(page: Page) {
  await page.waitForLoadState('networkidle')
  await expect(page.locator('.p-datatable-mask, [aria-busy="true"]')).toHaveCount(0)
}

/**
 * Opens the user menu in the top bar. Uses the keyboard: the sign-in notice
 * toast (top right) can cover the button for a few seconds after signing in.
 */
export async function openUserMenu(page: Page) {
  await page.getByRole('button', { name: /User menu|Användarmeny/ }).press('Enter')
  await expect(page.getByRole('menu')).toBeVisible()
}
