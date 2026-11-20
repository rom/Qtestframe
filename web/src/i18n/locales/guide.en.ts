// English texts of the in-app user guide and the intro tour (merged into
// the app messages in i18n/index.ts; guide.sv.ts must have the same keys).
const guideEn = {
  titles: {
    help: 'Help',
  },
  guide: {
    help: 'Help',
    helpMenu: 'Help menu',
    userGuide: 'User guide',
    restartTour: 'Restart intro tour',
    thisPage: 'Help for this page',
    contextHelp: 'Help: {topic}',
    contents: 'Contents',
    contentsNav: 'User guide contents',
    onThisPage: 'On this page',
    openContents: 'Show contents',
    search: 'Search the guide',
    searchPlaceholder: 'Search the guide…',
    searchResults: 'No results | 1 result | {count} results',
    searchNothing: 'Nothing found for “{query}”.',
    clearSearch: 'Clear search',
    pager: 'Previous and next page',
    previous: 'Previous: {title}',
    next: 'Next: {title}',
    fallback: 'This page is not available in your language yet; it is shown in English.',
    notFound: 'This guide page does not exist.',
    backToIndex: 'Go to the guide overview',
    table: 'Table (scrolls sideways)',
    missingImage: 'Screenshot not available: {alt}',
  },
  tour: {
    dialog: 'Introduction to Qtestframe',
    stepOf: 'Step {n} of {total}',
    next: 'Next',
    back: 'Back',
    skip: 'Skip tour',
    finish: 'Finish',
    started: 'The intro tour has started.',
    steps: {
      welcome: {
        title: 'Welcome to Qtestframe',
        body: 'This short tour shows where things are. Use Next and Back to move through it; Escape or Skip tour ends it at any time. You can restart it later from the Help menu.',
      },
      navigation: {
        title: 'Navigation',
        body: 'The sidebar is your main navigation. It lists the workspace pages and, once a project is open, the sections of that project. On smaller screens it opens from the menu button.',
      },
      projects: {
        title: 'Projects',
        body: 'Everything lives in a project: requirements, tests, runs and documents. Open Projects to pick one or to create a new one with its own key, such as SHOP.',
      },
      requirements: {
        title: 'Requirements',
        body: 'Write requirements with MUST, SHOULD and the other RFC 2119 keywords. The live language analysis flags vague wording, and every new requirement can get a linked test. You find it in every project.',
      },
      tests: {
        title: 'Tests',
        body: 'Tests are manual (step by step) or automated (a script that runs on the server). Organise them in folders and classify them with categories.',
      },
      operations: {
        title: 'Operations',
        body: 'Under Operations you create test runs, execute them step by step, record defects and import results from CI pipelines.',
      },
      search: {
        title: 'Search',
        body: 'Search finds requirements, tests and runs in all projects. It understands the query language, for example status:approved priority=high.',
      },
      profile: {
        title: 'Profile, theme and language',
        body: 'Your user menu holds your profile and password, the language and sign out. The buttons next to it switch the colour theme and between light and dark mode.',
      },
      help: {
        title: 'Help is always here',
        body: 'The Help menu opens the user guide, with a tutorial and an FAQ, and restarts this tour. Many pages also have a ? link to their part of the guide.',
      },
    },
  },
}

export default guideEn
export type GuideMessages = typeof guideEn
