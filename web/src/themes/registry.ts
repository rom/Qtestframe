// Colour theme registry: plain data only (no PrimeVue import), so lib/prefs
// and tests can use it. Presets are built from it in ./presets.ts and the
// matching --qtf-* CSS variables live in styles.css (kept in sync by a test).
import type { ContrastKind } from './contrast'

/** Every value of the `theme` preference. */
export const THEME_IDS = [
  'light',
  'dark',
  'system',
  'spring',
  'autumn',
  'desert',
  'camouflage',
  'city_camouflage',
  'navy',
  'green_console',
  'crazy',
  'ocean',
  'forest',
  'sunset',
  'nordic',
  'lavender',
  'high_contrast',
] as const
export type ThemeId = (typeof THEME_IDS)[number]

/** A theme that is actually painted ("system" resolves to light or dark). */
export type ConcreteThemeId = Exclude<ThemeId, 'system'>
export type Scheme = 'light' | 'dark'

export const DECORATIVE_THEMES = ['spring', 'autumn', 'desert', 'camouflage', 'city_camouflage', 'navy', 'green_console', 'crazy', 'ocean', 'forest', 'sunset', 'nordic', 'lavender', 'high_contrast'] as const satisfies readonly ConcreteThemeId[]
export type DecorativeThemeId = (typeof DECORATIVE_THEMES)[number]

export type Shade = 50 | 100 | 200 | 300 | 400 | 500 | 600 | 700 | 800 | 900 | 950
export type Scale = Record<Shade, string>
export type SurfaceScale = Scale & { 0: string }

/** Resolved, opaque key colours of a theme (what the contrast test checks). */
export interface ThemeColors {
  /** page background (--qtf-bg) */
  bg: string
  /** cards, tables, dialogs (--qtf-card, PrimeVue content background) */
  card: string
  /** body text (--qtf-text, PrimeVue text.color) */
  text: string
  /** secondary text (--qtf-muted, PrimeVue text.mutedColor) */
  muted: string
  border: string
  /** primary buttons, links, keys (PrimeVue primary.color) */
  primary: string
  primaryHover: string
  /** text on primary buttons (PrimeVue primary.contrastColor) */
  onPrimary: string
  sidebarBg: string
  sidebarText: string
  /** section headings in the sidebar */
  sidebarHeading: string
  /** brand and hovered nav items */
  sidebarStrong: string
  sidebarActiveBg: string
  /** text of the active nav item */
  sidebarActiveText: string
  /** opaque top bar background (default: the card colour) */
  topbarBg: string
  /** colours used in decorative sidebar/top bar patterns; sidebar and top bar text are checked against each */
  sidebarPattern?: string[]
  topbarPattern?: string[]
}

export interface ThemeDef {
  id: ThemeId
  /** null for "system", which follows the operating system */
  scheme: Scheme | null
  decorative: boolean
  /** colours of the small preview swatch (CSS colours or gradients) */
  swatch: { chrome: string; bg: string; card: string; accent: string }
  /** primary 50–950 for the PrimeVue preset */
  primary?: Scale
  /** surface 0–950 for the PrimeVue preset */
  surface?: SurfaceScale
  colors?: ThemeColors
  /** whole UI in a monospace font */
  mono?: boolean
  /** larger, rounder corners (px) */
  radius?: number
}

const INDIGO: Scale = {
  50: '#eef2ff',
  100: '#e0e7ff',
  200: '#c7d2fe',
  300: '#a5b4fc',
  400: '#818cf8',
  500: '#6366f1',
  600: '#4f46e5',
  700: '#4338ca',
  800: '#3730a3',
  900: '#312e81',
  950: '#1e1b4b',
}

