// Svenska texter för användarhandboken i appen och introduktionsturen
// (samma nycklar som guide.en.ts).
import type { GuideMessages } from './guide.en'

const guideSv: GuideMessages = {
  titles: {
    help: 'Hjälp',
  },
  guide: {
    help: 'Hjälp',
    helpMenu: 'Hjälpmeny',
    userGuide: 'Användarhandbok',
    restartTour: 'Starta introduktionsturen igen',
    thisPage: 'Hjälp för den här sidan',
    contextHelp: 'Hjälp: {topic}',
    contents: 'Innehåll',
    contentsNav: 'Handbokens innehåll',
    onThisPage: 'På den här sidan',
    openContents: 'Visa innehåll',
    search: 'Sök i handboken',
    searchPlaceholder: 'Sök i handboken …',
    searchResults: 'Inga träffar | 1 träff | {count} träffar',
    searchNothing: 'Inget hittades för ”{query}”.',
    clearSearch: 'Rensa sökningen',
    pager: 'Föregående och nästa sida',
    previous: 'Föregående: {title}',
    next: 'Nästa: {title}',
    fallback: 'Sidan finns ännu inte på ditt språk och visas därför på engelska.',
    notFound: 'Den här sidan finns inte i handboken.',
    backToIndex: 'Gå till handbokens översikt',
    table: 'Tabell (rullar i sidled)',
    missingImage: 'Skärmbilden är inte tillgänglig: {alt}',
  },
  tour: {
    dialog: 'Introduktion till Qtestframe',
    stepOf: 'Steg {n} av {total}',
    next: 'Nästa',
    back: 'Tillbaka',
    skip: 'Hoppa över turen',
    finish: 'Klar',
    started: 'Introduktionsturen har startat.',
    steps: {
      welcome: {
        title: 'Välkommen till Qtestframe',
        body: 'Den här korta turen visar var saker finns. Gå framåt och bakåt med Nästa och Tillbaka; Escape eller Hoppa över turen avslutar den när som helst. Du kan starta den igen från Hjälpmenyn.',
      },
      navigation: {
        title: 'Navigering',
        body: 'Sidopanelen är huvudnavigeringen. Där finns arbetsytans sidor och, när ett projekt är öppet, projektets delar. På mindre skärmar öppnas den med menyknappen.',
      },
      projects: {
        title: 'Projekt',
        body: 'Allt finns i ett projekt: krav, testfall, körningar och dokument. Öppna Projekt för att välja ett eller skapa ett nytt med en egen nyckel, till exempel SHOP.',
      },
      requirements: {
        title: 'Krav',
        body: 'Skriv krav med MUST, SHOULD och de andra kravorden enligt RFC 2119. Den löpande språkanalysen markerar vaga formuleringar, och varje nytt krav kan få ett länkat testfall. Den finns i varje projekt.',
      },
      tests: {
        title: 'Testfall',
        body: 'Testfall är manuella (steg för steg) eller automatiserade (ett skript som körs på servern). Ordna dem i mappar och klassificera dem med kategorier.',
      },
      operations: {
        title: 'Testkörningar',
        body: 'Under Testkörningar skapar du körningar, exekverar dem steg för steg, registrerar defekter och importerar utfall från CI-pipelines.',
      },
      search: {
        title: 'Sök',
        body: 'Sök hittar krav, testfall och körningar i alla projekt. Sökningen förstår frågespråket, till exempel status:approved priority=high.',
      },
      profile: {
        title: 'Profil, tema och språk',
        body: 'I användarmenyn finns din profil och ditt lösenord, språkvalet och utloggningen. Knapparna bredvid byter färgtema och växlar mellan ljust och mörkt läge.',
      },
      help: {
        title: 'Hjälpen finns alltid här',
        body: 'Hjälpmenyn öppnar användarhandboken, med en steg-för-steg-handledning och vanliga frågor, och startar den här turen igen. Många sidor har också en ?-länk till sin del av handboken.',
      },
    },
  },
}

export default guideSv
