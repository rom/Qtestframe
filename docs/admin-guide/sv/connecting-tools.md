# Koppla ihop med andra verktyg (OSLC och ReqIF)

Qtestframe utbyter krav och testfall med ALM- och kravverktyg på två sätt (användarhandbokens sida *Samverkan* beskriver vad användarna ser):

- **OSLC**, direkt: Qtestframe är en OSLC-leverantör för Requirements Management 2.1 (krav) och Quality Management 2.1 (testfall). Andra verktyg hittar den, ställer frågor, skapar, ändrar och tar bort poster, länkar sina egna poster till dem och visar Qtestframes urvals- och skapadialoger för sina användare.
- **ReqIF-filer**, för hela specifikationer: export från ett verktyg, import i det andra, gång på gång. Inget behöver ställas in på servern.

Det här kapitlet handlar om att sätta upp OSLC-kopplingar till IBM Engineering Lifecycle Management (DOORS Next, Engineering Test Management), Siemens Polarion och PTC Codebeamer, och om vad du kontrollerar när en koppling inte fungerar.

## Det här erbjuder servern

| Adress | Användning |
|---|---|
| `<base_url>/api/v1/oslc/rootservices` | Upptäcktsdokument i Jazz-stil (öppet, innehåller inga data). IBM ELM och verktyg som använder "friends" börjar här. |
| `<base_url>/api/v1/oslc/catalog` | Katalog över tjänsteleverantörer: en per projekt som kontot får läsa. |
| `<base_url>/api/v1/oslc/projects/<projekt-id>/catalog` | En katalog med bara ett projekt, för verktyg som kopplar ett av sina projekt till ett projekt här. |
| `<base_url>/api/v1/oslc/projects/<projekt-id>` | Projektets tjänsteleverantör: frågefunktioner, creation factories, urvals- och skapadialoger, resource shapes. |

Projekt-id:t är det i projektets webbadress (`/p/<id>/…`). Resurser skickas som RDF/XML (OSLC:s standard), Turtle eller JSON-LD; innehåll i begäranden tas emot i samma format. API-referensen (`docs/api.md`, avsnittet *OSLC*) listar alla egenskaper, länkegenskaperna och frågesyntaxen.

## Innan du kopplar in ett verktyg

1. **Bas-URL.** Ställ in `server.base_url` (eller **Bas-URL** i *Systeminställningar*) på adressen som det andra verktyget når Qtestframe på, till exempel `https://qtf.example.com`. Varje OSLC-resursadress byggs av den; bakom en omvänd proxy måste det vara den publika adressen, inte den interna.
2. **TLS.** De flesta ALM-servrar vägrar vanlig HTTP och certifikat som de inte litar på. Använd ett certifikat från en CA som det andra verktygets Java-förtroendelager känner till (eller lägg till din CA där).
3. **Ett tjänstekonto och en API-nyckel.** OSLC-klienter loggar in med en API-nyckel, antingen som `Authorization: Bearer <nyckel>` eller som lösenord i Basic-autentisering (valfritt användarnamn), bara på OSLC-sökvägarna. OAuth 1.0a, som IBM ELM använder mellan Jazz-program, stöds inte; använd anslutningstypen "custom" eller "basic" i det andra verktyget med nyckeln. Skapa en användare (till exempel `alm-connector@example.com`) och ge den rollen som kopplingen behöver:
   - **Läsare** för att låta verktyget läsa och länka (länken sparas i det andra verktyget; Qtestframe visar den där).
   - En roll med `requirement.write` och/eller `test.write` för att låta verktyget skapa och ändra poster, och `test.write` för att ändra spårlänkar (*validatedBy* / *validatesRequirement*).
   Logga in som den användaren, skapa nyckeln under **Administration → API-nycklar** med rollen som kopplingen behöver (en nyckel kan ha lägre roll än sitt konto, aldrig högre) och ge den till administratören för det andra verktyget. Begränsade projekt syns bara när kontot är medlem.
