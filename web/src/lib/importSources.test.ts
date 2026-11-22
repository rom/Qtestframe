import { describe, expect, it } from 'vitest'
import { externalImportBody, importSourcesDirty, importSourcesForm, importSourcesPayload, sourceConfigured, splitTypes, validateImportSources } from './importSources'
import { issueKeyExample, normalizeIssueKey, rtBase, trackerForm, trackerName, trackerPayload, validateTracker, validIssueKey } from './defectTracker'
import type { ImportSources } from './types'

const saved: ImportSources = {
  jira: { url: 'https://jira.example.test', user: 'qa@example.test', jql: 'project = SHOP', has_token: true },
  azure_devops: { url: 'https://dev.azure.com/acme', project: 'Shop', area_path: 'Shop\\Web', work_item_types: 'User Story', has_token: false },
  jira_from_tracker: false,
}

describe('import sources', () => {
  it('builds the form and the payload', () => {
    const f = importSourcesForm(saved)
    expect(f.jiraUrl).toBe('https://jira.example.test')
    expect(importSourcesDirty(f, saved)).toBe(false)
    f.azureToken = 'typed'
    expect(importSourcesDirty(f, saved)).toBe(true)
    const body = importSourcesPayload({ ...f, jiraUrl: 'https://jira.example.test/' }, { azure: true })
    expect(body).toEqual({
      jira: { url: 'https://jira.example.test', user: 'qa@example.test', jql: 'project = SHOP' },
      azure_devops: { url: 'https://dev.azure.com/acme', project: 'Shop', area_path: 'Shop\\Web', work_item_types: 'User Story', token: 'typed' },
      clear_tokens: { jira: false, azure_devops: true },
    })
    expect(importSourcesForm(null).azureUrl).toBe('')
  })
  it('validates like the server', () => {
    const f = importSourcesForm(null)
    expect(validateImportSources(f)).toEqual({})
    expect(validateImportSources({ ...f, jiraUrl: 'ftp://x' })).toEqual({ jiraUrl: 'jiraUrl' })
    expect(validateImportSources({ ...f, azureUrl: 'https://dev.azure.com/acme' })).toEqual({ azureProject: 'azureProject' })
    expect(validateImportSources({ ...f, azureUrl: 'https://dev.azure.com/acme', azureProject: 'a/b' })).toEqual({ azureProject: 'azureProject' })
    expect(validateImportSources({ ...f, azureProject: 'Shop' })).toEqual({ azureUrl: 'azureUrlRequired' })
    expect(validateImportSources({ ...f, azureUrl: 'nope', azureProject: 'Shop' })).toEqual({ azureUrl: 'azureUrl' })
  })
  it('knows which sources can be used', () => {
    expect(sourceConfigured(saved, 'jira')).toBe(true)
    expect(sourceConfigured(saved, 'azure-devops')).toBe(true)
    expect(sourceConfigured({ ...saved, jira: { ...saved.jira, url: '' } }, 'jira')).toBe(false)
    expect(sourceConfigured({ ...saved, jira: { ...saved.jira, url: '' }, jira_from_tracker: true }, 'jira')).toBe(true)
    expect(sourceConfigured(null, 'jira')).toBe(false)
    expect(sourceConfigured({ ...saved, azure_devops: { ...saved.azure_devops, project: '' } }, 'azure-devops')).toBe(false)
  })
  it('builds the import requests', () => {
    expect(splitTypes(' User Story, ,Requirement ')).toEqual(['User Story', 'Requirement'])
    expect(externalImportBody('jira', { query: ' project = SHOP ', relations: true }, { folderId: 'f1' })).toEqual({ relations: true, query: 'project = SHOP', folder_id: 'f1' })
    expect(externalImportBody('azure-devops', { query: '', areaPath: 'Shop', types: 'Bug', relations: false }, { mapping: { Title: 'title' } })).toEqual({
      relations: false,
      area_path: 'Shop',
      work_item_types: ['Bug'],
      mapping: { Title: 'title' },
    })
    expect(externalImportBody('azure-devops', { query: '', relations: true }, {})).toEqual({ relations: true, area_path: '', work_item_types: [] })
    expect(externalImportBody('jira', { query: 'x', relations: true }, {}, { overwrite: true, create: false, excludedRefs: ['SHOP-2', 'SHOP-1'] })).toEqual({
      relations: true,
      query: 'x',
      overwrite_conflicts: true,
      create: false,
      exclude_refs: ['SHOP-1', 'SHOP-2'],
    })
  })
})

describe('Request Tracker', () => {
  it('checks and sends the settings', () => {
    const f = { ...trackerForm(null), type: 'rt', url: 'https://rt.example.test/REST/2.0/', queue: 'QA', labels_field: 'Tags', labels: 'qtf' }
    expect(validateTracker(f)).toEqual({})
    expect(validateTracker({ ...f, url: 'rt.example.test', queue: '' })).toEqual({ url: 'rtUrl', queue: 'queue' })
    expect(validateTracker({ ...f, queue: 'a/b' })).toEqual({ queue: 'queue' })
    expect(trackerPayload(f)).toEqual({ type: 'rt', url: 'https://rt.example.test', queue: 'QA', labels_field: 'Tags', labels: 'qtf', requirement_labels: '', comment_on_pass: false })
    expect(trackerForm({ type: 'rt', queue: 'QA', labels_field: 'CF' }).labels_field).toBe('CF')
    expect(rtBase(' https://rt.example.test/ ')).toBe('https://rt.example.test')
  })
  it('knows ticket keys', () => {
    const t = { type: 'rt' }
    expect(trackerName('rt')).toBe('RT')
    expect(issueKeyExample(t)).toBe('#123')
    for (const k of ['#12', '12', 'RT#12', 'rt 12']) expect(validIssueKey(t, k)).toBe(true)
    for (const k of ['#0', 'SHOP-1', '#', 'x12']) expect(validIssueKey(t, k)).toBe(false)
    expect(normalizeIssueKey(t, 'RT#012')).toBe('#12')
    expect(normalizeIssueKey(t, 'abc')).toBe('abc')
  })
})
