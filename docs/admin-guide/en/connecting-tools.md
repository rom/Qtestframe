# Connecting other tools (OSLC and ReqIF)

Qtestframe exchanges requirements and test cases with ALM and requirements tools in two ways (the user guide page *Interoperability* describes what users see):

- **OSLC**, live: Qtestframe is an OSLC provider for Requirements Management 2.1 (requirements) and Quality Management 2.1 (test cases). Other tools discover it, query, create, change and delete items, link their own items to them, and show Qtestframe's selection and creation dialogs to their users.
- **ReqIF files**, for whole specifications: export from one tool, import into the other, repeatedly. Nothing needs to be set up on the server.

This chapter is about setting up OSLC connections with IBM Engineering Lifecycle Management (DOORS Next, Engineering Test Management), Siemens Polarion and PTC Codebeamer, and about what to check when one does not work.

## What the server offers

| Address | Use |
|---|---|
| `<base_url>/api/v1/oslc/rootservices` | Jazz-style discovery document (public, holds no data). IBM ELM and tools that use "friends" start here. |
| `<base_url>/api/v1/oslc/catalog` | Service provider catalog: one service provider per project the account may read. |
| `<base_url>/api/v1/oslc/projects/<project id>/catalog` | A catalog with one project only, for tools that connect one of their projects to one project here. |
| `<base_url>/api/v1/oslc/projects/<project id>` | The service provider of a project: query capabilities, creation factories, selection and creation dialogs, resource shapes. |

The project id is the one in the web address of the project (`/p/<id>/…`). Resources are served as RDF/XML (the OSLC default), Turtle or JSON-LD; request bodies are accepted in the same formats. The API reference (`docs/api.md`, section *OSLC*) lists every property, the link properties and the query syntax.

## Before you connect a tool

1. **Base URL.** Set `server.base_url` (or **Base URL** in *System settings*) to the address the other tool reaches Qtestframe at, for example `https://qtf.example.com`. Every OSLC resource address is built from it; behind a reverse proxy it must be the public address, not the internal one.
2. **TLS.** Most ALM servers refuse plain HTTP and certificates they do not trust. Use a certificate of a CA the other tool's Java trust store knows (or add your CA to it).
3. **A service account and an API token.** OSLC clients authenticate with an API token, either as `Authorization: Bearer <token>` or as the password of Basic authentication (any user name), on the OSLC paths only. OAuth 1.0a, which IBM ELM uses between Jazz applications, is not supported; use the Basic/token form in the other tool's "custom" or "basic" connection type. Create a user (for example `alm-connector@example.com`) and give it the role the connection needs:
   - **Viewer** to let the tool read and link (the link is stored in the other tool; Qtestframe shows it there).
   - A role with `requirement.write` and/or `test.write` to let the tool create and change items, and `test.write` to change trace links (*validatedBy* / *validatesRequirement*).
   Sign in as that user, create the token under **Administration → API tokens** with the role the connection needs (a token can have a lower role than its account, never a higher one) and give it to the administrator of the other tool. Restricted projects are only visible when the account is a member.
4. **Dialogs in a frame** (optional). The selection and creation dialogs open in a window of their own, which works everywhere. Tools that show them inside their own page (an iframe) need two things:
   - the tool's origin in `server.frame_ancestors`, for example
     ```yaml
     server:
       base_url: https://qtf.example.com
       frame_ancestors:
         - https://elm.example.com
         - https://polarion.example.com
     ```
     The dialog pages then send `Content-Security-Policy: frame-ancestors 'self' https://elm.example.com …`; without the setting they may only be framed by Qtestframe itself.
   - the browser must send Qtestframe's session cookie inside the frame. The cookie is `SameSite=Lax`, so the frame works when both tools are on the **same site** (subdomains of one registrable domain, such as `qtf.example.com` and `elm.example.com`). For tools on another site, leave the dialogs in a window.
   The dialogs use the user's own session: each user signs in to Qtestframe in the browser (the dialog offers a link if they have not).

## IBM DOORS Next and Engineering Test Management

DOORS Next links its requirements to test cases of a QM provider, and ETM links its test cases to requirements of an RM provider. Qtestframe can take the RM role (its requirements) and the QM role (its test cases).

