# Autentisering

Qtestframe känner till de här sätten att visa vem någon är:

| Metod | För | Konfigureras i |
|---|---|---|
| Lokalt lösenord | personer | `auth.local`, lösenordsregler |
| Andra faktor (TOTP-autentiseringsapp, återställningskoder, lösennycklar) | personer med lokalt lösenord, valfritt efter OIDC | MFA-policyn i webbklienten; `local_mfa` per OIDC-leverantör |
| Lösennyckel (WebAuthn), utan lösenord | personer med lokalt konto | `auth.passkeys` (filen eller systeminställningarna) |
| OpenID Connect | personer | `auth.oidc` (filen eller systeminställningarna) |
| SAML 2.0 | personer | `auth.saml` (bara filen) |
| Inbjudan via e-post | en persons första inloggning | kräver e-post |
| API-nyckel (`qtf_…`) | skript och CI | **Administration → API-nycklar** |
| SCIM-token (`qtfscim_…`) | en identitetsleverantörs provisionering | **Administration → Provisionering (SCIM)** |

## Lokala lösenord

- Lösenord hashas med **bcrypt** (kostnad 12). Reglerna är: minst `auth.password_min_length` tecken (standard 10, högst 72 byte, eftersom bcrypt ignorerar resten), med minst en bokstav och ett tecken som inte är en bokstav. `qtestframe user create` och bootstrap-administratören kontrolleras alltid mot 10.
- Okända användare, fel lösenord, avstängda konton och konton utan lösenord får alla samma svar (`401 invalid credentials`) och tar lika lång tid.
- **Gräns för inloggningsförsök:** högst `auth.login_max_attempts` försök (standard 10) per *klientadress + e-postadress* inom `auth.login_lockout` (standard 5 minuter); fler får `429 rate_limited`. En lyckad inloggning nollställer räknaren. Räknaren finns i minnet, per serverprocess. Bakom en omvänd proxy: sätt `server.trusted_proxies`, annars delar alla användare proxyns adress.
- Misslyckade inloggningar loggas på nivån WARN (`login failed`, med e-post och klientadress) och sparas i användarens inloggningshistorik, som webbklienten visar som "misslyckade försök sedan din senaste inloggning".
- **Stäng av lokal inloggning** med `auth.local: false` när single sign-on fungerar. I webbklienten går det bara när en single sign-on-leverantör är påslagen. Superadministratörer kan fortfarande hanteras på kommandoraden.
- Organisationsadministratörer återställer lösenord för medlemmar som bara tillhör deras organisation (**Administration → Medlemmar → Återställ lösenord**: ett tillfälligt lösenord, som visas en gång och måste bytas vid nästa inloggning); superadministratörer återställer allas under **Användare**. Båda loggar ut användaren överallt.

## Flerfaktorsautentisering

Den andra faktorn är en **TOTP**-kod från en autentiseringsapp (RFC 6238: HMAC-SHA-1, 6 siffror, 30-sekundersintervall, ±1 intervall för klockavvikelse), med **10 återställningskoder** för engångsbruk. Användarna lägger till den i sin profil. Varje kod fungerar bara en gång (det senast använda intervallet sparas), så högst en inloggning per 30 sekunder och användare.

