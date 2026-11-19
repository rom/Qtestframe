import { describe, expect, it } from 'vitest'
import {
  archiveParams,
  attachmentsPath,
  contentUrl,
  effectiveType,
  extension,
  fileIcon,
  formatFileSize,
  linkIcon,
  linkTarget,
  previewKind,
  progressPercent,
  sortFiles,
  tagsField,
  uploadProblem,
} from './files'

describe('formatFileSize', () => {
  it('uses binary units with one decimal below 100', () => {
    expect(formatFileSize(0)).toBe('0 B')
    expect(formatFileSize(512)).toBe('512 B')
    expect(formatFileSize(1536)).toBe('1.5 KB')
    expect(formatFileSize(2.25 * 1024 * 1024)).toBe('2.3 MB')
    expect(formatFileSize(150 * 1024 * 1024)).toBe('150 MB')
    expect(formatFileSize(1024 ** 3)).toBe('1 GB')
    expect(formatFileSize(1024 ** 5)).toBe('1,024 TB')
  })
  it('formats per locale', () => {
    expect(formatFileSize(1536, 'sv')).toBe('1,5 KB')
  })
  it('falls back for a bad locale', () => {
    expect(formatFileSize(1536, 'not a locale!!')).toBe('1.5 KB')
  })
  it('is empty for invalid sizes', () => {
    expect(formatFileSize(null)).toBe('')
    expect(formatFileSize(undefined)).toBe('')
    expect(formatFileSize(-1)).toBe('')
    expect(formatFileSize(NaN)).toBe('')
  })
})

describe('types', () => {
  it('extracts extensions', () => {
    expect(extension('a.PDF')).toBe('pdf')
    expect(extension('noext')).toBe('')
    expect(extension(null)).toBe('')
  })
  it('guesses generic types from the name', () => {
    expect(effectiveType('application/octet-stream', 'x.png')).toBe('image/png')
    expect(effectiveType('', 'x.pdf')).toBe('application/pdf')
    expect(effectiveType('text/plain; charset=utf-8', 'x')).toBe('text/plain')
    expect(effectiveType(null, 'x.unknown')).toBe('application/octet-stream')
    expect(effectiveType('application/octet-stream', 'x.bin')).toBe('application/octet-stream')
  })
})

describe('previewKind', () => {
  it('previews images, PDF and text inline', () => {
    expect(previewKind({ content_type: 'image/png', name: 'a.png' })).toBe('image')
    expect(previewKind({ content_type: 'image/webp', name: 'a' })).toBe('image')
    expect(previewKind({ content_type: 'application/pdf', name: 'a.pdf' })).toBe('pdf')
    expect(previewKind({ content_type: 'text/plain', name: 'a.txt' })).toBe('text')
  })
  it('downloads everything else, SVG included', () => {
    expect(previewKind({ content_type: 'image/svg+xml', name: 'a.svg' })).toBe('download')
    expect(previewKind({ content_type: 'text/html', name: 'a.html' })).toBe('download')
    expect(previewKind({ content_type: 'application/zip', name: 'a.zip' })).toBe('download')
    expect(previewKind(null)).toBe('download')
  })
})

describe('fileIcon', () => {
  it.each([
    ['image/png', 'a.png', 'pi pi-image'],
    ['application/pdf', 'a.pdf', 'pi pi-file-pdf'],
    ['text/csv', 'a.csv', 'pi pi-file-excel'],
    ['application/octet-stream', 'a.xlsx', 'pi pi-file-excel'],
    ['application/octet-stream', 'a.docx', 'pi pi-file-word'],
    ['application/zip', 'a.zip', 'pi pi-box'],
    ['application/x-gzip', 'a.gz', 'pi pi-box'],
    ['video/mp4', 'a.mp4', 'pi pi-video'],
    ['audio/ogg', 'a.ogg', 'pi pi-volume-up'],
    ['application/json', 'a.json', 'pi pi-file-edit'],
    ['application/x-thing', 'a', 'pi pi-file'],
  ])('%s %s', (ct, name, icon) => {
    expect(fileIcon(ct, name)).toBe(icon)
  })
})

describe('uploadProblem', () => {
  it('rejects empty, too large and wrong types', () => {
    expect(uploadProblem({ size: 0 }, 100)).toBe('empty')
    expect(uploadProblem({ size: 101 }, 100)).toBe('too_large')
    expect(uploadProblem({ size: 10, type: 'image/svg+xml', name: 'a.svg' }, 100, ['image/png'])).toBe('type')
    expect(uploadProblem({ size: 10, type: '', name: 'a.png' }, 100, ['image/png'])).toBeNull()
    expect(uploadProblem({ size: 10 ** 9 }, 0)).toBeNull()
  })
})

describe('paths and links', () => {
  it('builds URLs', () => {
    expect(contentUrl('f1')).toBe('/api/v1/files/f1/content')
    expect(contentUrl('f 1', true)).toBe('/api/v1/files/f%201/content?inline=1')
    expect(attachmentsPath('execution', 'e1')).toBe('/executions/e1/attachments')
    expect(attachmentsPath('requirement', 'r1')).toBe('/requirements/r1/attachments')
  })
  it('routes linked items', () => {
    expect(linkTarget({ entity_type: 'requirement', entity_id: 'r', key: 'K' }, 'p')).toEqual({ name: 'requirements', params: { projectId: 'p' }, query: { item: 'r' } })
    expect(linkTarget({ entity_type: 'test', entity_id: 't', key: 'K' }, 'p')?.name).toBe('tests')
    expect(linkTarget({ entity_type: 'run', entity_id: 'x', key: 'K' }, 'p')).toEqual({ name: 'run', params: { projectId: 'p', runId: 'x' } })
    expect(linkTarget({ entity_type: 'execution', entity_id: 'e', key: 'K' }, 'p')?.name).toBe('runs')
    expect(linkTarget({ entity_type: 'execution', entity_id: 'e', key: 'K' }, 'p', 'r')?.params).toEqual({ projectId: 'p', runId: 'r', executionId: 'e' })
    expect(linkTarget({ entity_type: 'execution', entity_id: 'e', key: 'K', run_id: 'r2' }, 'p')?.params).toEqual({ projectId: 'p', runId: 'r2', executionId: 'e' })
    expect(linkTarget({ entity_type: 'other', entity_id: 'e', key: 'K' }, 'p')).toBeNull()
  })
  it('has icons per type', () => {
    expect(['requirement', 'test', 'run', 'execution', 'x'].map(linkIcon)).toEqual(['pi pi-list-check', 'pi pi-check-square', 'pi pi-play-circle', 'pi pi-play', 'pi pi-link'])
  })
})

describe('misc', () => {
  it('joins tags', () => {
    expect(tagsField([' a ', '', 'b'])).toBe('a,b')
    expect(tagsField(null)).toBe('')
  })
  it('sorts newest first, then by name', () => {
    const s = sortFiles([
      { name: 'b', created_at: '2026-01-01' },
      { name: 'a', created_at: '2026-01-01' },
      { name: 'c', created_at: '2026-02-01' },
    ])
    expect(s.map((x) => x.name)).toEqual(['c', 'a', 'b'])
  })
  it('builds archive params', () => {
    expect(archiveParams('report', ' tag:x ')).toEqual({ kind: 'report', q: 'tag:x' })
    expect(archiveParams('avatar', '')).toEqual({})
    expect(archiveParams(null, null)).toEqual({})
  })
  it('computes progress', () => {
    expect(progressPercent(50, 200)).toBe(25)
    expect(progressPercent(300, 200)).toBe(100)
    expect(progressPercent(1, 0)).toBe(0)
  })
})