export const THEME_DEFS: Record<ThemeId, ThemeDef> = {
  light: {
    id: 'light',
    scheme: 'light',
    decorative: false,
    swatch: { chrome: '#111827', bg: '#f4f6fb', card: '#ffffff', accent: '#4f46e5' },
    primary: INDIGO,
    colors: {
      bg: '#f4f6fb',
      card: '#ffffff',
      text: '#0f172a',
      muted: '#5b6678',
      border: '#e5e7eb',
      primary: '#4f46e5',
      primaryHover: '#4338ca',
      onPrimary: '#ffffff',
      sidebarBg: '#111827',
      sidebarText: '#cbd5e1',
      sidebarHeading: '#8b98ad',
      sidebarStrong: '#ffffff',
      sidebarActiveBg: '#232953',
      sidebarActiveText: '#ffffff',
      topbarBg: '#ffffff',
    },
  },
  dark: {
    id: 'dark',
    scheme: 'dark',
    decorative: false,
    swatch: { chrome: '#0a0f1c', bg: '#0b1020', card: '#111827', accent: '#818cf8' },
    primary: INDIGO,
    colors: {
      bg: '#0b1020',
      card: '#111827',
      text: '#e5e7eb',
      muted: '#94a3b8',
      border: '#1f2937',
      primary: '#818cf8',
      primaryHover: '#a5b4fc',
      onPrimary: '#18181b',
      sidebarBg: '#0a0f1c',
      sidebarText: '#cbd5e1',
      sidebarHeading: '#8b98ad',
      sidebarStrong: '#ffffff',
      sidebarActiveBg: '#1e224b',
      sidebarActiveText: '#ffffff',
      topbarBg: '#111827',
    },
  },
  system: {
    id: 'system',
    scheme: null,
    decorative: false,
    swatch: { chrome: 'linear-gradient(135deg, #111827 50%, #f4f6fb 50%)', bg: '#f4f6fb', card: '#111827', accent: '#6366f1' },
  },

  spring: {
    id: 'spring',
    scheme: 'light',
    decorative: true,
    swatch: { chrome: '#e6f4ea', bg: '#f5faf4', card: '#fde4ec', accent: '#2a7a4b' },
    primary: {
      50: '#effaf2',
      100: '#d9f2e0',
      200: '#b5e4c4',
      300: '#88d0a2',
      400: '#58b67c',
      500: '#389a5f',
      600: '#2a7a4b',
      700: '#23633e',
      800: '#1e4f33',
      900: '#1a422c',
      950: '#0b2517',
    },
    surface: {
      0: '#ffffff',
      50: '#f6faf7',
      100: '#edf5ef',
      200: '#dce9e0',
      300: '#c4d5ca',
      400: '#9aaea1',
      500: '#6b8173',
      600: '#526659',
      700: '#3d4f44',
      800: '#2b3a31',
      900: '#1c2720',
      950: '#101812',
    },
    colors: {
      bg: '#f5faf4',
      card: '#ffffff',
      text: '#1d2b23',
      muted: '#53675a',
      border: '#d9e8dd',
      primary: '#2a7a4b',
      primaryHover: '#23633e',
      onPrimary: '#ffffff',
      sidebarBg: '#e6f4ea',
      sidebarText: '#2b3a31',
      sidebarHeading: '#4e6857',
      sidebarStrong: '#14301f',
      sidebarActiveBg: '#fbd9e5',
      sidebarActiveText: '#7a1f45',
      topbarBg: '#ffffff',
      sidebarPattern: ['#e6f4ea', '#e3eff8'],
    },
  },

  autumn: {
    id: 'autumn',
    scheme: 'light',
    decorative: true,
    swatch: { chrome: '#5a1e14', bg: '#f8f0e1', card: '#fffbf3', accent: '#b0461a' },
    primary: {
      50: '#fdf4ec',
      100: '#fae4d1',
      200: '#f4c6a2',
      300: '#eca06c',
      400: '#e17a3d',
      500: '#cf5e22',
      600: '#b0461a',
      700: '#913817',
      800: '#763018',
      900: '#612917',
      950: '#36130a',
    },
    surface: {
      0: '#fffbf3',
      50: '#fbf5ea',
      100: '#f5ebd9',
      200: '#eadbc0',
      300: '#d9c3a0',
      400: '#b99f7a',
      500: '#927858',
      600: '#735d43',
      700: '#5a4633',
      800: '#433426',
      900: '#2e231a',
      950: '#1c140e',
    },
    colors: {
      bg: '#f8f0e1',
      card: '#fffbf3',
      text: '#2d1d12',
      muted: '#6b5440',
      border: '#e6d5b8',
      primary: '#a8431a',
      primaryHover: '#913817',
      onPrimary: '#ffffff',
      sidebarBg: '#5a1e14',
      sidebarText: '#f4dfc4',
      sidebarHeading: '#d9ad7c',
      sidebarStrong: '#ffffff',
      sidebarActiveBg: '#86321b',
      sidebarActiveText: '#ffffff',
      topbarBg: '#fffbf3',
    },
  },

  desert: {
    id: 'desert',
    scheme: 'light',
    decorative: true,
    swatch: { chrome: '#56644f', bg: '#f4eada', card: '#fffaf0', accent: '#a64a2e' },
    primary: {
      50: '#fcf3ee',
      100: '#f8e3d8',
      200: '#f0c5b0',
      300: '#e49f81',
      400: '#d57a57',
      500: '#c3603c',
      600: '#a64a2e',
      700: '#893c26',
      800: '#703223',
      900: '#5c2b20',
      950: '#32150f',
    },
    surface: {
      0: '#fffaf0',
      50: '#fbf6ec',
      100: '#f4ebdb',
      200: '#e8dac1',
      300: '#d6c29f',
      400: '#bba27b',
      500: '#9a825f',
      600: '#7a6548',
      700: '#5f4e38',
      800: '#46392a',
      900: '#30271d',
      950: '#1d1711',
    },
    colors: {
      bg: '#f4eada',
      card: '#fffaf0',
      text: '#33271b',
      muted: '#685640',
      border: '#e2d2b4',
      primary: '#a64a2e',
      primaryHover: '#893c26',
      onPrimary: '#ffffff',
      sidebarBg: '#56644f',
      sidebarText: '#f1f0e4',
      sidebarHeading: '#f0e0b0',
      sidebarStrong: '#ffffff',
      sidebarActiveBg: '#d7a94e',
      sidebarActiveText: '#2a2012',
      topbarBg: '#fffaf0',
    },
  },

  camouflage: {
    id: 'camouflage',
    scheme: 'dark',
    decorative: true,
    swatch: { chrome: 'linear-gradient(135deg, #4b5320 0 30%, #5c4a32 30% 55%, #2e3b22 55% 80%, #8a7f5a 80%)', bg: '#2c2f23', card: '#373b2c', accent: '#c9c27e' },
    primary: {
      50: '#f8f7ec',
      100: '#eeecd2',
      200: '#ddd9a8',
      300: '#c9c27e',
      400: '#b3ab5a',
      500: '#989042',
      600: '#797334',
      700: '#5d5829',
      800: '#4a4724',
      900: '#3d3b21',
      950: '#21200f',
    },
    surface: {
      0: '#efeedd',
      50: '#e9e9da',
      100: '#d9dac6',
      200: '#c0c2a8',
      300: '#a5a88a',
      400: '#8a8d70',
      500: '#6f7259',
      600: '#585b46',
      700: '#4a4d3a',
      800: '#404333',
      900: '#373b2c',
      950: '#2c2f23',
    },
    colors: {
      bg: '#2c2f23',
      card: '#373b2c',
      text: '#efeedd',
      muted: '#c0c0a2',
      border: '#4f523e',
      primary: '#c9c27e',
      primaryHover: '#ddd9a8',
      onPrimary: '#1e2014',
      sidebarBg: '#333a22',
      sidebarText: '#ecead6',
      sidebarHeading: '#cfc89a',
      sidebarStrong: '#ffffff',
      sidebarActiveBg: '#555c2c',
      sidebarActiveText: '#ffffff',
      topbarBg: '#3a3f2b',
      sidebarPattern: ['#3d4526', '#463b2a', '#2a3520', '#45472f'],
      topbarPattern: ['#3f442e', '#433d2e', '#363c29'],
    },
  },

  city_camouflage: {
    id: 'city_camouflage',
    scheme: 'dark',
    decorative: true,
    swatch: { chrome: 'linear-gradient(135deg, #3b3e43 0 30%, #55595f 30% 55%, #2a2c30 55% 80%, #6c7178 80%)', bg: '#222428', card: '#2c2e33', accent: '#a3bbd1' },
    primary: {
      50: '#f2f6f9',
      100: '#e1eaf1',
      200: '#c4d5e3',
      300: '#a3bbd1',
      400: '#829fbb',
      500: '#6785a1',
      600: '#526b85',
      700: '#43576c',
      800: '#38495a',
      900: '#2f3d4c',
      950: '#1e2731',
    },
    surface: {
      0: '#eceef0',
      50: '#e6e8eb',
      100: '#d5d8dc',
      200: '#bbc0c6',
      300: '#9ea4ab',
      400: '#82888f',
      500: '#696e75',
      600: '#53575e',
      700: '#44474d',
      800: '#383a40',
      900: '#2c2e33',
      950: '#222428',
    },
    colors: {
      bg: '#222428',
      card: '#2c2e33',
      text: '#eceef0',
      muted: '#b3b8bf',
      border: '#43464c',
      primary: '#a3bbd1',
      primaryHover: '#c4d5e3',
      onPrimary: '#161c24',
      sidebarBg: '#2a2c30',
      sidebarText: '#e3e5e8',
      sidebarHeading: '#a9b9c9',
      sidebarStrong: '#ffffff',
      sidebarActiveBg: '#3d4b5a',
      sidebarActiveText: '#ffffff',
      topbarBg: '#303237',
      sidebarPattern: ['#36393e', '#24262a', '#303237'],
      topbarPattern: ['#35373c', '#2d2f34'],
    },
  },

  navy: {
    id: 'navy',
    scheme: 'dark',
    decorative: true,
    swatch: { chrome: '#0a1326', bg: '#0f1a31', card: '#172541', accent: '#ddb03d' },
    primary: {
      50: '#fdf9ec',
      100: '#f9efcc',
      200: '#f2dd96',
      300: '#e9c65f',
      400: '#ddb03d',
      500: '#c8952a',
      600: '#a87421',
      700: '#86571e',
      800: '#6f461f',
      900: '#5e3b1e',
      950: '#361e0c',
    },
    surface: {
      0: '#f5f8fd',
      50: '#eef3fb',
      100: '#d9e3f3',
      200: '#b5c7e4',
      300: '#8ba5cf',
      400: '#6a86b6',
      500: '#4d6899',
      600: '#3a527d',
      700: '#2b4066',
      800: '#203253',
      900: '#172541',
      950: '#0f1a31',
    },
    colors: {
      bg: '#0f1a31',
      card: '#172541',
      text: '#f5f8fd',
      muted: '#aebfdb',
      border: '#2b4066',
      primary: '#ddb03d',
      primaryHover: '#e9c65f',
      onPrimary: '#0b1427',
      sidebarBg: '#0a1326',
      sidebarText: '#d9e3f3',
      sidebarHeading: '#c9a64e',
      sidebarStrong: '#f2d27a',
      sidebarActiveBg: '#1d2f52',
      sidebarActiveText: '#f2d27a',
      topbarBg: '#132039',
    },
  },

  green_console: {
    id: 'green_console',
    scheme: 'dark',
    decorative: true,
    mono: true,
    swatch: { chrome: '#020703', bg: '#050c07', card: '#0a160d', accent: '#33f06a' },
    primary: {
      50: '#ecfff0',
      100: '#cfffd9',
      200: '#a0ffb4',
      300: '#66ff8a',
      400: '#33f06a',
      500: '#1fd455',
      600: '#17a843',
      700: '#148236',
      800: '#13662e',
      900: '#105428',
      950: '#032e12',
    },
    surface: {
      0: '#b6f7c5',
      50: '#a6efb8',
      100: '#8fe2a4',
      200: '#6fc987',
      300: '#56ae6e',
      400: '#44925a',
      500: '#357548',
      600: '#285b37',
      700: '#1d432a',
      800: '#14301e',
      900: '#0a160d',
      950: '#050c07',
    },
    colors: {
      bg: '#050c07',
      card: '#0a160d',
      text: '#b6f7c5',
      muted: '#6fc987',
      border: '#1d432a',
      primary: '#33f06a',
      primaryHover: '#66ff8a',
      onPrimary: '#021405',
      sidebarBg: '#020703',
      sidebarText: '#8fe2a4',
      sidebarHeading: '#56ae6e',
      sidebarStrong: '#66ff8a',
      sidebarActiveBg: '#0d2a16',
      sidebarActiveText: '#66ff8a',
      topbarBg: '#07110a',
    },
  },

  crazy: {
    id: 'crazy',
    scheme: 'light',
    decorative: true,
    radius: 18,
    swatch: { chrome: 'linear-gradient(160deg, #c2187a, #6a2bd9 50%, #086b8b)', bg: '#fff8dc', card: '#ffffff', accent: '#ad0b6c' },
    primary: {
      50: '#fff0f9',
      100: '#ffdcf1',
      200: '#ffb8e3',
      300: '#ff85cf',
      400: '#ff4fb7',
      500: '#f5209d',
      600: '#d30d83',
      700: '#ad0b6c',
      800: '#8c0d59',
      900: '#740f4b',
      950: '#47022a',
    },
    surface: {
      0: '#ffffff',
      50: '#fbf7ff',
      100: '#f4ecff',
      200: '#e7daff',
      300: '#d1bdf3',
      400: '#a993d6',
      500: '#7d66ab',
      600: '#614e89',
      700: '#4a3b6b',
      800: '#34294f',
      900: '#231b37',
      950: '#140e21',
    },
    colors: {
      bg: '#fff8dc',
      card: '#ffffff',
      text: '#1e1433',
      muted: '#594876',
      border: '#e7d6ff',
      primary: '#ad0b6c',
      primaryHover: '#8c0d59',
      onPrimary: '#ffffff',
      sidebarBg: '#6a2bd9',
      sidebarText: '#ffffff',
      sidebarHeading: '#fff3a3',
      sidebarStrong: '#ffffff',
      sidebarActiveBg: '#ffe14d',
      sidebarActiveText: '#1e1433',
      topbarBg: '#ffffff',
      sidebarPattern: ['#c2187a', '#6a2bd9', '#086b8b'],
      topbarPattern: ['#fff3b0', '#ffd6ef', '#c9f3ff'],
    },
  },
  ocean: {
    id: 'ocean',
    scheme: 'light',
    decorative: true,
    swatch: { chrome: '#0b3a4f', bg: '#f0f8fb', card: '#ffffff', accent: '#0b6680' },
    primary: {
      50: '#ecf4f6',
      100: '#d4e6eb',
      200: '#a8cdd7',
      300: '#78b1c1',
      400: '#4393a8',
      500: '#0e7490',
      600: '#0c6179',
      700: '#0a4f62',
      800: '#083f4e',
      900: '#06313c',
      950: '#041e25',
    },
    surface: {
      0: '#ffffff',
      50: '#f3f4NaN',
      100: '#e3e6NaN',
      200: '#c7cdNaN',
      300: '#a8b1NaN',
      400: '#8693NaN',
      500: '#6474NaN',
      600: '#5461NaN',
      700: '#444fNaN',
      800: '#363fNaN',
      900: '#2a31NaN',
      950: '#1a1eNaN',
    },
    colors: {
      bg: '#f0f8fb',
      card: '#ffffff',
      text: '#0c2a36',
      muted: '#47616d',
      border: '#cfe4ec',
      primary: '#0b6680',
      primaryHover: '#095368',
      onPrimary: '#ffffff',
      sidebarBg: '#0b3a4f',
      sidebarText: '#d3ecf4',
      sidebarHeading: '#8fc7d9',
      sidebarStrong: '#ffffff',
      sidebarActiveBg: '#13607d',
      sidebarActiveText: '#ffffff',
      topbarBg: '#ffffff',
      sidebarPattern: ['#0b3a4f', '#0d4660'],
    },
  },

  forest: {
    id: 'forest',
    scheme: 'dark',
    decorative: true,
    swatch: { chrome: '#0c140e', bg: '#111a14', card: '#18241b', accent: '#8fcf78' },
    primary: {
      50: '#f5faf3',
      100: '#e8f3e4',
      200: '#d1e8c9',
      300: '#b7dbac',
      400: '#9bcd8b',
      500: '#7fbf6a',
      600: '#6ba059',
      700: '#568248',
      800: '#456739',
      900: '#35502d',
      950: '#21321c',
    },
    surface: {
      0: '#f9faf8',
      50: '#f2f4f2',
      100: '#e2e7e1',
      200: '#c5cfc4',
      300: '#a5b5a3',
      400: '#82977e',
      500: '#5f7a5a',
      600: '#50664c',
      700: '#41533d',
      800: '#334231',
      900: '#283326',
      950: '#192017',
    },
    colors: {
      bg: '#111a14',
      card: '#18241b',
      text: '#e6f0e3',
      muted: '#a7bba2',
      border: '#2c3d2f',
      primary: '#8fcf78',
      primaryHover: '#a6dc93',
      onPrimary: '#0e1a10',
      sidebarBg: '#0c140e',
      sidebarText: '#cfe0c9',
      sidebarHeading: '#9fbf8c',
      sidebarStrong: '#e9f7df',
      sidebarActiveBg: '#24412a',
      sidebarActiveText: '#e9f7df',
      topbarBg: '#141f17',
      sidebarPattern: ['#0c140e', '#122016'],
    },
  },

  sunset: {
    id: 'sunset',
    scheme: 'light',
    decorative: true,
    swatch: { chrome: '#4a1d2e', bg: '#fff7f1', card: '#ffffff', accent: '#b23b0a' },
    primary: {
      50: '#faf0ec',
      100: '#f4ddd3',
      200: '#e9bba8',
      300: '#dd9577',
      400: '#cf6b41',
      500: '#c2410c',
      600: '#a3370a',
      700: '#842c08',
      800: '#692306',
      900: '#511b05',
      950: '#321103',
    },
    surface: {
      0: '#ffffff',
      50: '#f6f3NaN',
      100: '#eae5NaN',
      200: '#d5cbNaN',
      300: '#bdaeNaN',
      400: '#a48fNaN',
      500: '#8a6fNaN',
      600: '#745dNaN',
      700: '#5e4bNaN',
      800: '#4b3cNaN',
      900: '#3a2fNaN',
      950: '#241dNaN',
    },
    colors: {
      bg: '#fff7f1',
      card: '#ffffff',
      text: '#3a1f16',
      muted: '#6e4e43',
      border: '#f1d9cb',
      primary: '#b23b0a',
      primaryHover: '#932f08',
      onPrimary: '#ffffff',
      sidebarBg: '#4a1d2e',
      sidebarText: '#f7dbe0',
      sidebarHeading: '#f0b08f',
      sidebarStrong: '#ffffff',
      sidebarActiveBg: '#7a2c43',
      sidebarActiveText: '#ffffff',
      topbarBg: '#ffffff',
      sidebarPattern: ['#4a1d2e', '#5a2433', '#3f1a2b'],
    },
  },

  nordic: {
    id: 'nordic',
    scheme: 'light',
    decorative: true,
    swatch: { chrome: '#e9edf1', bg: '#f4f6f8', card: '#ffffff', accent: '#3f5f80' },
    primary: {
      50: '#f1f3f6',
      100: '#dfe4ea',
      200: '#bfc9d5',
      300: '#9bacbd',
      400: '#738ba4',
      500: '#4c6a8a',
      600: '#405974',
      700: '#34485e',
      800: '#29394b',
      900: '#202d3a',
      950: '#141c24',
    },
    surface: {
      0: '#ffffff',
      50: '#f3f4NaN',
      100: '#e4e6NaN',
      200: '#caceNaN',
      300: '#acb2NaN',
      400: '#8c94NaN',
      500: '#6b76NaN',
      600: '#5a63NaN',
      700: '#4950NaN',
      800: '#3a40NaN',
      900: '#2d32NaN',
      950: '#1c1fNaN',
    },
    colors: {
      bg: '#f4f6f8',
      card: '#ffffff',
      text: '#262d36',
      muted: '#55606c',
      border: '#dde2e8',
      primary: '#3f5f80',
      primaryHover: '#334e6a',
      onPrimary: '#ffffff',
      sidebarBg: '#e9edf1',
      sidebarText: '#2f3842',
      sidebarHeading: '#56626f',
      sidebarStrong: '#1b2530',
      sidebarActiveBg: '#d4dfeb',
      sidebarActiveText: '#1f3550',
      topbarBg: '#ffffff',
    },
  },

  lavender: {
    id: 'lavender',
    scheme: 'light',
    decorative: true,
    swatch: { chrome: '#efe7fa', bg: '#f8f5fd', card: '#ffffff', accent: '#6a3fab' },
    primary: {
      50: '#f5f1fa',
      100: '#e7dff3',
      200: '#d0bfe7',
      300: '#b69bd9',
      400: '#9974cb',
      500: '#7c4dbc',
      600: '#68419e',
      700: '#543480',
      800: '#432a66',
      900: '#34204f',
      950: '#201431',
    },
    surface: {
      0: '#ffffff',
      50: '#f4f4NaN',
      100: '#e7e5NaN',
      200: '#ceccNaN',
      300: '#b3afNaN',
      400: '#968fNaN',
      500: '#7870NaN',
      600: '#655eNaN',
      700: '#524cNaN',
      800: '#413cNaN',
      900: '#322fNaN',
      950: '#1f1dNaN',
    },
    colors: {
      bg: '#f8f5fd',
      card: '#ffffff',
      text: '#2c2240',
      muted: '#5f5575',
      border: '#e5dcf3',
      primary: '#6a3fab',
      primaryHover: '#57338d',
      onPrimary: '#ffffff',
      sidebarBg: '#efe7fa',
      sidebarText: '#352a4c',
      sidebarHeading: '#62557c',
      sidebarStrong: '#21163a',
      sidebarActiveBg: '#ddccf5',
      sidebarActiveText: '#3f1f73',
      topbarBg: '#ffffff',
      sidebarPattern: ['#efe7fa', '#f6e9f3'],
    },
  },

  high_contrast: {
    id: 'high_contrast',
    scheme: 'dark',
    decorative: true,
    swatch: { chrome: '#000000', bg: '#000000', card: '#0a0a0a', accent: '#ffd400' },
    primary: {
      50: '#fffceb',
      100: '#fff7d1',
      200: '#fff0a3',
      300: '#ffe770',
      400: '#ffdd38',
      500: '#ffd400',
      600: '#d6b200',
      700: '#ad9000',
      800: '#8a7200',
      900: '#6b5900',
      950: '#423700',
    },
    surface: {
      0: '#fafafa',
      50: '#f5f5f5',
      100: '#e8e8e8',
      200: '#d1d1d1',
      300: '#b8b8b8',
      400: '#9c9c9c',
      500: '#808080',
      600: '#6c6c6c',
      700: '#575757',
      800: '#454545',
      900: '#363636',
      950: '#212121',
    },
    colors: {
      bg: '#000000',
      card: '#0a0a0a',
      text: '#ffffff',
      muted: '#d6d6d6',
      border: '#ffffff',
      primary: '#ffd400',
      primaryHover: '#ffe55c',
      onPrimary: '#000000',
      sidebarBg: '#000000',
      sidebarText: '#ffffff',
      sidebarHeading: '#ffd400',
      sidebarStrong: '#ffffff',
      sidebarActiveBg: '#ffd400',
      sidebarActiveText: '#000000',
      topbarBg: '#000000',
    },
  },
}

