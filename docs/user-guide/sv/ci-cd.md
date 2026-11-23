# CI/CD

Qtestframe samarbetar med din **CI/CD-pipeline** (i första hand GitLab CI/CD, men också GitHub Actions och andra CI-system): pipelinen laddar upp sina automatiserade testresultat, en **kvalitetsgrind** avgör om ändringen får gå vidare, resultatet kommer tillbaka till GitLab som en **commit-status** och en **anteckning i merge requesten**, och **testurvalet** kör bara de testfall en ändring behöver. Allt görs med kommandot `qtestframe` i pipelinens jobb och en API-nyckel; servern behöver ingen extra konfiguration.

Sidan bygger på [testkörningar och CI-importen](runs.md), [releaser och deras slutkriterier](releases-and-risks.md) och [kodlänkar, byggen och regressionsurvalet](variants-and-code.md#kodlänkar).

## Koppla en pipeline

Öppna **Inställningar → Projekt** och bläddra ned till **CI-uppsättning**. Där visas, för det här projektet och den här servern:

1. **En API-nyckel.** Skapa en med rollen **testare** under **Administration → API-nycklar**, helst för en egen CI-användare som är medlem i projektet med rollen testare. En nyckel får aldrig mer än användarens egen roll. Se [API-nycklar](administration.md#api-nycklar).
2. **CI/CD-variablerna** att lägga in i GitLab under *Settings → CI/CD → Variables*:

   | Variabel | Värde |
   |---|---|
   | `QTF_URL` | serverns adress (redan ifylld i mallarna) |
   | `QTF_PROJECT` | projektnyckeln (redan ifylld i mallarna) |
   | `QTF_TOKEN` | API-nyckeln; markera den som **masked** (och **protected** om bara skyddade grenar rapporterar) |
   | `QTF_CLI_IMAGE` | en containeravbildning som innehåller programmet `qtestframe` |
   | `QTF_GITLAB_TOKEN` | valfri: en projektåtkomsttoken i GitLab med rollen *Developer* och behörigheten `api`, för commit-statusar och anteckningar från pipelinen (masked) |
   | `QTF_RELEASE` | valfri: den release vars slutkriterier är grinden |

3. **En mall.** Välj en och **Kopiera** eller **Ladda ned** den:
   - **GitLab**: dina egna testjobb skriver JUnit XML till `qtf-results/`; ett sista jobb laddar upp den och kontrollerar grinden.
   - **GitLab med testurval**: ett första jobb väljer ändringens testfall och startar en genererad underpipeline som kör dem (se [Testurval](#testurval)).
   - **GitLab-komponent**: samma sak som en [CI/CD-komponent](https://docs.gitlab.com/ee/ci/components/) med indata, för ett komponentprojekt.
   - **GitHub Actions**: ett arbetsflöde som gör detsamma.

   I stället för att kopiera kan en `.gitlab-ci.yml` inkludera mallen från servern: `include: - remote: 'https://<server>/api/v1/ci/templates/gitlab-ci.yml'`. Samma filer finns i källkoden under `examples/ci/`.

Alla som kan läsa projektet ser CI-uppsättningen; nycklar och tokens visas aldrig där.

## Namnge automatiserade testfall efter nyckeln

Qtestframe kopplar ett automatiserat resultat till ett testfall genom dess **testnyckel**, till exempel `SHOP-TC-12`. Lägg in nyckeln i testet (eller [generera testkoden](#generera-testkod) med nyckeln redan på plats):

- i **testnamnet** eller **klassnamnet** i JUnit-rapporten, till exempel `test_SHOP_TC_12_checkout` eller `SHOP-TC-12 kassa`. Understreck står för bindestreck, eftersom de flesta språk inte tillåter bindestreck i namn; nyckeln måste skrivas ut helt och med versaler (`ShopTc12` känns inte igen);
- eller som en **egenskap** (property) på testfallet: `qtestframe.key`, `test_key` eller `qtestframe`.

Samma identifierare är det testurvalet filtrerar på (`SHOP_TC_12`). Resultat utan nyckel kan skapa automatiserade testfall i utkast med `--create-missing` (se [körningar](runs.md)). För att dela upp testfallen på jobben i en genererad underpipeline taggar du dem `runner:pytest`, `runner:go`, `runner:playwright` eller `runner:maven` (eller ett eget namn); utan en sådan tagg hamnar testfall med skriptspråket *playwright* i `playwright` och *python* i `pytest`, övriga i `default`.

## Generera testkod

I stället för att namnge testen för hand kan Qtestframe skriva en **stubbe** för varje testfall: en källfil för ditt testramverk som redan är namngiven efter nyckeln, beskriver testfallet och rapporteras som *överhoppad* (skipped) tills någon implementerar den. Resultaten hittar sedan tillbaka utan mer arbete.

**I webbklienten.** Markera testfall (eller filtrera listan) under **Testfall** och välj **Generera testkod**. Välj ramverk, testens katalog i förrådet (och paketet för Go och Java), titta på förhandsvisningen av varje fil och välj **Ladda ned zip** (eller den enda filen). Med rätt att ändra testfall kan du också kryssa i **Sätt skriptet för testfall utan skript till stubbens sökväg** (se nedan). Under *Kommandorad* visar dialogen kommandot `qtestframe stubs` som skriver samma filer.

**På kommandoraden**, i förrådet:

```
qtestframe stubs --framework pytest --query 'execution_type:automated has:!script' --out tests/
```

`--query` väljer testfallen ([frågespråket](search-and-filters.md); standard är `execution_type:automated`, `""` är alla testfall), `--ids` anger testfall med nyckel, `--package` sätter Go- eller Java-paketet (Go: katalogens namn som standard), `--dry-run` listar bara vad som skulle ändras. `QTF_URL`, `QTF_TOKEN` och `QTF_PROJECT` läses som för `report`; en token med rollen *läsare* räcker för att generera.

**Ramverk:**

| `--framework` | Fil | Nyckeln finns i | Inte implementerad | Testdata |
|---|---|---|---|---|
| `pytest` | `test_SHOP_TC_12.py` | `def test_SHOP_TC_12_checkout…()` | `@pytest.mark.skip` | `@pytest.mark.parametrize` |
| `playwright` | `SHOP-TC-12.spec.ts` | titeln, `tag: '@SHOP-TC-12'` och annoteringen `qtestframe` | `test.fixme()` | ett test per rad |
| `go` | `shop_tc_12_test.go` | `func TestSHOP_TC_12_Checkout…` | `t.Skip` | en tabell med deltester |
| `junit5` | `SHOP_TC_12Test.java` | klass- och metodnamnen, `@Tag("SHOP-TC-12")` | `@Disabled` | `@ParameterizedTest` med `@CsvSource` |
| `jest`, `vitest` | `SHOP-TC-12.test.ts` | testnamnet | `test.skip` | `test.skip.each` |
| `robot` | `SHOP-TC-12.robot` | testnamnet och taggen `SHOP-TC-12` | `Skip` | ett testfall per rad som anropar ett nyckelord |
| `cucumber` | `SHOP-TC-12.feature` | feature- och scenarionamnen, taggen `@SHOP-TC-12` | stegen saknar ännu definitioner; taggen `@qtestframe-stub` | en Scenario Outline med Examples |

Varje stubbe innehåller titeln, en länk till testfallet, kraven det verifierar, prioritet och taggar, beskrivningen, förutsättningarna och det förväntade resultatet, och **stegen som numrerade TODO-kommentarer** med data och förväntade resultat (Cucumber: som Given-, When- och Then-steg). Parametrarna i [testdatat](tests.md) blir parametrar i det parametriserade testet; platshållarna `{{namn}}` står kvar i kommentarerna (Cucumber: `<namn>`). För Playwright låter du JUnit-rapporteraren skriva ut annoteringen (`embedAnnotationsAsProperties: true`); namnet räcker också. JUnit 5 behöver `junit-jupiter-params` för datadrivna test. Cucumber har ingen markering för överhoppade test: stubbens steg saknar definitioner tills du skriver dem; i strikt läge rapporterar Cucumber dem som underkända, så uteslut stubbarna med `--tags 'not @qtestframe-stub'` tills de är implementerade (då har de inget resultat, och aldrig ett godkänt).

**Generera igen.** Varje fil börjar med ett block mellan kommentarraderna `qtestframe:begin SHOP-TC-12 pytest body=…` och `qtestframe:end`. Behåll de två raderna. När du kör `qtestframe stubs` igen:

- en fil som inte finns **skapas**;
- en fil vars kod under blocket fortfarande är den genererade stubben **genereras om** helt (nya steg, testdata eller titel tas med);
- en fil vars kod du har ändrat (implementerat) behåller den: bara **huvudblocket** skrivs om (titel, krav, förutsättningar …). Stegen i koden rörs inte igen;
- en fil utan markeringsraderna (eller med ett annat testfalls) **lämnas orörd**.

Radslut och avslutande blanksteg räknas inte som ändringar. Nedladdningen från webbklienten innehåller alltid hela stubbar: slå ihop den med ett förråd med kommandoraden i stället.

**Markera skripten.** Med `--mark-script` (eller kryssrutan i dialogen) får varje valt testfall som saknar skript skriptet `qtestframe:stub tests/test_SHOP_TC_12.py`, blir automatiserat, får ramverkets skriptspråk (python, playwright eller javascript, där det finns ett) och taggen `runner:<namn>` för [underpipelinen](#testurval) (pytest, playwright, go, maven, jest, vitest, robot, cucumber) om det inte redan har en runner-tagg. Servern kör aldrig ett sådant skript (*Kör automatiserade tester* hoppar över det): resultaten kommer från CI. Eftersom skriptet inte längre är tomt utesluter `has:!script` de här testfallen nästa gång. `--script-dir` sätter katalogen som skrivs i skriptet när `--out` inte är sökvägen i förrådet. Att markera kräver rätt att ändra testfall och syns i varje testfalls historik.

## Ladda upp resultat

```
qtestframe report --run "Pipeline $CI_PIPELINE_IID" --create-run --junit 'qtf-results/*.xml'
```

I GitLab CI, GitHub Actions och Jenkins läser kommandot CI-variablerna och registrerar pipelinen som resultatens **bygge**: dess id och nummer, commit, gren, pipelinens adress och merge requesten. Kravet visar då *verifierat i bygge* (se [Byggen](variants-and-code.md#byggen)). Vilket som helst av dem kan ersättas med `--build-id`, `--build-number`, `--commit`, `--branch`, `--pipeline-url` och `--merge-request`, eller stängas av med `--ci none`. `QTF_URL`, `QTF_TOKEN` och `QTF_PROJECT` är standardvärden för `--url`, `--token` och `--project`.

## Kvalitetsgrind

`qtestframe gate` kontrollerar om resultaten är tillräckligt bra, skriver i jobbloggen en sammanfattning av det som inte uppfylls (med länkar) och avslutar med slutkod **0** när grinden passeras, **1** när den inte gör det och **2** när den inte kan utvärderas (fel nyckel, okänd release). Ett jobb som misslyckas stoppar pipelinen.

```
qtestframe gate --run "Pipeline $CI_PIPELINE_IID" --release 1.0 --format junit --output qtf-gate.xml
```

**Vad som kontrolleras:**

- Med `--release`: releasens [slutkriterier](releases-and-risks.md) på dess omfattning, som på releasens flik *Status*. En release som riktar sig till varianter kontrolleras för varje variant (`--variant` kontrollerar en).
- Utan release: villkoren i `--require` på projektets krav (alla utom avvisade och utgångna). Utan `--require` är de: alla MUST/SHALL-krav godkända, inga underkända testfall, inga inaktuella resultat och inget krav blockerat av en defekt.
- `--require` ersätter också releasens kriterier. Det kan upprepas och tar kommaseparerade listor.
- `--run` (namn eller id) eller `--build` (CI-id eller nummer) begränsar resultaten till den körningen eller till byggets körningar; med en release till dess körningar i bygget.
- `--scope affected` kontrollerar bara de krav som merge requesten eller commiten hänvisar till (från [kodlänkarna](variants-and-code.md#kodlänkar)); en [fråga](search-and-filters.md) som `--scope "tag:kassa"` kontrollerar de krav den hittar.

**Villkor** (`TYP[:ARGUMENT][>=PROCENT]`):

| Villkor | Uppfyllt när |
|---|---|
| `must` (`mandatory_passed`) | varje MUST- och SHALL-krav har alla sina testfall godkända |
| `no_failed` | inget testfall är underkänt eller blockerat |
| `no_stale` | inget testfall har bara ett inaktuellt godkänt utfall (se [inaktualitet](administration.md#ändringspåverkan-inaktualitet-och-defekter)) |
| `no_blocking_defects` (`blocking`) | inget krav blockeras av en öppen defekt |
| `coverage>=80` | minst 80 % av kraven har testfall |
| `passed>=95` | minst 95 % av kraven har alla sina testfall godkända |
| `executed>=100` | varje exekvering i körningarna har ett utfall |
| `approved>=90` | minst 90 % av kraven är godkända, implementerade eller verifierade |
| `no_suspect` | inga misstänkta spårlänkar |
| `no_open_defects:major` | ingen öppen defekt med allvarlighet major eller högre är länkad |
| `risks_mitigated:high`, `residual_risk:medium` | releasernas riskvillkor |
| `trace_rules` (`trace_rules:warning`) | [spårbarhetsreglerna](traceability-and-coverage.md) uppfylls |
| `cycle_executed:"Sprint 3">=90` | 90 % av testcykelns planerade testfall har ett utfall |

**Undantag** (waivers) räknas som i releasestatusen: ett godkänt undantag för releasen, eller för hela projektet, accepterar sina krav, och sammanfattningen visar dem som *undantagna*.

**Utdata.** `--format text` (standard) skriver sammanfattningen; `json`, `junit` (ett testfall per villkor, så att GitLabs testrapport visar grinden) och `markdown` (även för GitHubs jobbsammanfattning) skrivs till filen i `--output`, och sammanfattningen skrivs ändå. Samma kontroll finns som `POST /api/v1/projects/{pid}/gate` (se API-beskrivningen).

## Resultat tillbaka i GitLab

Resultatet av en pipeline kan synas där utvecklarna tittar:

- en **commit-status** på commiten: `qtestframe/tests` (misslyckad när ett testfall är underkänt eller blockerat) och `qtestframe/gate` (grindens utslag), med en länk till körningen eller releasen och en kort sammanfattning;
- en **anteckning i merge requesten** med resultaten, täckningen av ändringens krav, de underkända testfallen och grindens utslag. Qtestframe håller **en** anteckning per projekt och merge request och uppdaterar den vid varje uppladdning (den hittar sin egen anteckning genom en dold markör), så att merge requesten inte fylls av upprepningar.

Det finns två sätt, och du behöver bara ett.

### Från pipelinen

Lägg till `--post-status` och `--post-comment` till `qtestframe report` och `qtestframe gate`, eller sätt CI/CD-variablerna `QTF_POST_STATUS` och `QTF_POST_COMMENT` till `true` (det gör mallarna). Kommandot läser `CI_API_V4_URL`, `CI_PROJECT_ID`, `CI_COMMIT_SHA` och `CI_MERGE_REQUEST_IID` och autentiserar med `QTF_GITLAB_TOKEN`: en projektåtkomsttoken med rollen **Developer** (commit-statusar kräver Developer) och behörigheten `api`, sparad som en maskerad variabel.

Jobbets egen token (`CI_JOB_TOKEN`) prövas när det inte finns någon `QTF_GITLAB_TOKEN`, men GitLab godtar den i dag inte för commit-statusar och anteckningar i merge requests; jobbloggen säger det då. En misslyckad rapport är en varning och får inte jobbet att misslyckas.

### Från servern

Med rollen **Manager** öppnar du **Inställningar → Projekt → Resultat tillbaka till GitLab**:

1. Kryssa i **Skicka commit-statusar och anteckningar i merge requests till GitLab**.
2. Ange **GitLabs API-adress** (tom för gitlab.com), **GitLab-projektet** (dess numeriska id eller `grupp/projekt`) och en **åtkomsttoken** (en projektåtkomsttoken med rollen Developer och behörigheten `api`). Token lagras krypterad och visas aldrig igen.
3. Välj vad som ska skickas och, för `qtestframe/gate`, en **release för grinden** eller **villkor för grinden** (till exempel `must, no_failed`). Utan något av dem skickas bara `qtestframe/tests`.

Från och med då skickar servern statusen och anteckningen i bakgrunden när resultat kommer in med en commit (som `qtestframe report` skickar dem i en pipeline). När GitLab inte svarar försöker den igen efter 1, 2, 4, 8 och 16 minuter; en token som GitLab avvisar försöks inte igen. **Senaste rapporter** i samma kort visar vad som skickades, eller varför inte. Merge requesten är den som pipelinen anger, annars grenens öppna merge request (från [webhookarna](variants-and-code.md#webhookar)).

## Testurval

`qtestframe select` svarar på frågan "vilka testfall behöver den här ändringen?":

```
pytest -k "$(qtestframe select --format pytest)"
```

Det samlar in **ändringen**: i en pipeline för en merge request commitarna mellan `CI_MERGE_REQUEST_DIFF_BASE_SHA` och `CI_COMMIT_SHA` (med `git log`), filerna de ändrar (`git diff`) och merge requesten; annars commitarna sedan `--since REV`, eller `--commits` och `--files` angivna för hand. Sedan väljer det:

- de krav och testfall som commitarna och merge requesten är länkade till ([kodlänkar](variants-and-code.md#kodlänkar)), och nycklarna som nämns i commit-meddelandena (`SHOP-REQ-4`, `REQ-4`, `TC-7`);
- testfallen i en **sökvägskarta**: en fil `qtestframe-map.yml` i förrådet som kopplar filmönster till testfall, taggar och krav (ett exempel finns i `examples/ci/qtestframe-map.yml`):

  ```yaml
  - paths: ["src/cart/**", "api/cart.go"]
    tests: [SHOP-TC-3]
    tags: [cart]
  - paths: ["src/payment/"]
    requirements: [SHOP-REQ-4]
  ```

- [regressionsurvalet](variants-and-code.md#regressionsurval) av dem: testfallen som verifierar kraven, krav med `--include-related` och förutsättande testfall;
- testfallen med en tagg som anges med `--tag` (till exempel `--tag always-run` för röktester);
- med `--variant` bara det som gäller varianten.

Bara **automatiserade** testfall väljs (manuella räknas i loggen; `--include-manual` tar med dem). När ändringen är okänd (inga commits, filer, nycklar eller merge request) väljs bara de taggade testfallen, om inte `--fallback all`. Okända nycklar listas.

**Utdata** (`--format`):

| Format | Utdata | Användning |
|---|---|---|
| `keys` | en nyckel per rad | egna skript |
| `pytest` | `SHOP_TC_3 or SHOP_TC_7` | `pytest -k "…"` |
| `go` | `(SHOP_TC_3\|SHOP_TC_7)([^0-9]\|$)` | `go test -run '…'` |
| `playwright` | `(SHOP[-_]TC[-_]3\|…)(?![0-9])` | `npx playwright test --grep '…'` |
| `maven` | `*SHOP_TC_3*,*SHOP_TC_7*` | `mvn test -Dtest='…'` (träffar också `SHOP_TC_31`; hellre fler än färre) |
| `json` | urvalet med skälen | |
| `gitlab-child-pipeline` | en underpipeline | se nedan |

Ett tomt urval skriver ingenting: kör då inte testerna, eftersom ett tomt filter väljer alla testfall i de flesta testkörare.

**En genererad underpipeline.** `--format gitlab-child-pipeline --output qtestframe-pipeline.yml` skriver en pipeline med ett jobb per testkörargrupp, vart och ett med de valda testfallen i `$QTF_TESTS` och sitt filter i `$QTF_FILTER`, och ett sista jobb som kör `qtestframe report` och `qtestframe gate` även när testfall misslyckades. Ett föräldrajobb sparar filen som en artefakt och startar den med `trigger: include: - artifact: qtestframe-pipeline.yml`; mallen *GitLab med testurval* gör det. Ange varje testkörares avbildning och kommando med `--image pytest=python:3.13` och `--job pytest='pytest -k "$QTF_FILTER" --junitxml="$QTF_RESULTS/pytest.xml"'`; standardvärdena använder pytest, gotestsum, Playwright och Maven. `--create-run --run NAMN` skapar urvalets körning först, så att resultaten hamnar i den; `--gate-release` och `--gate-require` ställer in grinden i slutet.

Urvalet finns också som `POST /api/v1/projects/{pid}/ci/select`.

## GitHub Actions och andra CI-system

- **GitHub Actions**: mallen använder arbetsflödets `GITHUB_TOKEN` (med `statuses: write` och `pull-requests: write`) som `QTF_GITHUB_TOKEN`; `--post-status` och `--post-comment` skickar då en commit-status och en kommentar i pull requesten som hålls aktuell. `--format markdown --output "$GITHUB_STEP_SUMMARY"` visar grinden i jobbsammanfattningen.
- **Jenkins** och andra: `qtestframe report` läser Jenkins variabler (`BUILD_TAG`, `BUILD_NUMBER`, `GIT_COMMIT`, `BRANCH_NAME`, `CHANGE_ID`); på andra ställen sätter du `QTF_BUILD_ID`, `QTF_BUILD_NUMBER`, `QTF_COMMIT`, `QTF_BRANCH`, `QTF_MERGE_REQUEST` eller anger flaggorna. `gate` och `select` fungerar överallt; att skicka statusar fungerar för GitLab och GitHub.

## Begränsningar

- GitLabs jobbtoken (`CI_JOB_TOKEN`) kan inte skicka commit-statusar eller anteckningar i merge requests: använd en projektåtkomsttoken, eller låt servern skicka.
- Servern skickar bara till GitLab; statusar i GitHub kommer från pipelinen.
- Testurvalet är aldrig bättre än länkarna: commits och merge requests blir kända genom [webhookarna](variants-and-code.md#webhookar) eller nycklarna i commit-meddelandena, och filer bara genom sökvägskartan.
- Genererade stubbar använder mallar som är inbyggda i servern; de kan ännu inte anpassas per organisation. Projektets konfigurationer blir inte parametrar: kör stubbarna en gång per konfiguration i CI (`qtestframe report --configuration`).
- API-nycklar begränsas av roller, inte av finare behörigheter: ge CI-användaren rollen testare, eller en egen roll med bara `project.read`, `test.read`, `run.read`, `run.execute` och `run.write`.
