// Signs the administrator in once and stores the session for all tests
// (use.storageState). Also marks the intro tour as done and resets theme and
// language, so every test starts from the same English, light UI.
import { request, type FullConfig } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { dirname } from 'node:path'
import { ADMIN, STORAGE_STATE } from './helpers'

export default async function globalSetup(config: FullConfig) {
  const baseURL = config.projects[0].use.baseURL as string
  const ctx = await request.newContext({ baseURL })
  const login = await ctx.post('/api/v1/auth/login', { data: { email: ADMIN.email, password: ADMIN.password } })
  if (!login.ok()) throw new Error(`cannot sign in at ${baseURL}: HTTP ${login.status()} ${await login.text()}`)
  const me = await (await ctx.get('/api/v1/auth/me')).json()
  const prefs = await ctx.patch('/api/v1/auth/me', {
    data: { preferences: { intro_done: true, theme: null, language: 'en' } },
    headers: { 'X-CSRF-Token': me.csrf_token },
  })
  if (!prefs.ok()) throw new Error(`cannot set preferences: HTTP ${prefs.status()} ${await prefs.text()}`)
  mkdirSync(dirname(STORAGE_STATE), { recursive: true })
  await ctx.storageState({ path: STORAGE_STATE })
  await ctx.dispose()
}
