# Första start

## Vad som händer vid start

`qtestframe serve` gör i den här ordningen:

1. läser in och kontrollerar konfigurationen (en ogiltig konfiguration stoppar starten, med alla fel listade);
2. öppnar databasen och tillämpar de [migreringar](backup-and-migration.md#migreringar) som återstår;
3. öppnar nyckelleverantören för de lagrade hemligheterna, och krypterar hemligheter som en tidigare version lagrat i klartext (en saknad eller fel nyckel stoppar starten här);
4. skapar **bootstrap-administratören** om databasen saknar användare;
5. tillämpar systeminställningarna som sparats i databasen;
6. öppnar lyssnaren och TLS-certifikatet, och **låser sedan processen** (no_new_privs, capabilities, Landlock, seccomp) och loggar en rad `hardening`;
7. startar bakgrundsjobben och loggar `qtestframe listening` med adress, TLS, version och databasdrivrutin.

## Den första administratören

Det finns två sätt att få det första kontot.

**Bootstrap i konfigurationen.** Så länge databasen **inte har några användare alls** skapar servern en användare från avsnittet `bootstrap`, gör den till **superadministratör** och **administratör** i organisationen `bootstrap.tenant_slug` (skapas om den saknas, standard `default` / *Default*):

```yaml
bootstrap:
  tenant_slug: acme
  tenant_name: "Acme AB"
  admin_email: admin@acme.example
  admin_password_file: /etc/qtestframe/secrets/admin-password
```

Lösenordet måste ha minst 10 tecken med minst en bokstav och ett tecken som inte är en bokstav; ett svagare stoppar starten. När någon användare finns ignoreras avsnittet, så det kan stå kvar i filen, men ta bort lösenordsfilen när administratören har loggat in och bytt lösenord. Utan bootstrap-inställningar loggar servern `no users exist; configure bootstrap.admin_email/admin_password or run qtestframe user create`.

**På kommandoraden**, som tjänsteanvändaren:

```sh
alias qtf='sudo -u qtestframe env QTF_CONFIG=/etc/qtestframe/config.yaml /usr/bin/qtestframe'
read -rs PW && printf '%s\n' "$PW" | qtf user create --email admin@acme.example --name "Admin" --password-stdin --super-admin
qtf tenant create --slug acme --name "Acme AB"
qtf member add --tenant acme --email admin@acme.example --role admin
```

Lösenordet läses från standard input, aldrig från kommandoraden (där `ps` skulle visa det). En användare som skapas utan `--password-stdin` har inget lösenord och kan bara logga in med single sign-on.

## Organisationer

En **organisation** (tenant) är en isolerad arbetsyta: projekt, medlemmar, roller, API-token, SCIM-token, MFA-policy och granskningslogg hör till en organisation och syns inte för de andra. Ett användarkonto hör till installationen och kan vara medlem i flera organisationer, med en roll i varje; användaren växlar mellan dem i webbklienten.

- Skapa organisationer under **Administration → Organisationer** (superadministratörer) eller med `qtestframe tenant create --slug S --name N`. Slug är 2–63 tecken av `a-z`, `0-9` och `-`; användare kan skriva den vid inloggningen.
- Ge personer roller under **Administration → Medlemmar** (organisationsadministratörer) eller med `qtestframe member add`.
- Att ta bort en organisation tar bort alla dess projekt och data och kan inte ångras (bara en [återställning](backup-and-migration.md#återställning) tar tillbaka den).

Single sign-on kan lägga nya användare direkt i en organisation (`default_tenant`, `role_mapping`, se [Autentisering](authentication.md#provisionering-av-användare-med-single-sign-on)), och SCIM provisionerar medlemmarna i en organisation per token.

## Demodata

```sh
qtf seed-demo                 # i bootstrap.tenant_slug
qtf seed-demo --tenant acme
```

`seed-demo` lägger till exempelprojektet *Demo – Online Banking* (nyckeln `DEMO`, sedan `DEMO2`, … när den redan finns) med krav, tester, länkar, körningar med historik, en release, ett demowebbtest, och ett begränsat projekt som delas med en av två demoanvändare, `dana@example.com` och `erik@example.com`, vars lösenord är `Demo-user-1234`. Kommandot kör bootstrap först, så demoanvändarna tar aldrig den första administratörens plats. Demowebbtestet öppnar serverns egen inloggningssida på `server.base_url`.

**Kör aldrig `seed-demo` mot en produktionsdatabas**: det skapar konton med ett publicerat lösenord. Använd en separat databas för provning och utbildning.

## Checklista efter första starten

- Logga in som bootstrap-administratören och **byt lösenord** (användarmenyn → **Profil och lösenord**); lägg till en autentiseringsapp.
- Sätt `server.base_url` till adressen som användarna skriver; den behövs för single sign-on och e-post.
- Säkerhetskopiera **nyckelfilen** för de lagrade hemligheterna (`/var/lib/qtestframe/qtestframe-secrets.key` på en SQLite-installation) till en plats skild från databasens säkerhetskopior. Se [Säkerhet](security.md#kryptering-av-lagrade-hemligheter).
- Bestäm om single sign-on, en [MFA-policy](authentication.md#flerfaktorsautentisering), e-post och syslog.
- Sätt upp regelbundna [säkerhetskopior](backup-and-migration.md) och prova en återställning.
