# Getting started

This page explains how to sign in, find your way around the screen, and set up Qtestframe the way you like it.

## Sign in

Open the address your administrator gave you (for a local trial installation `http://localhost:8080`).

![The sign-in page with e-mail, password, the language switch and the link for signing in to a specific organisation](../images/en-login.png)

1. Enter your **E-mail** and **Password** and select **Sign in**.
2. If you belong to several organisations and want to start in a particular one, select **Sign in to a specific organisation…** and enter its short name (slug), for example `acme`. Without it you start in your default organisation (see [Preferences](#preferences)) or the one you used last.
3. If your organisation uses single sign-on, the page shows a button per provider below **or continue with**. Select it and sign in at your identity provider; you return to Qtestframe automatically.
4. If you use [multi-factor authentication](#multi-factor-authentication), the page then asks for the **Authentication code**: open your authenticator app, enter the 6-digit code it shows for Qtestframe and select **Verify**. Each code works once; if one is refused, wait for the next. Without your phone, select **Use a recovery code instead** and enter one of your recovery codes. If your organisation allows it, **Remember this device** skips the code on this browser for a number of days.

The language buttons at the top of the sign-in page switch between English and Swedish before you sign in. After sign-in, the language in your preferences wins.

> **Tip:** After ten failed attempts within five minutes, sign-in is blocked for a while for that e-mail address and network address. Wait a few minutes and try again, or ask an administrator to [reset your password](administration.md#reset-a-password).

### What you see after signing in

- **The intro tour.** On your first sign-in a short tour highlights the main parts of the screen. Use **Next** and **Back**, or **Skip tour** (or <kbd>Esc</kbd>) to close it. You can start it again at any time with **Help → Restart intro tour**.

  ![The intro tour: a step card next to the highlighted Requirements entry of the sidebar, with Back, Next and Skip tour](../images/en-intro-tour.png)
- **Sign-in information.** On later sign-ins Qtestframe can show when and from where you last signed in, and how many failed attempts there have been since. If you do not recognise a sign-in, change your password and sign out your other sessions (see [Security](#security)).
- **A forced password change.** If an administrator has reset your password, you must choose a new one before you can continue.
- **Multi-factor authentication required.** If your organisation requires a second factor and you have not set one up, a reminder tells you by when; after that date you must [set up an authenticator app](#multi-factor-authentication) at sign-in before you can continue.

## The layout

![The main layout: sidebar on the left, top bar with organisation switch, search, help, theme picker, dark mode and the user menu](../images/en-layout.png)

### Sidebar

The dark bar on the left is the main navigation:

- **Workspace:** **Projects** (all projects of the current organisation) and **Search** (search across projects). Each project card shows when the project was started and by whom: who created it, or loaded or duplicated it from a [project file](reports-and-export.md). The creator is left out when it is not known (projects from the demo data or the command line, and projects older than this information). The table view has the same as columns, and **Categories & settings → Project** shows it too.
- **The open project:** once you open a project, its key and name appear with its sections: **Dashboard**, **Requirements**, **Tests**, **Operations** (test runs), **Traceability**, **Analytics**, **Dependency graph**, **Test flow**, **AI prompts** (software projects), **Tags**, **Baselines**, **Categories & settings**, **Export** and, where available, **Documents** and **Reports**.
- **Administration**, if your role allows it.

Inside a project, the same sections are also shown as tabs at the top of the page, below the breadcrumb *Projects › project name*.

**Collapse** at the bottom shrinks the sidebar to icons; hover an icon to see its name. On screens narrower than 1024 pixels the sidebar is hidden and opens with the menu button (☰) in the top bar; <kbd>Esc</kbd> or the close button closes it again.

### Top bar

From left to right:

| Element | Use |
|---|---|
| **Organisation** switch | Shows the current organisation and lets you switch to another one you are a member of. On phones it is a building icon. |
| **Search** field | Searches requirements, tests and runs in all projects of the organisation. Type text or a [query](search-and-filters.md) and press <kbd>Enter</kbd>. On phones it is a magnifier button. |
| **Help** (?) | Opens the **User guide**, **Help for this page**, **Query language**, **References and standards** and **Restart intro tour**. |
| **Undo / Redo / Undo history** | Undoes and redoes your changes, see [Undo and redo](#undo-and-redo). |
| **Colour theme** | Picks one of the twenty-two themes, see [Themes](accessibility-and-languages.md#themes). |
| **Light mode / Dark mode** | Switches between light and dark quickly. |
| **User menu** | Your name and role, **Profile & password**, light/dark mode, **Language** and **Sign out**. |

### Undo and redo

Most changes can be undone: editing, creating, copying and deleting requirements and tests, moving them, bulk edits, links, relations, dependencies, review decisions, restoring a version, test results in the runner, and creating, renaming and moving folders.

- **Undo** (↶, <kbd>Ctrl</kbd>+<kbd>Z</kbd>) reverts the latest change; select it again to go further back. The tooltip names the change, for example *Undo: Edit SHOP-TC-4*.
- **Redo** (↷, <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>Z</kbd> or <kbd>Ctrl</kbd>+<kbd>Y</kbd>) applies an undone change again. A new change clears what could be redone.
- **Undo history** (🕘) lists your last 50 changes, newest first, with when they were made. Selecting one undoes it and everything after it.

The keyboard shortcuts work outside text fields; in a text field they undo your typing as usual. The history belongs to you and your browser tab: it is cleared when you switch organisation, sign out or reload the page. A deleted requirement or test comes back with its key, links, relations, comments, history, attachments and execution results; deletions can be undone for 30 days through the API (see `GET /projects/{pid}/trash` in the API reference). An undo fails when the situation changed in the meantime, for example when someone else edited the item or you no longer have the permission; the item is then left as it is and a message explains why.

### Autocomplete

Text fields where you often write the same thing suggest what you saved in them before: titles of requirements, tests, defects and risks, a requirement's source, owner and rationale, a test's preconditions and expected result, the action, expected result and data of test steps, execution notes and actual results, comments, review comments, run and release names, and recent searches. For a requirement's **Source** and **Owner** and a risk's **Owner**, the values already used in the project are suggested too.

- Start typing: after a short pause a list of at most eight suggestions opens under the field. Values that start with what you typed come first, then values with a word that starts with it, then values that contain it; the ones you use most and most recently win ties.
- Nothing is chosen until you press <kbd>↓</kbd> or <kbd>↑</kbd>, so <kbd>Enter</kbd> and <kbd>Tab</kbd> work as usual while the list is just open. On a highlighted suggestion, <kbd>Enter</kbd> or <kbd>Tab</kbd> puts it in the field (it replaces the text); <kbd>Esc</kbd> closes the list without closing the dialog. You can also click or tap a suggestion.
- In an empty one-line field, <kbd>↓</kbd> lists your most used recent values; in a multi-line field use <kbd>Alt</kbd>+<kbd>↓</kbd>, because the arrow keys move the cursor there.
- Screen readers hear how many suggestions there are and what was inserted. A clock icon marks your own history, a folder icon a value used in the project.

Your history is saved with your account when you save a form (not while you type), so it follows you to every browser. You only ever see your own history, and only from projects you can still open. Turn the suggestions off, or limit them to one source, with **Autocomplete suggestions** in your [preferences](#preferences); **Clear my autocomplete history** in your profile deletes everything that was saved. Passwords, tokens and e-mail addresses are never remembered.

### Help where you need it

- Many fields and sections have a small **i** next to their name. Point at it with the mouse, or move the keyboard focus to it, to see what the field means. Select it (or tap it on a phone) to keep the explanation open, and choose **Read more in the user guide** to open the section that describes it. <kbd>Esc</kbd> closes it.
- Press <kbd>?</kbd> anywhere outside a text field to open the guide page for the current view (in the execution runner, <kbd>?</kbd> shows the runner's keyboard shortcuts instead).
- The **?** next to every filter field lists the operators and fields of the [query language](search-and-filters.md) for that list.

### Organisations

An organisation (tenant) is a separate workspace with its own projects, members and API tokens. When you switch organisation in the top bar, Qtestframe takes you to that organisation's project list. Your role can differ between organisations, so what you may do can change too.

If you see the message *You have no current organisation*, pick one in the organisation switch. If you are not a member of any organisation, ask an administrator to add you.

### Search

The search field in the top bar searches everywhere in the current organisation. Results are grouped by type with the matching text highlighted; select a result to open it. Full details are in [Search and filters](search-and-filters.md).

### Theme and language

- Pick a **colour theme** in the top bar or in your preferences. Your choice is stored with your account and follows you to every device.
- Change the **language** with **User menu → Language**: *English* or *Svenska*. The change applies at once and is saved as your preference.

## Your profile

Open **User menu → Profile & password**.

![The profile page with account details, memberships, preferences and security](../images/en-profile.png)

### Account

- **Display name**: the name others see in comments, history and the audit log.
- **Avatar**: choose a picture for your account: upload an image (PNG, JPEG, GIF or WebP, at most 1 MB), pick one of the built-in pictures, or use your initials.
- **Sign-in provider**, **Member since** and your **Memberships**: the organisations you belong to and your role in each.
- **Your data**: **Download my data** gives you a ZIP of JSON files with the personal data the server holds about you: profile, memberships and roles, comments you wrote and comments that mention you, sessions, API tokens (without their secrets), sign-in log, notifications, electronic signatures, input history and the audit entries by or about you. The download is recorded in the audit log. To have your account erased, ask a super administrator (see the administrator guide, *Privacy and data retention*).

### Preferences

Preferences are stored with your account and applied on every device. Change them and select **Save preferences**.

| Preference | Effect |
|---|---|
| **Language** | English or Swedish; *Browser default* follows the browser. |
| **Theme** | Colour theme. *Follow the operating system* switches between light and dark with your system setting. |
| **Start page** | What you see after signing in: the project list, the last opened project, a default project, search or your [home dashboard](traceability-and-coverage.md#customising-dashboards). |
| **Default project** | The project opened when the start page is *Default project*. |
| **Default organisation** | The organisation selected at sign-in when you do not name one. |
| **Rows per table page** | 10, 25, 50 or 100 rows in tables. |
| **Date format** | *Locale* (as usual in your language), *ISO* (2026-03-14 09:30) or *Relative* (3 hours ago). |
| **Time zone** | The time zone for all times; *Browser* uses your computer's. |
| **Also create linked test (new requirements)** | Your personal default for the checkbox in the requirement editor. |
| **Compact tables** | Denser table rows, to see more at once. |
| **Hide the live requirement-language analysis** | Turns off the analysis box in the requirement editor. |
| **Highlight obligation keywords** | On by default: marks MUST, SHOULD, MAY, SKA, BÖR, FÅR … in requirement descriptions and shows the keyword line under the description in the editor. See [Highlighted obligation keywords](requirements.md#highlighted-obligation-keywords). |
| **Autocomplete suggestions** | What text fields suggest (see [Autocomplete](#autocomplete)): *On: my history and values used in the project* (the default), *Only my own history*, *Only values used in the project* or *Off*. With *Off* no suggestions are shown; with *Off* and *Only values used in the project* nothing you save is added to your history. |

Under the preference, **Clear my autocomplete history** deletes all the values your text fields remember for you, in every organisation, and shows how many are stored.

### Security

- **Password**: for local accounts, enter your **Current password** and the **New password** twice, then select **Change password**. If you sign in with single sign-on, your password is managed by your identity provider.
- **API tokens**: tokens for CI pipelines and scripts are created under **Administration → API tokens** (see [API tokens](administration.md#api-tokens)).
- **Active sessions**: every browser and device that is signed in with your account, with browser, IP address, sign-in time and last activity. **Sign out** ends one session; **Sign out all other sessions** ends all except the one you are using.
- **Sign-in history**: your recent sign-ins and failed attempts, if shown. Wrong codes of the second factor are listed as failed attempts with the method *Second factor*.

### Multi-factor authentication

A second factor protects your account even if someone learns your password: after the password you enter a 6-digit code from an **authenticator app** on your phone, for example Microsoft Authenticator, Google Authenticator, 1Password or FreeOTP.

**Turn it on**

1. In your profile, under **Multi-factor authentication**, select **Set up authenticator app**.
2. Scan the QR code with the app. On a phone, **Open in an authenticator app on this device** adds the account directly; if you cannot scan, select **Cannot scan the code? Enter the key by hand** and type the setup key into the app.
3. Enter the 6-digit code the app shows and select **Turn on**.
4. Qtestframe shows ten **recovery codes**. Save them now (**Copy** or **Download**), for example in a password manager: they are shown only this once. Each code works once and lets you sign in without your phone. Then select **I have saved the codes**.

From then on, every sign-in with your password asks for a code. API tokens are not affected.

**Later**

- **New recovery codes** creates ten new codes (the old ones stop working). Do this when only a few are left; the profile warns you.
- **Forget remembered devices** makes every browser where you chose *Remember this device* ask for a code again.
- **Turn off** removes the authenticator app and your recovery codes; both need a current code (or a recovery code). If your organisation requires multi-factor authentication, you cannot turn it off. You get an e-mail when it is turned off.
- **New phone or lost phone**: with a recovery code you can still sign in; turn MFA off and set it up again with the new phone. Without phone and recovery codes, ask an administrator to [reset your multi-factor authentication](administration.md#reset-multi-factor-authentication).

If you sign in with single sign-on, your identity provider usually asks for the second factor; Qtestframe asks for its own code only if the server is configured to do so.

## Next steps

- Follow the [tutorial](tutorial.md) to build a small project from scratch.
- Learn the [query language](search-and-filters.md); it works in every filter field.