4. **Dialoger i en ram** (valfritt). Urvals- och skapadialogerna öppnas i ett eget fönster, vilket fungerar överallt. Verktyg som visar dem inne på sin egen sida (en iframe) behöver två saker:
   - verktygets ursprung i `server.frame_ancestors`, till exempel
     ```yaml
     server:
       base_url: https://qtf.example.com
       frame_ancestors:
         - https://elm.example.com
         - https://polarion.example.com
     ```
     Dialogsidorna skickar då `Content-Security-Policy: frame-ancestors 'self' https://elm.example.com …`; utan inställningen får bara Qtestframe självt visa dem i en ram.
   - webbläsaren måste skicka Qtestframes sessionskaka inne i ramen. Kakan är `SameSite=Lax`, så ramen fungerar när båda verktygen ligger på **samma webbplats** (underdomäner till samma registrerbara domän, som `qtf.example.com` och `elm.example.com`). För verktyg på en annan webbplats låter du dialogerna öppnas i ett fönster.
   Dialogerna använder användarens egen session: varje användare loggar in i Qtestframe i webbläsaren (dialogen visar en länk om de inte har gjort det).

## IBM DOORS Next och Engineering Test Management

DOORS Next länkar sina krav till testfall hos en QM-leverantör, och ETM länkar sina testfall till krav hos en RM-leverantör. Qtestframe kan ha RM-rollen (sina krav) och QM-rollen (sina testfall).

1. Lägg till en association i ELM:s **Lifecycle Project Administration** (eller projektområdets **Associations**) till en **OSLC-leverantör som inte är ett Jazz-program**: välj *Add → Service provider catalog* (ordalydelsen varierar mellan versioner) och ange `<base_url>/api/v1/oslc/catalog`, eller `<base_url>/api/v1/oslc/projects/<id>/catalog` för ett projekt. När versionen bara tar emot friends registrerar du en friend med rotdokumentet `<base_url>/api/v1/oslc/rootservices` och väljer Basic-autentisering med tjänstekontots e-postadress och API-nyckeln som lösenord.
2. Välj associationstyp: **Provides Requirements** (eller *Uses Requirements* från ETM-sidan) för krav i Qtestframe, **Provides Test Cases** (*Validated By*) för testfall i Qtestframe.
3. I en artefakt i DOORS Next erbjuder **Links → Add link → Validated By** nu *Qtestframe* som mål: urvalsdialogen öppnas, användaren söker och markerar testfall, och DOORS Next sparar länkarna. **Create** öppnar Qtestframes skapadialog i stället.

Länkar som DOORS Next sparar finns i DOORS Next. När ett verktyg också skriver länken i Qtestframe-resursen (en PUT med `oslc_qm:validatesRequirement` eller `oslc_rm:validatedBy`) behåller Qtestframe den som en spårlänk när den andra änden är en av dess egna poster, annars som en länk till det andra verktygets adress.

## Siemens Polarion

Polarion länkar arbetsposter till OSLC-resurser på andra servrar genom *OSLC-anslutningar*:

1. Lägg till en leverantör i **Administration → Linked data / OSLC → Remote OSLC providers** (Polarions konfiguration av *Connections*) med katalogen `<base_url>/api/v1/oslc/catalog`, autentisering **Basic**, tjänstekontots e-postadress som användare och API-nyckeln som lösenord.
2. Mappa länkrollerna: ett krav i Polarion *is validated by* → testfall i Qtestframe (`oslc_qm:TestCase`), ett testfall i Polarion *verifies* → krav i Qtestframe (`oslc_rm:Requirement`).
3. Polarions användare väljer sedan poster i Qtestframe i dialogen **Link to OSLC resource**, som öppnar Qtestframes urvalsdialog.

För en enstaka eller regelbunden överföring av ett Polarion-dokument använder du ReqIF i stället: exportera dokumentet som ReqIF i Polarion och importera det i Qtestframe med *Importera → ReqIF … med uppdateringar*; mappa arbetsposttyperna (till exempel *Test Case* till testfall) i förhandsgranskningen av importen.

## PTC Codebeamer

Codebeamer ansluter till OSLC-leverantörer som *Remote trackers / OSLC associations*:

