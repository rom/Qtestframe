# Test environments and test data

A **test environment** is a system your tests run against: a development or staging system, a pre-production system, a lab, a hardware-in-the-loop (HIL) rig or a device. Qtestframe keeps what each environment is made of (its **components and versions**), who owns it, whether it can be used right now, and who has **booked** it when. Every test result records the environment it was obtained in and the versions of its components at that moment, so a release can show which environments and versions were tested.

**Test data sets** are named, versioned sets of test data: tables of parameters that data-driven tests can share, or data files. Each set has a classification (synthetic, anonymised, a copy of production data), an owner and an expiry date.

Select **Environments** in the project sidebar. The Test data page is linked from there.

| Task | Role (permission) |
|---|---|
| See environments, their bookings and the test data | every role that sees runs and tests (`run.read`, `test.read`) |
| Book an environment | **Tester** and above (`run.execute`) |
| Create and change environments, approve bookings, reveal a stored secret | **Test lead**, **Manager**, **Admin** (`environment.manage`); for an environment shared by the organisation, in the organisation roles |
| Create and change test data sets | **Tester** and above (`test.write`) |
| Ask for a test data refresh | everyone who writes tests or executes runs |

## Environments

Select **New environment** and enter:

- a **name**, unique in the project, and the **type**: development, test, staging, pre-production, production-like, lab, HIL rig or device;
- the **status**: *Available*, *In use*, *Maintenance*, *Broken* or *Retired*, with a **status note** (for example *Firmware update until Friday*);
- the **owner**, the person responsible for it (you by default);
- the **capacity**: how many bookings may overlap. 1 means one at a time; an environment of CI containers may allow many;
- **Bookings need approval**: bookings wait until the owner or an environment manager approves them;
- the **URLs**, a description and tags;
- the **components and versions**, for example *Web app 1.4.0*, *PostgreSQL 16.4*, *Terminal firmware 5.0.1*;
- the **configurations** and **variants** it provides;
- where the **credentials** are, and optionally a **secret** (see below);
- optionally a **health check URL**.

Choose **Shared by the organisation** to make the environment available to every project of the organisation (for example a device lab used by several teams). Only someone who manages environments in the organisation can do this.

The **Status board** shows every environment as a card: its status, its components, how many bookings it has right now and its next booking, and the result of its last health check. The **List** tab has the same environments in a table that you can filter with the [query language](search-and-filters.md), for example `status:available component:PostgreSQL`, `type:hil_rig` or `booked:true`. Select an environment to open it.

### Components and versions

Keep the versions up to date when the environment is upgraded. A result recorded in the environment keeps the versions it was recorded with: changing the environment later does not change older results. The execution runner shows the environment and its versions next to the result, and **Releases → Runs → Tested environments** lists the environments and versions the release's results come from (also in the release readiness report).

### Credentials and secrets

**Where the credentials are** is a reference: a vault path, an entry in your password manager, a key cabinet. Prefer it to storing the secret in Qtestframe.

If you store a **secret** anyway, it is write-only: it is encrypted like the other secrets Qtestframe stores and never shown again in the dialog. Environment managers can select **Reveal** on the environment's page; every reveal is recorded in the audit log. A project archive (*Save project*) leaves the secret out.

### Health check

With a **health check URL**, Qtestframe fetches the URL in the background every few minutes (**Check every**). An answer with an HTTP status below 400 is *up*. When the check fails, an *Available* or *In use* environment becomes **Broken** (marked *set by the health check*) and its owner is notified; when the check passes again, it becomes *Available* again. A status you set by hand stays: the health check does not change *Maintenance*, *Retired*, or *Broken* set by hand. Select **Check now** to check at once.

## Booking an environment

Select **Book** on the Environments page or on an environment, or choose a day in the **Booking calendar**. Enter **From**, **To** and the **purpose**.

You can also book while you create a [run](runs.md), a [test cycle](test-cycles.md) or an [exploratory session](exploratory-testing.md): choose the **Environment** and tick **Book it for this**. The booking is linked to the run, cycle or session.

- An environment takes as many overlapping bookings as its **capacity**. A booking that does not fit is refused and the dialog lists the bookings in the way. Bookings that only touch (one ends when the next starts) do not overlap.
- When the environment needs approval, the booking is **Pending** until its owner or an environment manager approves or rejects it. They see it in **My work** (*Booking to approve*) and get a notification; you see your own pending booking in My work as *Awaiting approval*, and are notified of the decision. Owners and managers book without approval.
- Moving an approved booking of an environment that needs approval makes it pending again.
- You can cancel your own booking; owners and managers can cancel any booking of their environment.
- A retired environment cannot be booked. A booking of a shared environment made for a project you cannot see shows only its time.

The **Booking calendar** shows a week: one row per environment, a bar per booking (pending bookings dashed). Select a bar to see, move, cancel, approve or reject the booking. The environment's own page has the same calendar and its upcoming and past bookings.

## Environments of runs, cycles and sessions

Runs, test cycles and exploratory sessions select an **environment** from the project's environments (and the shared ones). The run keeps the environment's name as its environment text, so filters such as `environment:staging` work as before. A run created with a free-text environment (for example from CI or a result import) is linked to the environment of that name if there is one. When you upgraded, the free-text environments of your existing runs and cycles became environments of the project.

A test cycle's runs get the cycle's environment.

## Test data sets

Open **Test data** from the Environments page (or **Environments → Test data**). Select **New data set** and enter:

- a **name**, the **classification** and the **kind**: a **table of parameters** or **files**;
- the **owner**, an **expiry date** and tags;
- for a table: the **parameters** and **rows**. Select **Paste table** to paste data from a spreadsheet (CSV, semicolon or tab separated; the first line names the parameters, a column called *name* gives the row names);
- for files: save the set, then add the files on the **Files** tab.

**Classification.** *Synthetic* data is made up; *anonymised* data comes from production with personal data replaced; a **production copy** may hold personal or confidential data and is shown with a warning. Restrict access to it, follow your data protection rules and delete it when it expires.

**Versions.** Every change of a set is a new version (see the **History** tab).

### Sharing data between tests

On the **Links** tab, link the set to tests, configurations and runs. Tick **As the data source of the tests** when you pick data-driven tests: such a test then takes its parameters and rows from the set instead of its own (see [data-driven tests](tests.md)). When a run is created with the test, each row of the set becomes an iteration, and the **Usage** tab records which run used which version. The test's editor shows which shared set it takes its rows from. An archived set is not used: the test falls back to its own rows.

### Refresh requests and expiry

Anyone who writes tests or executes runs can **ask for a refresh** on the **Refresh requests** tab (what should be refreshed, and by when). The owner is notified and sees the request in **My work** (*Refresh requested*); they mark it **Done** or **Reject** it, and the requester is notified.

A set whose expiry date has passed is marked **Expired**, and one expiring within 14 days **Expires soon**. Both appear in the owner's My work, so the data is refreshed, extended or archived in time.

## Notifications and events

| Event | Who is notified |
|---|---|
| `environment.booking_requested` | the owner and the managers of the environment |
| `environment.booking_approved`, `environment.booking_rejected` | the person who booked |
| `environment.status_changed` | the owner (also when the health check changes the status) |
| `testdata.refresh_requested` | the owner of the data set |
| `testdata.refresh_resolved` | the person who asked |

The events can also be sent to [webhooks and chat channels](reviews-and-notifications.md).

## Backups and project archives

Environments, bookings, test data sets, their links, usage and refresh requests are part of backups. A project archive holds the project's own environments and their bookings and the project's test data sets; shared environments belong to the organisation and stay behind (runs keep the name), and so do stored secrets.
