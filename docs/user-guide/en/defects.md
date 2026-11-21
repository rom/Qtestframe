# Defects

A **defect** (bug) is a problem found while testing. Every defect has a key such as `SHOP-BUG-3`, a title, a description, a status, a severity, a priority and an assignee, and it is linked to the requirements, tests and executions it concerns. Defects are under **Defects** in the project sidebar.

Defects can be mirrored in **Jira**, **GitHub Issues** or **GitLab**: Qtestframe creates the issue for you and reads its status back, so the developers work in their tracker while the test team sees the state of each defect next to its tests.

| Action | Role needed |
|---|---|
| View defects | Viewer |
| Report and edit defects, link them, push, sync and import issues | Tester |
| Delete defects, connect the tracker | Manager |

## The defect list

**Defects** lists the defects of the project, open ones first and the most severe at the top. The columns show the key, title, status, severity, assignee, the linked items and the issue in the tracker with its own status.

Filter with the quick filters **Status** and **Severity**, or with the [query language](search-and-filters.md), for example `open:true severity:critical`, `assignee:anna*` or `linked:SHOP-REQ-12`. The fields are `key`, `title`, `description`, `status`, `severity`, `priority`, `assignee`, `external` (the issue key in the tracker), `created_by`, `open` (`true` for *Open* and *In progress*) and `linked` (keys of linked requirements and tests).

Select a defect to open and edit it. Select **New defect** to report one.

## Change many defects at once

Tick defects in the list (the header checkbox ticks the whole page). The toolbar changes all of them at once:

- **Set status** and **Set severity**, and **Assign…** to give them to someone (empty removes the assignee). The new assignee is notified. This needs permission to record results (`run.execute`).
- **Push to Jira/GitHub/GitLab** creates an issue for each ticked defect that has none yet; defects that already have an issue are left alone. **Sync** reads the status of the ticked defects' issues. Both are shown when a tracker is connected.
- **Delete** (permission to change the project), after confirmation.

Every defect is checked before any is changed, so one wrong value changes nothing. Status, severity and assignee changes can be undone with the undo button in the top bar.

## Statuses and severities

| Status | Meaning |
|---|---|
| **Open** | Reported, not yet being worked on |
| **In progress** | Someone is fixing it |
| **Resolved** | Fixed, waiting to be verified by a new test |
| **Closed** | Verified or done |
| **Rejected** | Not a defect, a duplicate or will not be fixed |

