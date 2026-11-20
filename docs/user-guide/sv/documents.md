# Dokument

Varje projekt har ett **dokumentarkiv**: en samlad plats för alla filer i projektet. Det innehåller

- **dokument** som du laddar upp direkt till arkivet, till exempel specifikationer, avtal, testplaner eller mötesanteckningar;
- **bilagor** som har laddats upp till ett krav, ett testfall, en körning eller en exekvering;
- **rapporter** som har tagits fram och sparats i arkivet (se [Rapporter](reports-and-export.md#standardrapporter)).

Välj **Dokument** i projektets sidopanel. Alla som kan läsa projektet kan se och ladda ned dess filer.

![Dokumentarkivet med filer, deras typ, storlek, uppladdare och länkade objekt](../images/sv-documents.png)

## Hitta filer

- Tabellen visar varje fils **namn**, **typ** (dokument, bilaga eller rapport), **storlek**, vem som laddade upp den, när, dess taggar och de objekt den är **Kopplad till**.
- Filtrera på **Typ**, eller med en [fråga](search-and-filters.md) på fälten `name`, `description`, `tag`, `kind`, `type` (innehållstyp), `uploaded_by`, `created_at`, `size` och `linked`:

```text
kind=report                                    bara framtagna rapporter
type:pdf created_at>=2026-09-01                PDF-filer sedan september
linked:SHOP-REQ-12                             filer som är bifogade till SHOP-REQ-12
tag:avtal -kind=attachment                     avtalsdokument
size>5000000                                   filer större än ungefär 5 MB
```

## Ladda upp

1. Välj **Ladda upp dokument**, eller dra filer från datorn till sidan.
2. Flera filer laddas upp efter varandra; en förloppslista visar varje fil och rapporterar fel (till exempel en fil som är för stor).
3. Lägg till en beskrivning och taggar, åtskilda med kommatecken, så att filerna blir lättare att hitta. Du kan ändra dem senare med **Redigera**.

Den största tillåtna storleken är 25 MB per fil om administratören inte har angett en annan gräns. Att ladda upp dokument till arkivet kräver rollen **Testare** eller **Ansvarig** (rätten att redigera testfall eller krav).

## Förhandsvisa och ladda ned

- **Förhandsvisa** visar bilder (PNG, JPEG, GIF, WebP), PDF-filer och vanlig text direkt i webbläsaren.
- **Ladda ner** sparar filen. Andra filtyper laddas alltid ned i stället för att visas, av säkerhetsskäl.

## Arbeta med många filer på en gång

Kryssa i filer i listan (rubrikens kryssruta markerar hela sidan). Verktygsfältet erbjuder:

- **Ladda ner som ZIP**: ett arkiv med alla markerade filer (högst 512 MB tillsammans). Filer med samma namn numreras, till exempel *plan (2).pdf*. Alla som kan se arkivet kan ladda ner.
- **Lägg till taggar** / **Ta bort taggar** för alla markerade filer.
- **Beskrivning**: ersätter beskrivningen av varje fil (tomt rensar den).
- **Ta bort**, efter bekräftelse.

Ändringar och borttagning gäller de filer du får ändra: dina egna uppladdningar, eller alla filer om du har behörighet att ändra projektet. Om urvalet också innehåller andra filer visar verktygsfältet hur många av dem du kan ändra, och bara de ändras. Ändringar av taggar och beskrivning kan ångras med ångra-knappen.

## Länka dokument till objekt

En fil i arkivet kan bifogas till hur många krav, testfall, körningar och exekveringar som helst i samma projekt, utan att kopieras. På fliken **Bilagor** för ett objekt väljer du **Bifoga från arkivet** och väljer filerna. Arkivet listar sedan objektet under **Kopplad till**.

- **Att ta bort en bilaga** från ett objekt tar bara bort länken.
- En fil av typen *bilaga* som inte längre är länkad till något tas bort automatiskt; *dokument* och *rapporter* finns kvar i arkivet.
- **Att ta bort en fil** i arkivet tar bort den tillsammans med alla dess länkar, efter en bekräftelse som namnger de länkade objekten.

Den som laddade upp en fil samt projektets ansvariga och administratörer får redigera och ta bort den.

## Var lagras filerna?

Filerna lagras i Qtestframes databas, inte på en separat filserver. Det betyder att de ingår i varje [säkerhetskopia](administration.md#säkerhetskopiering-och-återställning), skyddas av samma behörighetsregler som projektet och tas bort tillsammans med projektet. Varje nedladdning kontrolleras mot dina behörigheter.
