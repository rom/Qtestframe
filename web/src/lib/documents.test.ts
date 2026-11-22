import { describe, expect, it } from 'vitest'
import { DOC_FIELDS, blankDocForm, defaultDocFields, docFormProblems, generateDocBody, switchDocKind } from './documents'

describe('generated documents', () => {
  it('shows every field but the latest results by default', () => {
    expect(defaultDocFields('requirements_spec')).not.toContain('results')
    expect(defaultDocFields('requirements_spec')).toContain('tests')
    expect(defaultDocFields('test_plan')).toEqual(DOC_FIELDS.test_plan.filter((f) => f !== 'results'))
  })

  it('builds the request body', () => {
    const f = blankDocForm('requirements_spec', 'sv')
    expect(generateDocBody(f)).toEqual({
      kind: 'requirements_spec',
      format: 'pdf',
      options: { fields: defaultDocFields('requirements_spec'), include_inactive: false, traceability: true, glossary: true, lang: 'sv', group_by: 'folder', dependency_graph: true, new_page_per_requirement: true },
    })
    const body = generateDocBody({ ...f, title: ' SRS ', version: '2.0', author: '', query: 'status:approved', baselineId: 'b1', groupBy: 'category', dimension: 'Level', format: 'md', fields: ['results', 'description'] })
    expect(body.format).toBe('md')
    expect(body.options).toMatchObject({ title: 'SRS', version: '2.0', query: 'status:approved', baseline_id: 'b1', group_by: 'category', dimension: 'Level', fields: ['description', 'results'] })
    expect(body.options).not.toHaveProperty('author')
    // a test plan has no grouping
    const plan = generateDocBody({ ...blankDocForm('test_plan', 'en'), groupBy: 'type' })
    expect(plan.options).not.toHaveProperty('group_by')
    expect(generateDocBody({ ...f, groupBy: 'category' }).options).not.toHaveProperty('dimension')
    expect(generateDocBody(f).options).not.toHaveProperty('include_discussions')
    expect(generateDocBody({ ...f, discussions: true }).options.include_discussions).toBe(true)
  })

  it('starts every requirement of a specification on a new page by default', () => {
    const f = blankDocForm('requirements_spec', 'en')
    expect(f.newPagePerRequirement).toBe(true)
    expect(generateDocBody({ ...f, newPagePerRequirement: false }).options.new_page_per_requirement).toBe(false)
    expect(generateDocBody(blankDocForm('test_plan', 'en')).options).not.toHaveProperty('new_page_per_requirement')
    const off = { ...f, newPagePerRequirement: false }
    expect(switchDocKind(switchDocKind(off, 'test_plan'), 'requirements_spec').newPagePerRequirement).toBe(false)
  })

  it('adds the dependency graph appendix to a specification by default', () => {
    const f = blankDocForm('requirements_spec', 'en')
    expect(f.dependencyGraph).toBe(true)
    expect(generateDocBody(f).options.dependency_graph).toBe(true)
    expect(generateDocBody({ ...f, dependencyGraph: false }).options.dependency_graph).toBe(false)
    // a test plan has none
    expect(generateDocBody(blankDocForm('test_plan', 'en')).options).not.toHaveProperty('dependency_graph')
    // switching the kind keeps the choice
    const off = { ...f, dependencyGraph: false }
    expect(switchDocKind(switchDocKind(off, 'test_plan'), 'requirements_spec').dependencyGraph).toBe(false)
  })

  it('switches the kind with its own fields and without the filter', () => {
    const f = { ...blankDocForm('requirements_spec', 'en'), query: 'status:draft', fields: ['description', 'results'], title: 'Mine' }
    const p = switchDocKind(f, 'test_plan')
    expect(p.kind).toBe('test_plan')
    expect(p.query).toBe('')
    expect(p.title).toBe('Mine')
    expect(p.fields).toEqual(DOC_FIELDS.test_plan)
    expect(switchDocKind(p, 'test_plan')).toBe(p)
    expect(switchDocKind({ ...p, fields: ['steps'] }, 'requirements_spec').fields).toEqual(defaultDocFields('requirements_spec'))
  })

  it('reports problems', () => {
    const f = blankDocForm('test_plan', 'en')
    expect(docFormProblems(f)).toEqual([])
    expect(docFormProblems({ ...f, fields: [], title: 'x'.repeat(201), version: 'v'.repeat(41) })).toEqual(['fields', 'title', 'version'])
  })
})
