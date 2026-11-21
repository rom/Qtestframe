// Chart colours and fonts per painted theme (used by composables/chartTheme).
// Text and grid follow the theme's muted text and border colours; series
// colours start with the theme's primary colour. Charts whose colours carry
// meaning (passed/failed, coverage) keep the semantic colours from lib/colors.
import { THEME_DEFS, type ConcreteThemeId } from './registry'

export const SANS_FONT = "Inter, ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"
export const MONO_FONT = "ui-monospace, SFMono-Regular, 'JetBrains Mono', Menlo, Consolas, monospace"

/** Categorical series per theme (after the primary colour). */
const SERIES: Record<ConcreteThemeId, string[]> = {
  light: ['#0ea5e9', '#10b981', '#f59e0b', '#ec4899', '#8b5cf6', '#14b8a6', '#ef4444'],
  dark: ['#38bdf8', '#34d399', '#fbbf24', '#f472b6', '#a78bfa', '#2dd4bf', '#f87171'],
  spring: ['#f7a8c4', '#7cc0e8', '#9ad7ad', '#e8b04f', '#b59ad9', '#5fb3a1', '#e07a8f'],
  autumn: ['#d9a95f', '#86321b', '#e17a3d', '#7c8b3a', '#a65a2a', '#c9822b', '#5a1e14'],
  desert: ['#d7a94e', '#c3603c', '#56644f', '#8d6e48', '#e0bf7c', '#9a4f32', '#6f8f7a'],
  camouflage: ['#989042', '#c0b080', '#7d8a4a', '#a98d5f', '#5c6b3a', '#d2c79a', '#8a6f4a'],
  city_camouflage: ['#829fbb', '#b3b8bf', '#6d8aa3', '#9aa4ad', '#c0cad4', '#5f7690', '#8e98a3'],
  arctic_camouflage: ['#4a749b', '#8f9eae', '#1f3e58', '#6890b6', '#5a6977', '#92b3cf', '#38424c'],
  navy: ['#f2d27a', '#7fa6e0', '#c9a64e', '#9fb9e3', '#e0b85a', '#5b86c9', '#d9e3f3'],
  midnight: ['#60a5fa', '#f472b6', '#34d399', '#fbbf24', '#c4b5fd', '#22d3ee', '#fb7185'],
  green_console: ['#66ff8a', '#1fd455', '#a0ffb4', '#17a843', '#cfffd9', '#56ae6e', '#33f06a'],
  amber_terminal: ['#ffc54d', '#e69a00', '#ffd98a', '#b87a00', '#fff0cc', '#d4a965', '#ff8c1a'],
  crazy: ['#ff4fb7', '#22d3ee', '#ffe14d', '#6a2bd9', '#2dd47a', '#ff8a1f', '#c2187a'],
  ocean: ['#0b6680', '#2bb3c9', '#f28c6b', '#5c7cfa', '#1f9d76', '#e0b545', '#94a3b8'],
  forest: ['#8fcf78', '#d9b45a', '#6fb3a8', '#c98a5a', '#a7c96f', '#7d9fd1', '#cfe0c9'],
  sunset: ['#e8743b', '#b23b0a', '#f2b134', '#7a2c43', '#d95d8a', '#6c5b9e', '#f0b08f'],
  nordic: ['#5e81ac', '#bf616a', '#a3be8c', '#d08770', '#b48ead', '#88c0d0', '#ebcb8b'],
  lavender: ['#9b72d6', '#e48bb5', '#6fb7d9', '#c9a0f0', '#7fc8a9', '#f0b77a', '#6a3fab'],
  rose: ['#e06f92', '#73203f', '#f0a35e', '#8a6fb8', '#4f9d8a', '#cc4773', '#d9a0b4'],
  high_contrast: ['#ffd400', '#00e5ff', '#ff6ec7', '#7cff4f', '#ff9a3c', '#b9a6ff', '#ffffff'],
  high_contrast_light: ['#000000', '#b30000', '#006b1f', '#7a1fa2', '#a65200', '#005f73', '#525252'],
}

export interface ChartColors {
  /** tick and legend text */
  text: string
  /** grid lines */
  grid: string
  /** chart background (doughnut segment borders) */
  surface: string
  /** categorical series, primary first */
  series: string[]
  /** font family for all chart text */
  font: string
  dark: boolean
}

/** Chart colours for a painted theme. */
export function chartColors(id: ConcreteThemeId): ChartColors {
  const def = THEME_DEFS[id] ?? THEME_DEFS.light
  const c = def.colors!
  return {
    text: c.muted,
    grid: c.border,
    surface: c.card,
    series: [c.primary, ...SERIES[def.id as ConcreteThemeId]],
    font: def.mono ? MONO_FONT : SANS_FONT,
    dark: def.scheme === 'dark',
  }
}

/** The i-th series colour (wraps around). */
export function seriesColor(colors: Pick<ChartColors, 'series'>, i: number): string {
  const s = colors.series
  return s[Math.abs(i) % s.length]
}