/** Default English display names (the i18n layer can override them). */
export const THEME_LABELS: Record<ThemeId, string> = {
  light: 'Light',
  dark: 'Dark',
  system: 'Follow the operating system',
  spring: 'Spring',
  autumn: 'Autumn',
  desert: 'Desert',
  camouflage: 'Camouflage',
  city_camouflage: 'City camouflage',
  navy: 'Navy',
  green_console: 'Green console',
  crazy: 'Crazy',
  ocean: 'Ocean',
  forest: 'Forest',
  sunset: 'Sunset',
  nordic: 'Nordic',
  lavender: 'Lavender',
  high_contrast: 'High contrast',
}

/** Other default English strings used by ThemePicker. */
export const THEME_TEXT = {
  picker: 'Colour theme',
  unset: 'Not set (this device)',
} as const

export function isThemeId(v: unknown): v is ThemeId {
  return typeof v === 'string' && (THEME_IDS as readonly string[]).includes(v)
}

export function isDecorative(v: unknown): v is DecorativeThemeId {
  return typeof v === 'string' && (DECORATIVE_THEMES as readonly string[]).includes(v)
}

/** Light or dark scheme of a theme; "system" uses the OS setting. */
export function schemeOf(id: ThemeId, systemDark = false): Scheme {
  const s = THEME_DEFS[id].scheme
  return s ?? (systemDark ? 'dark' : 'light')
}

