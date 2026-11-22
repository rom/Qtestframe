# Discussions

Every requirement, test, defect, risk, test run and review request has a **discussion**: threads of comments where you ask questions, agree on changes and mention colleagues. A thread stays open until someone resolves it, so open questions do not get lost.

## Where to find a discussion

- **Requirements and tests**: the **Discussion** tab of the editor. The number on the tab is the number of open (unresolved) threads.
- **Test runs**: the **Discussion** tab of the run, next to the executions, files and links.
- **Defects and risks**: below the form when you open a defect or a risk.
- **Review requests**: in the review dialog, below the checklist and the review comment. The reviewer, the person who asked for the review and everyone who can read the reviewed item take part.
- **All threads of a project**: the project's **Discussions** page (see [The Discussions page](#the-discussions-page)).

## Write a comment

Type in the field at the bottom and select **Start thread** (or press <kbd>Ctrl</kbd>+<kbd>Enter</kbd>). Each new comment written there starts a thread. To answer in a thread, select **Reply** below it. Replies are one level deep: a reply to a reply goes to the same thread.

Comments are written in **Markdown**:

| You type | You get |
|---|---|
| `**bold**`, `*italic*`, `~~struck~~` | **bold**, *italic*, ~~struck~~ |
| `` `code` `` and blocks between ```` ``` ```` lines | code |
| lines starting with `-` or `1.` | lists |
| `> text` | a quote |
| `[text](https://example.com)` or a bare address | a link (opens in a new tab) |
| `SHOP-REQ-12`, `SHOP-TC-3`, `SHOP-BUG-4` | a link to that item |

HTML typed into a comment is shown as text, and images are not shown, so a comment cannot load anything from other sites.

## Mention people

Type `@` and a list of the project's people appears, as in the other text fields (see [Mention people](requirements.md#mention-people)). Use the arrow keys and <kbd>Enter</kbd> or <kbd>Tab</kbd> to insert someone. Mentioned members get a notification, and an e-mail if they chose to get notifications by e-mail (see [Notifications](reviews-and-notifications.md#notifications)). Nobody hears about a project they cannot see.

Who else is told:

- the people who wrote in a thread, about new replies and when the thread is resolved;
- the owner of the item (a requirement's owner, a defect's assignee, a risk's owner, the reviewer and the requester of a review request), about a new thread.

Everybody is told once per comment, and never about their own comments.

## Edit, remove, react

- **Edit** (pencil) changes your own comment; it is then marked *(edited)*. People you mention in the new text are notified.
- **Delete** (bin) removes your own comment. It stays in its thread as *This comment was removed*, so the replies still make sense; its text is gone. A **moderator** (someone who may change the project's settings, `project.update`, or manage members) can remove anybody's comment; the audit log records it as moderated.
- **Add reaction** (smiley) adds a thumbs up, thumbs down, heart, hooray, *looking* or *done*. Select a reaction under a comment to add or take back yours.

## Resolve and reopen

When a question is answered, select **Resolve** below the thread. Resolved threads are folded away: turn on **Show resolved** to see them, marked *Resolved* (point at the mark to see who resolved it and when). **Reopen** opens a thread again. Anybody who can write in the discussion can resolve and reopen threads.

## The Discussions page

The project's **Discussions** page lists the threads of all its items, the latest activity first: the item (with its kind), who started the thread and how, the number of replies and who wrote last. Select an item to open it with its discussion.

Narrow the list with:

- **Open**, **Resolved** or **All** threads;
- **All threads**, **I took part** (threads you wrote in) or **Mention me**;
- the kind of item (requirements, tests, defects, risks, test runs, review requests);
- **Search the comments** for a word in any comment of the thread.

The filters are part of the page address, so you can bookmark *Open threads that mention me*.

## Find items with open discussions

In the requirement and test lists, filter with:

- `has:open-discussion`: items with at least one unresolved thread (`open_discussions>2` for more than two);
- `mentions:me`: items that mention you, in their texts or in their discussion; `mention:anna.berg` for someone else.

See [Search and filters](search-and-filters.md).

## Permissions

| To | You need |
|---|---|
| read a discussion | the read permission of the item (for example `requirement.read`, `run.read`) |
| comment, reply, edit your comments, resolve, reopen and react | `comment.create` (every role except **Viewer** by default) |
| remove your own comments | `comment.create` |
| remove anybody's comment | `project.update` in the project, or `member.manage` |

`comment.create` is a project permission: a project role can grant or withhold it in one project. Every comment, edit, removal, resolution and reopening is recorded in the audit log (`comment.create`, `comment.update`, `comment.delete`, `comment.resolve`, `comment.reopen`).

## Discussions elsewhere

- **Generated documents**: tick **Discussions of the items** under *Appendices* when you generate a requirements specification or a test plan to add an appendix with the threads of its requirements or tests (see [Create a requirements specification or a test plan](documents.md#create-a-requirements-specification-or-a-test-plan)).
- **Saved projects, backups and the trash**: discussions are part of a saved project and of a backup, and a deleted requirement or test comes back from the trash with its discussion. Deleting a defect, a risk or a run deletes its discussion.
- **Webhooks**: the events `comment.created`, `comment.mentioned` and `comment.resolved` send new comments, mentions and resolved threads to other systems.