*Open* and *In progress* count as **open**. Open defects are shown on requirements and tests and block a release (see [Open defects and releases](#open-defects-and-releases)).

| Severity | Typical use |
|---|---|
| **Critical** | Crash, data loss, security issue, no workaround |
| **Major** | An important function does not work (the default) |
| **Minor** | Works with a workaround, or a small function is affected |
| **Trivial** | Cosmetic, e.g. a typo |

Severity says how bad the problem is; **priority** (critical, high, medium, low) says how soon it should be fixed.

## Linking defects to items

A defect can be linked to any number of **requirements**, **tests** and **executions** of the project. In the defect dialog, under **Linked items**, select **Link requirement** or **Link test** to pick items, or **Link execution** to choose a run and one of its executions. Remove a link with ×.

The **Defects** tab of a requirement or test shows its defects:

- a **test** shows the defects linked to it and to its executions in every run (*via an execution*);
- a **requirement** shows the defects linked to it, to its tests and to their executions (*via a test*).

There you can **Report defect** (a new defect already linked to the item) or **Link existing** (choose a defect of the project). × removes a direct link. A requirement with open defects shows a red badge such as *2 open defects* next to its key; select it to open the tab.

Defect keys work like other keys: `SHOP-BUG-3` in a description or comment becomes a link, and `/go/SHOP-BUG-3` opens the defect.

Defect IDs typed into the **Defects** field of an execution (the older free-text list, see [Defects in the runner](runs.md#defects)) keep working. When such an ID is a defect key of this tool, for example `SHOP-BUG-3`, it links to the defect itself.

## Report a defect from a failed execution

When a test fails in the [execution runner](runs.md#the-execution-runner), a notice with **Report defect** appears on the result. It opens the defect dialog prefilled with:

- the test's key and title as the title;
- the run, the failed steps with their expected and actual results, and the notes of the execution as the description;
- a link to the execution (and so to the test and its requirements).

Adjust the text, set the severity and select **Report defect**. The new defect appears in the **Defects** tab of the runner, and the test and its requirements show it too. The tab also has **Link existing** for a defect that is already known.

## Connect Jira, GitHub or GitLab

A manager connects the project to a tracker in **Categories & settings**, tab **Project**, card **Issue tracker**. The same connection is used for defects and for [issues created from requirements](requirements.md#issues-in-gitlab-github-or-jira). Choose the **Tracker** and fill in the fields; **Check connection** saves the settings and checks the address, the project or repository and the token.

### Jira

- **Jira site address**: the address of the site, e.g. `https://example.atlassian.net`, or of your Jira Server or Data Center.
- **Jira project key**, e.g. `SHOP`, and the **Issue type** of new issues (default *Bug*).
- **Jira account e-mail** and **Token**: for Jira Cloud, the e-mail of the account and an **API token** created at id.atlassian.com under *Security, API tokens*. For Jira Server or Data Center, leave the e-mail empty and use a **personal access token**.

The account needs permission to browse the project and to create issues in it.

### GitHub Issues

- **Repository** as `owner/repository`.
- **API address**: empty for github.com; for GitHub Enterprise Server e.g. `https://github.example.com/api/v3`.
- **Labels** added to new issues, separated by commas, e.g. `bug, qtestframe`.
- **Token**: a **fine-grained personal access token** with access to the repository and the repository permission **Issues: Read and write** (*Metadata: Read* is included automatically).
- **Milestone** and **Assignee** (optional): given to new issues; the milestone by number or title, the assignee by login.

### GitLab

- **GitLab address**: empty for gitlab.com; for a self-managed instance its address, e.g. `https://gitlab.example.com`. Only `https` addresses are accepted.
- **Project**: the numeric project ID (shown on the project's overview page) or its path, e.g. `group/subgroup/project`. You can also paste the project's address; it is reduced to the path.
- **Labels** added to new issues, separated by commas. GitLab creates labels that do not exist yet.
- **Milestone** (optional): the number GitLab shows as `%3`, or the title. Milestones of the project's groups work too.
- **Assignee** (optional): the GitLab username of whoever gets new issues.
- **Token**: an access token with the scope **`api`** (the smallest scope that can create and edit issues). Make one of these:
  - a **project access token** (recommended, as it can only reach this project): in the GitLab project, **Settings → Access tokens → Add new token**, give it a name and an expiry date, the role **Reporter** (or higher) and the scope **api**;
  - a **group access token**, in the same way under the group's **Settings → Access tokens**, when several projects of the group share it;
  - a **personal access token**: **Edit profile → Access tokens → Add new token** with the scope **api**; it acts as you in every project you can reach, so prefer one of the others.

  Copy the token when GitLab shows it (it is shown only once) and paste it into **Token**. Reporter can create issues and set labels, milestones and assignees; with Guest only the title and description are kept. Qtestframe sends the token in the `PRIVATE-TOKEN` header.

The status of a GitLab issue is *opened* or *closed*; see the table under [Push, sync and import](#push-sync-and-import).

### The token

The token is **write-only**: once saved it is never shown or sent back to the browser, the card only says **Token saved**. To keep it, leave the field empty when you change other settings. To replace it, type a new one and save. **Remove token** deletes it. Setting the tracker to **None** removes the whole connection.

## Push, sync and import

With a tracker connected, the defect list has three more actions:

- **Push to tracker** (the upload button of a defect) creates an issue from the defect: the title with the defect key, the description, the severity and priority, and the keys of the linked items. The defect then shows the issue key, a link to it and its status in the tracker. A defect that already has an issue cannot be pushed again.
- **Sync** (the sync button of a defect) reads the issue's status and updates the defect's status; **Sync all** does this for every defect with an issue in the connected tracker and reports how many changed. Statuses are mapped as follows:

  | Jira | GitHub | GitLab | Defect status |
  |---|---|---|---|
  | status category *To Do* | open | opened | Open |
  | status category *In Progress* | | | In progress |
  | *Done* | closed (completed) | closed | Closed |
  | *Done* with resolution *Won't Do*, *Duplicate* or similar | closed (not planned) | | Rejected |

- **Import issue** creates a defect from an issue that already exists in the tracker: enter its key (`SHOP-123` in Jira, `#123` or `owner/repo#123` in GitHub, `#123` or `group/project#123` in GitLab) and the severity. Title and status come from the issue.

Sync reads only; it never changes the issue. Changes you make to a defect in Qtestframe are not written to the tracker.

If the tracker cannot be reached or refuses a request, for example because the token expired or lacks a permission, a red notice at the top of the list shows the tracker's own answer. Check the settings with **Check connection**. The notice explains the common answers: **401** the token was rejected (wrong, expired or revoked), **403** the token lacks a scope or role, **404** the project, repository or issue does not exist or the token cannot see it, **422** the tracker refused the content (for example an unknown label or milestone). When the tracker limits the rate, Qtestframe waits and tries again if the tracker asks for a short wait, and otherwise says how long to wait.

Qtestframe reaches the tracker from the server, through the server's outbound proxy (`HTTPS_PROXY`, `NO_PROXY`) if one is set, and trusts the certificates the server's system trusts. For a self-managed GitLab or Jira with a certificate from your own CA, install that CA on the server.

## Open defects and releases

Open defects (status *Open* or *In progress*) linked to a requirement, to its tests or to their executions count for the requirement. They are shown:

- as the badge on the requirement and the count on the **Defects** tab;
- in the requirement list as the column *Open defects* (`open_defects`);
- in the [release](releases-and-risks.md) exit criterion **No open defects** (`no_open_defects`): a release is not ready while a requirement in its scope has an open defect. The criterion can be limited to a minimum severity, e.g. only *critical* and *major* defects block the release. The release page lists the defects that block it.

Resolve or close the defects (in Qtestframe or in the tracker followed by **Sync**) to meet the criterion.