/** The painted theme for a preference value ("system" → light/dark). */
export function concreteTheme(id: ThemeId, systemDark = false): ConcreteThemeId {
  return id === 'system' ? (systemDark ? 'dark' : 'light') : id
}

export interface ContrastPair {
  name: string
  fg: string
  bg: string
  kind: ContrastKind
}

/** Key foreground/background pairs of a theme that must meet WCAG AA. */
export function contrastPairs(c: ThemeColors): ContrastPair[] {
  const pairs: ContrastPair[] = [
    { name: 'text on background', fg: c.text, bg: c.bg, kind: 'text' },
    { name: 'text on card', fg: c.text, bg: c.card, kind: 'text' },
    { name: 'muted text on card', fg: c.muted, bg: c.card, kind: 'text' },
    { name: 'muted text on background', fg: c.muted, bg: c.bg, kind: 'text' },
    { name: 'link on card', fg: c.primary, bg: c.card, kind: 'text' },
    { name: 'link on background', fg: c.primary, bg: c.bg, kind: 'text' },
    { name: 'primary button text', fg: c.onPrimary, bg: c.primary, kind: 'text' },
    { name: 'primary button text (hover)', fg: c.onPrimary, bg: c.primaryHover, kind: 'text' },
    { name: 'primary button against card', fg: c.primary, bg: c.card, kind: 'ui' },
    { name: 'text on top bar', fg: c.text, bg: c.topbarBg, kind: 'text' },
    { name: 'muted text on top bar', fg: c.muted, bg: c.topbarBg, kind: 'text' },
    { name: 'sidebar text', fg: c.sidebarText, bg: c.sidebarBg, kind: 'text' },
    { name: 'sidebar heading', fg: c.sidebarHeading, bg: c.sidebarBg, kind: 'text' },
    { name: 'sidebar brand / hovered item', fg: c.sidebarStrong, bg: c.sidebarBg, kind: 'text' },
    { name: 'sidebar active item', fg: c.sidebarActiveText, bg: c.sidebarActiveBg, kind: 'text' },
    // :focus-visible ring (styles.css): an outline 2px outside the element, the gap filled by an
    // inner ring in the card (sidebar: sidebar) colour, so the outline only touches these colours
    { name: 'focus ring on card', fg: c.primary, bg: c.card, kind: 'ui' },
    { name: 'focus ring on background', fg: c.primary, bg: c.bg, kind: 'ui' },
    { name: 'focus ring on top bar', fg: c.primary, bg: c.topbarBg, kind: 'ui' },
    { name: 'focus ring in the sidebar', fg: c.sidebarStrong, bg: c.sidebarBg, kind: 'ui' },
  ]
  for (const p of c.sidebarPattern ?? []) {
    pairs.push({ name: `sidebar text on pattern ${p}`, fg: c.sidebarText, bg: p, kind: 'text' })
    pairs.push({ name: `sidebar heading on pattern ${p}`, fg: c.sidebarHeading, bg: p, kind: 'text' })
    pairs.push({ name: `sidebar brand on pattern ${p}`, fg: c.sidebarStrong, bg: p, kind: 'text' })
  }
  for (const p of c.topbarPattern ?? []) {
    pairs.push({ name: `top bar text on pattern ${p}`, fg: c.text, bg: p, kind: 'text' })
    pairs.push({ name: `top bar muted text on pattern ${p}`, fg: c.muted, bg: p, kind: 'text' })
  }
  return pairs
}
