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