1. In the ELM **Lifecycle Project Administration** (or the project area's **Associations**), add an association to an **OSLC provider that is not a Jazz application**: choose *Add → Service provider catalog* (wording varies by version) and enter `<base_url>/api/v1/oslc/catalog`, or `<base_url>/api/v1/oslc/projects/<id>/catalog` for one project. When the version only accepts friends, register a friend with the root services `<base_url>/api/v1/oslc/rootservices` and choose the basic authentication method with the service account's e-mail and the API token as password.
2. Choose the association type: **Provides Requirements** (or *Uses Requirements* from the ETM side) for Qtestframe requirements, **Provides Test Cases** (*Validated By*) for Qtestframe test cases.
3. In a DOORS Next artifact, **Links → Add link → Validated By** now offers *Qtestframe* as a target: the selection dialog opens, the user searches and ticks test cases, and DOORS Next stores the links. **Create** opens Qtestframe's creation dialog instead.

Links stored by DOORS Next live in DOORS Next. When a tool writes the link into the Qtestframe resource as well (a PUT with `oslc_qm:validatesRequirement` or `oslc_rm:validatedBy`), Qtestframe keeps it as a trace link when the other end is one of its own items, or as an item link to the other tool's address otherwise.

## Siemens Polarion

Polarion links work items to OSLC resources of other servers through *OSLC connections*:

1. In **Administration → Linked data / OSLC → Remote OSLC providers** (Polarion's *Connections* configuration), add a provider with the catalog `<base_url>/api/v1/oslc/catalog`, authentication **Basic**, user the service account's e-mail and password the API token.
2. Map the link roles: a Polarion requirement *is validated by* → Qtestframe test cases (`oslc_qm:TestCase`), a Polarion test case *verifies* → Qtestframe requirements (`oslc_rm:Requirement`).
3. Polarion users then pick Qtestframe items in the **Link to OSLC resource** dialog, which opens Qtestframe's selection dialog.

For a one-off or regular transfer of a Polarion document, use ReqIF instead: export the document as ReqIF in Polarion and import it in Qtestframe with *Import → ReqIF … with updates*; map the work item types (for example *Test Case* to test cases) in the import preview.

## PTC Codebeamer

Codebeamer connects to OSLC providers as *Remote trackers / OSLC associations*:

1. In the project's **Admin → Integrations → OSLC**, add a server with the root services or catalog address above, authentication **Basic** with the service account and the API token.
2. Choose the remote service: *Requirements* (Qtestframe requirements) or *Test cases* (Qtestframe test cases), and the Codebeamer association type it should create (for example *verifies* or *related*).
3. Codebeamer's association dialog then lists Qtestframe items; the delegated dialogs are used where Codebeamer supports them.

Codebeamer also exports and imports ReqIF; its *Test Case* trackers map to Qtestframe test cases in the import preview.

## Other OSLC clients and scripts

Any OSLC 2.0 client works with the catalog and a token. A quick check from the command line:

```sh
curl -H "Authorization: Bearer $QTF_TOKEN" -H "Accept: text/turtle" \
  "https://qtf.example.com/api/v1/oslc/projects/<id>/requirements?oslc.where=qtf:status=%22approved%22&oslc.select=dcterms:title"
```

Creating a requirement:

```sh
curl -H "Authorization: Bearer $QTF_TOKEN" -H "Content-Type: text/turtle" -H "Accept: text/turtle" \
  --data-binary '<> a <http://open-services.net/ns/rm#Requirement> ; <http://purl.org/dc/terms/title> "Emergency stop" .' \
  "https://qtf.example.com/api/v1/oslc/projects/<id>/requirements"
```

The answer is `201 Created` with the new address in `Location` and the version in `ETag`. An update (PUT) with `If-Match: "<ETag>"` is refused with `412 Precondition Failed` when someone saved in the meantime, as in the web client.

## What is stored, audited and backed up

- Items created, changed or deleted through OSLC are ordinary items: they have history, the changes are in the audit log under the account of the token, and deletion moves them to the project's trash.
- Trace links, relations and item links written through OSLC are ordinary links; links to resources of other tools are item links to their URL, labelled with the OSLC property (`oslc_rm:implementedBy` …).
- The ReqIF import records where every item, relation and trace link came from (table `external_refs`), so that importing the same document again updates them. This table is part of backups and of project archives.
- Nothing about a connection is stored in Qtestframe besides the API token: the other tool keeps the connection settings.

## Troubleshooting

| Symptom | Check |
|---|---|
| The tool says the catalog is empty | The account is not a member of the project (restricted projects), or the token's role has no read permission. Open the catalog address with the token in a browser or with `curl`. |
| Links point to `http://localhost:…` or an internal host | `server.base_url` is not the public address. |
| 401 Unauthorized | The token is wrong, expired or revoked; Basic authentication must carry the token as the password. Plain passwords are not accepted on the OSLC paths. |
| 403 Forbidden on create or update | The token's role lacks `requirement.write` / `test.write`, or `test.write` for trace links. |
| 412 Precondition Failed | The item changed since the tool read it; the tool must read it again (its ETag is the item's version). |
| 415 Unsupported Media Type | The tool sends a body that is not Turtle, RDF/XML or JSON-LD. |
| The dialog stays blank in a frame | The tool's origin is not in `server.frame_ancestors`, or the tools are on different sites so the browser does not send the session cookie: use the dialog in a window. |
| The dialog says *You are not signed in* | The user has no Qtestframe session in this browser: sign in in another tab and choose **Try again**. |