**Policyer** bestämmer vem som måste använda den (**Administration → Tvåstegsverifiering**, se [användarhandboken](../../user-guide/sv/administration.md#tvåstegsverifiering)):

| Policy | Sätts av | Gäller |
|---|---|---|
| Serverpolicy | superadministratörer | alla användare; roller räknas i alla organisationer, plus pseudorollen *superadministratör* |
| Organisationspolicy | organisationsadministratörer (`tenant.manage`) | organisationens medlemmar efter deras roll där |

Var och en är *valfri* (standard), *krävs för alla* eller *krävs för vissa roller*. En organisation kan kräva mer än servern, aldrig mindre. En **övergångsperiod** (dagar) börjar när en policy börjar kräva MFA för fler, eller när användaren skapades om det är senare; därefter måste en användare utan andra faktor lägga till en innan något annat fungerar (`403 mfa_enrolment_required`). Policyn utvärderas vid inloggning, så en striktare policy når befintliga sessioner vid deras nästa inloggning.

**Kom ihåg den här enheten** (serverpolicy, *dagar*, standard 0 = av) låter en webbläsare hoppa över koden – aldrig lösenordet – under så många dagar. Token kan återkallas på servern; användare kan glömma sina enheter, och en administratörsåterställning eller att stänga av MFA glömmer dem också.

Gissning begränsas på tre sätt: varje kod räknas mot inloggningsgränsen ovan (som det rätta lösenordet ensamt inte nollställer), ett konto tillåter 10 kodförsök per 15 minuter oavsett adress, och en påbörjad inloggning tas bort efter 5 felaktiga koder.

**Borttappad telefon och återställningskoder:** en organisationsadministratör använder **Återställ tvåstegsverifiering** under **Medlemmar** (för medlemmar som bara tillhör den organisationen), en superadministratör under **Användare**. Det tar bort autentiseringsappen, återställningskoderna och ihågkomna enheter, loggar ut användaren och meddelar hen med avisering och e-post. Kontrollera begäran via en annan kanal först.

TOTP-hemligheterna lagras krypterade (se [Säkerhet](security.md#kryptering-av-lagrade-hemligheter)). API-nycklar och SCIM-token påverkas inte av MFA; skydda dem som lösenord. [Lösennycklar](#lösennycklar) är också en andra faktor.

## Lösennycklar

En **lösennyckel** (passkey) är en WebAuthn-autentiseringsuppgift: ett nyckelpar som användarens enhet håller (en plattformsautentiserare med fingeravtryck, ansikte eller PIN, en lösenordshanterare som synkar den, eller en säkerhetsnyckel). Qtestframe sparar bara den publika nyckeln. Lösennycklar används på tre sätt:

- **Inloggning utan lösenord** – **Logga in med lösennyckel** på inloggningssidan, och förslag på lösennycklar i e-postfältet där webbläsaren stöder det (villkorlig medling, conditional mediation). Den använder *upptäckbara* autentiseringsuppgifter och kräver **användarverifiering** (UV-flaggan: PIN eller biometri), så den räknas som flerfaktor i sig: ingen TOTP-kod efterfrågas. Bara **lokala konton** kan logga in så; konton från en identitetsleverantör loggar in där (Qtestframe låter inte en lösennyckel kringgå leverantören).
- **Andra faktor** – efter lösenordet (och efter OIDC med `local_mfa`) kan en användare med lösennycklar bekräfta med en sådan i stället för en TOTP-kod. Användarverifiering föredras men krävs inte (användarnärvaro krävs). En lösennyckel räknas som andra faktor för alla MFA-policyer, så en användare kan ha lösennycklar, en autentiseringsapp eller båda.
- **Ny bekräftelse av identiteten** – att lägga till och ta bort lösennycklar kräver en inloggning för mindre än 10 minuter sedan; annars bekräftar användaren sin identitet igen med lösenordet, en TOTP- eller återställningskod eller en lösennyckel (`POST /api/v1/auth/reauth`).

Användarna hanterar sina lösennycklar i sin profil (flera per konto, högst 25, var och en med namn, datum då den lades till och senaste användning; byt namn och ta bort). Att ta bort den sista andra faktorn för ett konto som en policy kräver MFA för nekas.

**Inställningar** (Systeminställningar → Inloggning och lösenord, eller konfigurationsfilen):

| Inställning | Standard | Betydelse |
|---|---|---|
| `auth.passkeys.enabled` | på | Användare kan registrera och använda lösennycklar. Av behåller registrerade lösennycklar men de kan inte användas, inte heller som andra faktor. |
| `auth.passkeys.passwordless` | på | Inloggning utan lösenord erbjuds (kräver också `auth.local`). Av: lösennycklar är bara en andra faktor. |
| `auth.passkeys.require_for_admins` | av | Superadministratörer och organisationsadministratörer måste ha en lösennyckel eller en autentiseringsapp, utan övergångsperiod: deras nästa inloggning måste först konfigurera en. |
| `auth.passkeys.rp_id` | värden i `server.base_url` | WebAuthn-id för den förlitande parten (RP-id), domänen som lösennycklarna gäller för (`example.com` låter `qtf.example.com` och andra underdomäner använda dem). |
| `auth.passkeys.origins` | ursprunget i `server.base_url` | Webbursprungen som får använda lösennycklarna. Vart och ett måste vara `https` (`http` bara för `localhost`) och ligga inom RP-id:t. |

Lösennycklar kräver en **https**-adress i `server.base_url` (eller de två inställningarna ovan): webbläsare erbjuder WebAuthn bara i ett säkert sammanhang, och RP-id:t binder varje lösennyckel till domänen. Utan en sådan ser användarna att lösennycklar inte är konfigurerade. **Välj RP-id innan användarna registrerar lösennycklar**: om det ändras, eller om servern flyttas till en annan domän, blir alla registrerade lösennycklar oanvändbara (användarna loggar då in med lösenordet och registrerar nya).

**Vad servern kontrollerar.** Utmaningarna är 32 slumpmässiga byte, sparas på servern, gäller i 5 minuter och tas bort när de besvaras (engångsbruk). De är bundna till webbläsarsessionen, till den påbörjade inloggningen med andra faktor eller, för inloggning utan lösenord, till en kortlivad `qtf_passkey`-kaka i webbläsaren som bad om den. För varje ceremoni kontrollerar servern klientdatans typ, utmaning och ursprung (inga ramar från andra ursprung), SHA-256 av RP-id:t i autentiseringsdatan, flaggan för användarnärvaro och, där det krävs, flaggan för användarverifiering, samt signaturen. Registreringar accepterar intygsformaten `none` (det Qtestframe ber om) och `packed` (självintyg, eller ett x5c-certifikat vars signatur kontrolleras men som inte kedjas till ett betrott rotcertifikat: Qtestframe avgör inte efter autentiserarmodell), med algoritmerna ES256, EdDSA (Ed25519) och RS256 (minst 2048 bitar). **Signaturräknaren** måste öka för autentiserare som har en: om den inte gör det släpps inloggningen igenom, men en varning loggas och händelsen `passkey.counter_regressed` registreras i händelseloggen, eftersom det kan betyda en klonad autentiserare. Synkade lösennycklar har ingen räknare. Implementationen använder bara Gos standardbibliotek (en liten, begränsad CBOR-avkodare för intygsobjekten); den lägger inte till något WebAuthn-beroende.

**Gränser.** Inloggning utan lösenord räknas mot inloggningsgränsen för klientadressen; dess alternativ begränsas till 60 per 5 minuter och adress. En lösennyckel som andra faktor räknas som en kod (10 försök per 15 minuter och konto, 5 per påbörjad inloggning).

**Administratörer** ser antalet lösennycklar per användare under **Medlemmar** (kolumnen *Lösennycklar*) och **Användare**, och kan **Återkalla lösennycklar** (en borttappad eller komprometterad enhet): alla användarens lösennycklar tas bort och användaren loggas ut överallt. Precis som för MFA-återställningar kan organisationsadministratörer göra detta för medlemmar som bara tillhör deras organisation.

**Händelselogg:** `passkey.register`, `passkey.rename`, `passkey.delete`, `user.reauth`, `member.revoke_passkeys`, `user.revoke_passkeys`, `passkey.counter_regressed`, och inloggningarna som `login` med metoden `passkey` (utan lösenord) eller `mfa=passkey` (andra faktor), med lösennyckelns namn. Misslyckade inloggningar med lösennyckel registreras i inloggningshistoriken med metoden *passkey*.

## OpenID Connect

Registrera en **konfidentiell webbklient** hos leverantören med:

- omdirigerings-URI `<server.base_url>/api/v1/auth/oidc/<namn>/callback`,
- beviljandetypen *authorization code* (Qtestframe använder alltid PKCE med S256),
- scopes `openid email profile`, plus `groups` om leverantören erbjuder det.

Lägg sedan till leverantören:

```yaml
auth:
  oidc:
    - name: corp
      display_name: "Företagsinloggning"
      issuer: "https://sso.example.com/realms/acme"
      client_id: qtestframe
      client_secret_file: oidc-corp            # eller QTF_OIDC_CORP_CLIENT_SECRET
      scopes: [openid, email, profile]
      groups_claim: groups
      auto_provision: true
      default_tenant: acme
      default_role: viewer
      role_mapping: {"qtf-admins": admin, "qtf-testers": tester}
```

eller lägg till den under **Administration → Systeminställningar → Enkel inloggning (OIDC)**, som kan testa discovery-dokumentet. Alla nycklar står i [referensen](configuration.md#authoidc). Steg för Keycloak, Microsoft Entra ID och Okta finns i [docs/operations.md](https://github.com/rom/Qtestframe/blob/rom/docs/operations.md#6-single-sign-on-oidc) (på engelska).

Vad Qtestframe kontrollerar: state, nonce och PKCE-verifieraren sparas i en signerad kaka för engångsbruk som gäller i 10 minuter; ID-tokens signatur verifieras mot leverantörens nycklar, och utfärdare, mottagare och giltighetstid kontrolleras. Token måste innehålla en e-postadress (`email_claim`); `email_verified: false` avvisas. Efter inloggningen går webbläsaren alltid till `/`.

Servern behöver utgående HTTPS till utfärdaren. Med SELinux: `setsebool -P qtestframe_connect_http on`. Namnge leverantörer med gemena bokstäver, siffror och `_`: namnet ingår i adressen och, med versaler, i `QTF_OIDC_<NAMN>_CLIENT_SECRET` (ett `-` gör variabeln svår att sätta).

### Lokal MFA och amr/acr

Som standard ansvarar identitetsleverantören för MFA, och Qtestframes egen andra faktor efterfrågas inte efter en OIDC-inloggning (`local_mfa: off`). Två striktare inställningar:

| `local_mfa` | Qtestframe frågar efter sin egen kod |
|---|---|
| `off` (standard) | aldrig |
| `unless_idp_mfa` | om inte ID-token visar MFA: dess claim `amr` innehåller `mfa` eller ett värde i `mfa_amr_values` (t.ex. `[mfa, otp, hwk]`), eller dess `acr` är ett av `mfa_acr_values` |
| `always` | vid varje inloggning via den här leverantören |

När koden efterfrågas behöver användarna en autentiseringsapp i Qtestframe också (eller måste lägga till en om MFA-policyn kräver det). De rapporterade värdena för `amr`/`acr` skrivs i granskningsposten för inloggningen. SAML-inloggningar använder aldrig den lokala andra faktorn; konfigurera MFA hos identitetsleverantören.

## SAML 2.0

Skapa ett nyckelpar för Qtestframe som tjänsteleverantör (SP) och peka IdP:n mot SP-metadata:

```sh
sudo scripts/certs/gen-saml-sp-cert.sh --cn qtf.example.com --name saml-adfs --out /etc/qtestframe/tls --group qtestframe
```

```yaml
auth:
  saml:
    - name: adfs
      display_name: "Företagsinloggning (ADFS)"
      idp_metadata_url: "https://adfs.example.com/FederationMetadata/2007-06/FederationMetadata.xml"
      sp_cert_file: /etc/qtestframe/tls/saml-adfs.crt
      sp_key_file: /etc/qtestframe/tls/saml-adfs.key
      email_attr: "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress"
      groups_attr: "http://schemas.xmlsoap.org/claims/Group"
      auto_provision: true
      default_tenant: acme
      role_mapping: {"QTF-Admins": admin}
```

| Adress | |
|---|---|
| `<base_url>/api/v1/auth/saml/<namn>/metadata` | SP-metadata (också standard för entitets-id) |
| `<base_url>/api/v1/auth/saml/<namn>/acs` | assertion consumer service (HTTP-POST) |

Bara **SP-initierad** inloggning stöds; IdP-initierade (oombedda) svar avvisas. Svaret eller varje assertion måste vara signerat av IdP-certifikatet i metadata; destination, mottagare och giltighetstid kontrolleras. AuthnRequests signeras inte, så stäng av *client signature required* hos IdP:n. Krypterade assertions dekrypteras med `sp_key_file`. SAML-leverantörer konfigureras bara i filen och läser `server.base_url` vid start. Metadataadressen hämtas vid första inloggningen; `idp_metadata_file` kräver ingen utgående anslutning.

**Lösenordsregler** (Systeminställningar → Inloggning och lösenord): `auth.password_min_classes` kräver 2–4 av gemener, versaler, siffror och symboler; `auth.password_history` (0–24) nekar de senaste lösenorden när användare byter sitt; `auth.password_max_age_days` gör att ett lösenord går ut – nästa inloggning därefter måste byta det (för konton äldre än inställningen räknas dagarna från nästa inloggning). Tillfälliga lösenord som administratörer sätter måste ändå bytas vid nästa inloggning.

## Provisionering av användare med single sign-on

Vid varje inloggning med OIDC eller SAML:

1. Användaren söks efter leverantör och subjekt. Annars efter e-post: ett lokalt konto (eller ett som SCIM skapat) med den adressen **kopplas** till leverantören och dess lösenord tas bort; ett konto som är kopplat till en *annan* leverantör avvisas. Annars skapas en ny användare bara med `auto_provision: true`.
2. Avstängda användare avvisas.
3. Med `default_tenant` blir rollen den mest privilegierade roll som `role_mapping` ger någon av användarens grupper, annars `default_role` (`viewer` när den är tom eller okänd). Ett saknat medlemskap skapas med `auto_provision`; med en `role_mapping` sätts ett befintligt medlemskap till den mappade rollen vid varje inloggning – identitetsleverantören bestämmer.

Eftersom konton kopplas via e-postadress: anslut bara identitetsleverantörer vars adresser är verifierade och tillhör er organisation. Medlemmar som SCIM hanterar lämnas åt SCIM.

Med `allowed_domains` nekas adresser i andra domäner innan något annat händer (`*.example.com` tillåter underdomänerna). Organisationen är `default_tenant`; utan den den organisation vars regler listar användarens e-postdomän (Administration → Organisationer → *E-postdomäner*). En avstängd organisation, eller en vars regler inte tillåter leverantören, går man inte med i. Rollerna är alla roller som `role_mapping` ger någon av användarens grupper – en koppling kan ange flera roller (`qa: "tester, reviewer"`) – annars `default_role`, som också kan lista flera. OIDC-leverantörerna har i webbklienten också en **knappikon** (`icon`), och knappen *Testa anslutningen* hämtar discovery-dokumentet.

## Provisionering med SCIM

Med SCIM 2.0 skapar, uppdaterar och inaktiverar en identitetsleverantör (Microsoft Entra ID, Okta med flera) medlemmarna i **en organisation**. Organisationsadministratören skapar en SCIM-token under **Administration → Provisionering (SCIM)** och ger leverantören basadressen `<base_url>/scim/v2` och token; [användarhandboken](../../user-guide/sv/administration.md#provisionering-med-scim) beskriver leverantörernas skärmar.

Det här bör serveradministratören veta:

- En SCIM-token (`qtfscim_` + 256 slumpbitar) lagras som hash, visas en gång, kan ha ett utgångsdatum och autentiserar bara `/scim/v2` för sin organisation. Den styr organisationens medlemskap upp till administratörsrollen (via en grupp som mappas till admin), så behandla den som ett administratörslösenord.
- Den kan inte ändra superadministratörsstatus, lösenord, projektdata eller andra organisationer; en organisation behåller alltid en administratör.
- Provisionerade konton saknar lösenord: de loggar in via OIDC eller SAML, som kopplar dem via e-post.
- När en användare inaktiveras tas medlemskapet bort och användarens sessioner och API-nycklar i den organisationen upphör.
- Gränser: 1200 begäranden per minut och token; 20 misslyckade autentiseringar per 5 minuter blockerar klientadressen (`429`). Varje ändring granskas med aktören `SCIM: <tokennamn>`.

## API-nycklar

API-nycklar är för skript och CI-flöden (`Authorization: Bearer qtf_…`). Medlemmar med `token.manage` (Testare och uppåt som standard) skapar dem under **Administration → API-nycklar**.

- En nyckel är `qtf_` + 32 slumpbyte; bara dess SHA-256-hash och ett kort prefix lagras, och den visas en gång.
- Den hör till **en organisation** och har en roll som inte kan överstiga skaparens. Vid varje begäran **begränsas** dess behörigheter **av skaparens nuvarande roll**: sänks användarens roll sänks nyckelns, tas medlemmen bort upphör den.
- Nycklar kan inte skapa nycklar, har aldrig superadministratörsrättigheter (inte ens en superadministratörs), kan inte använda systeminställningarna och omfattas inte av MFA.
- Valfritt utgångsdatum i dagar; senaste användning sparas; återkallelse gäller direkt.
- Begäranden med en nyckel hoppar över CSRF-kontrollen (det finns ingen kaka), så nyckeln är den enda inloggningsuppgiften: förvara den i CI-systemets hemlighetslager.

## Sessioner

- En session skapas vid varje inloggning. Webbläsaren har bara den slumpmässiga kakan `qtf_session` (`HttpOnly`, `SameSite=Lax`, `Secure` utom med `auth.cookie_secure: false`); databasen lagrar en SHA-256-hash av den.
- Sessioner upphör efter `auth.session_ttl` (standard 12 h, absolut, förlängs inte), vid utloggning, och när en administratör stänger av användaren, återställer lösenordet eller återställer MFA. Att byta sitt eget lösenord avslutar användarens andra sessioner. Utgångna sessioner rensas varje timme.
- Användare ser sina sessioner (webbläsare, adress, senaste aktivitet) i sin profil och kan avsluta var och en.
- Begäranden från webbläsaren som ändrar något måste skicka sessionens CSRF-token och klara en ursprungskontroll mot begärans värd eller `server.base_url`.
- Sessioner ingår inte i en säkerhetskopia; en återställning loggar ut alla.

**Token-regler och sessioner.** Serverns regler för API-tokens (Systeminställningar → API-tokens) kan kräva ett utgångsdatum (`auth.token_require_expiry`) och begränsa det (`auth.token_max_days`); de gäller nya tokens. Sessioner avslutas också efter `auth.session_idle_timeout` utan aktivitet (av som standard; läget för elektroniska poster i en organisation kan ange en kortare) och när en superadministratör loggar ut en användare överallt (Administration → Användare). "Kom ihåg den här enheten" för den andra faktorn anges i serverns MFA-regler.
