# Funktioner som lagts till senare

Det här kapitlet samlar administrationsanteckningarna på serversidan för funktioner som lagts till efter att resten av handboken skrevs: konfigurationsnycklar, behörigheter, bakgrundsjobb, data som en säkerhetskopia innehåller eller inte innehåller, och utgående anslutningar. Varje funktion som behöver en sådan anteckning får ett avsnitt nedan; till dess beskriver [användarhandboken](../../user-guide/sv/administration.md) funktionens skärmar.

När en funktion lägger till konfigurationsnycklar, systeminställningar, behörigheter, webhook-händelser, tabeller eller kommandoradsflaggor tar handbokens genererade delar (konfigurationsreferensen, systeminställningarna, behörighetsmatrisen, webhook-händelserna, tabellerna i en säkerhetskopia, migreringarna, kommandoraden) med dem via `make admin-guide-ref`; bara förklaringen hör hemma här.

<!--
Platshållare för anteckningar som läggs in vid sammanslagning (ett "##"-avsnitt
per funktion, hämtat från funktionens administrationsanteckningar i
docs/user-guide/<lang>/administration.md och kontrollerat mot koden):

## Verifieringsposter
## Undantag (waivers)
## Egna fält
## OSLC
## Kod-webhooks
## Varianter
-->

## Utforskande test

Utforskande testsessioner (migrering 0040) behöver ingen konfiguration. De använder behörigheterna för testkörningar: `run.read` för att se sessioner, `run.write` för att planera, ändra och ta bort dem och för att utvärdera eller stoppa någon annans session, `run.execute` för att genomföra sin egen session (tidtagning, anteckningar, defekter från anteckningar, utvärdering). Sessionens tidtagning hålls i databasen (`exploratory_sessions.running_since`, `elapsed_seconds`), så en omstart av servern tappar ingen sessionstid; en session som lämnas igång fortsätter att räkna tills någon pausar eller stoppar den.

Tabellerna `exploratory_sessions`, `exploratory_scope` och `exploratory_notes` ingår i säkerhetskopian och i projektarkiv; skärmbilder är vanliga bilagor (`files`, `attachments`) och räknas mot `files.max_upload_mb`. Om utvärderade sessioner är verifieringsbevis är en projektinställning (`verification_settings.exploratory_evidence`, avstängd som standard) som ändras av någon med `project.update`. Varje åtgärd skrivs till händelseloggen med åtgärderna `exploratory.*`.

## CI/CD-integration

Kvalitetsgrinden, testurvalet och pipelinemallarna (migrering 0053) behöver ingen konfiguration; serverns rapporter av commit-statusar och anteckningar i merge requests till GitLab är en projektinställning med en krypterad token och en utkorg i bakgrunden (`ci_deliveries`, som minutjobbet försöker igen, och som inte säkerhetskopieras). Tokens, roller, jobbtokens begränsningar och utgående anslutningar beskrivs i [E-post, webhooks och integrationer](integrations.md); ändringar av inställningarna loggas som `project.ci_settings`.

## Testmiljöer och testdata

Testmiljöer, deras bokningar och testdatamängder (migrering 0057) behöver ingen konfiguration. Den nya behörigheten `environment.manage` (testledare, förvaltare och administratörer som standard; migreringen lägger till den i sparade testledar- och förvaltarroller som har `run.write`) skapar och ändrar miljöer, godkänner bokningar och visar en miljös hemlighet; för en miljö som delas av organisationen hämtas den från organisationsrollerna, inte från projektroller. Att boka kräver `run.execute` i projektet bokningen gäller; testdatamängder använder `test.read` och `test.write`.

En miljös valfria hemlighet krypteras som de andra sparade hemligheterna (`environments.secret`, krypteras om av `qtestframe secrets rotate`), är endast skrivbar i API:t, utelämnas ur projektarkiv, och varje visning loggas som `environment.secret_reveal`. Använd hellre hänvisningen till inloggningsuppgifterna (en sökväg i ett valv).

Minutjobbet kontrollerar miljöerna som har en adress för hälsokontroll (högst 100 per minut, 8 åt gången, 10 s tidsgräns var) med serverns utgående HTTP-klient: adresserna anges av miljöförvaltare och servern måste kunna nå dem. En misslyckad kontroll markerar en tillgänglig miljö som trasig och en lyckad gör den tillgänglig igen; statusbyten loggas som `environment.health`. Tabellerna `environments`, `environment_bookings`, `test_data_sets`, `test_data_links`, `test_data_usage` och `test_data_refresh_requests` ingår i säkerhetskopian; ett projektarkiv innehåller projektets egna miljöer med deras bokningar och dess testdatamängder (delade miljöer stannar hos organisationen). Migreringen gör om de olika fritextmiljöerna i varje projekts körningar och cykler till miljöposter och kopplar körningarna, cyklerna och deras slutliga resultat till dem. Nya webhook- och chatthändelser: `environment.status_changed`, `environment.booking_requested`, `environment.booking_approved`, `environment.booking_rejected`, `testdata.refresh_requested`, `testdata.refresh_resolved`.