1. Lägg till en server i projektets **Admin → Integrations → OSLC** med rotdokumentets eller katalogens adress ovan, autentisering **Basic** med tjänstekontot och API-nyckeln.
2. Välj fjärrtjänst: *Requirements* (krav i Qtestframe) eller *Test cases* (testfall i Qtestframe), och vilken associationstyp i Codebeamer den ska skapa (till exempel *verifies* eller *related*).
3. Codebeamers associationsdialog listar sedan poster i Qtestframe; de delegerade dialogerna används där Codebeamer stöder dem.

Codebeamer exporterar och importerar också ReqIF; dess trackers av typen *Test Case* mappas till testfall i förhandsgranskningen av importen.

## Andra OSLC-klienter och skript

Alla OSLC 2.0-klienter fungerar med katalogen och en nyckel. En snabb kontroll från kommandoraden:

```sh
curl -H "Authorization: Bearer $QTF_TOKEN" -H "Accept: text/turtle" \
  "https://qtf.example.com/api/v1/oslc/projects/<id>/requirements?oslc.where=qtf:status=%22approved%22&oslc.select=dcterms:title"
```

Skapa ett krav:

```sh
curl -H "Authorization: Bearer $QTF_TOKEN" -H "Content-Type: text/turtle" -H "Accept: text/turtle" \
  --data-binary '<> a <http://open-services.net/ns/rm#Requirement> ; <http://purl.org/dc/terms/title> "Nödstopp" .' \
  "https://qtf.example.com/api/v1/oslc/projects/<id>/requirements"
```

Svaret är `201 Created` med den nya adressen i `Location` och versionen i `ETag`. En uppdatering (PUT) med `If-Match: "<ETag>"` nekas med `412 Precondition Failed` om någon har sparat under tiden, som i webbklienten.

## Det här sparas, loggas och säkerhetskopieras

- Poster som skapas, ändras eller tas bort via OSLC är vanliga poster: de har historik, ändringarna står i granskningsloggen under nyckelns konto, och borttagning flyttar dem till projektets papperskorg.
- Spårlänkar, relationer och länkar som skrivs via OSLC är vanliga länkar; länkar till resurser i andra verktyg är länkar till deras URL, med OSLC-egenskapen som etikett (`oslc_rm:implementedBy` …).
- ReqIF-importen sparar var varje post, relation och spårlänk kom ifrån (tabellen `external_refs`), så att en ny import av samma dokument uppdaterar dem. Tabellen ingår i säkerhetskopior och projektarkiv.
- Inget om en koppling sparas i Qtestframe utöver API-nyckeln: det andra verktyget har kopplingens inställningar.

## Felsökning

| Symptom | Kontrollera |
|---|---|
| Verktyget säger att katalogen är tom | Kontot är inte medlem i projektet (begränsade projekt), eller nyckelns roll saknar läsbehörighet. Öppna katalogadressen med nyckeln i en webbläsare eller med `curl`. |
| Länkar pekar på `http://localhost:…` eller en intern värd | `server.base_url` är inte den publika adressen. |
| 401 Unauthorized | Nyckeln är fel, har gått ut eller har återkallats; Basic-autentisering måste ha nyckeln som lösenord. Vanliga lösenord tas inte emot på OSLC-sökvägarna. |
| 403 Forbidden vid skapa eller ändra | Nyckelns roll saknar `requirement.write` / `test.write`, eller `test.write` för spårlänkar. |
| 412 Precondition Failed | Posten har ändrats sedan verktyget läste den; verktyget måste läsa den igen (dess ETag är postens version). |
| 415 Unsupported Media Type | Verktyget skickar innehåll som inte är Turtle, RDF/XML eller JSON-LD. |
| Dialogen förblir tom i en ram | Verktygets ursprung finns inte i `server.frame_ancestors`, eller verktygen ligger på olika webbplatser så att webbläsaren inte skickar sessionskakan: använd dialogen i ett fönster. |
| Dialogen säger *Du är inte inloggad* | Användaren har ingen session i Qtestframe i den här webbläsaren: logga in i en annan flik och välj **Försök igen**. |
