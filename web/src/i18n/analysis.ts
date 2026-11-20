// Translated texts for the requirement-language analysis. The API reports
// issues with an English message plus structured params; known rules get a
// message in the active UI language from those params (or, for older
// servers, params recovered from the API text), others keep the API message.
import type { LanguageIssue } from '@/lib/types'

export const ANALYSIS_RULES = [
  'missing-keyword',
  'lowercase-keyword',
  'multiple-obligations',
  'ambiguous-term',
  'tbd',
  'unbounded-term',
  'and-or',
  'passive-capability',
  'long-sentence',
  'negative-requirement',
  'escape-clause',
  'ambiguous-negation',
] as const

/** Message parameters per rule; null when the API text does not have the expected shape. */
export function issueParams(issue: Pick<LanguageIssue, 'rule' | 'message' | 'offset' | 'length' | 'params'>, text = ''): Record<string, string> | null {
  if (issue.params && Object.keys(issue.params).length) return { ...issue.params }
  const msg = issue.message ?? ''
  const quoted = msg.match(/"((?:[^"\\]|\\.)*)"/)?.[1]
  const fragment = text && issue.length > 0 ? text.slice(issue.offset, issue.offset + issue.length) : ''
  const term = quoted ?? fragment
  const paren = msg.match(/\(([^)]+)\)/)?.[1]
  switch (issue.rule) {
    case 'missing-keyword':
      return { examples: msg.match(/\(([^)]*?),? *(?:\.\.\.|…)\)/)?.[1] ?? 'MUST, SHALL, SHOULD, MAY' }
    case 'lowercase-keyword': {
      const kw = msg.match(/uppercase (\S+) has/)?.[1] ?? term.toUpperCase()
      return term ? { term, keyword: kw } : null
    }
    case 'multiple-obligations':
      return paren ? { keywords: paren } : null
    case 'negative-requirement':
      return paren ? { keyword: paren } : null
    case 'long-sentence': {
      const m = msg.match(/(\d+) words \(more than (\d+)\)/)
      return m ? { words: m[1], max: m[2] } : null
    }
    case 'ambiguous-term':
    case 'tbd':
    case 'unbounded-term':
    case 'and-or':
    case 'passive-capability':
    case 'escape-clause':
    case 'ambiguous-negation':
      return term ? { term } : null
    default:
      return null
  }
}

/**
 * Display text of an issue: `translate(key, params)` for known rules
 * (keys `analysis.rules.<rule>`), else the API message.
 */
export function issueMessage(
  issue: Pick<LanguageIssue, 'rule' | 'message' | 'offset' | 'length' | 'params'>,
  text: string,
  translate: (key: string, params: Record<string, string>) => string,
): string {
  if (!(ANALYSIS_RULES as readonly string[]).includes(issue.rule)) return issue.message
  const params = issueParams(issue, text)
  if (!params) return issue.message
  return translate(`analysis.rules.${issue.rule}`, params)
}
