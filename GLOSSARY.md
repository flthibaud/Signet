# Signet

A self-hosted read-it-later app where saved pages and RSS/Atom feeds land in one per-user library.

## Language

### Library

**Article**:
A web page's extracted content, stored once for the whole instance however many users have it.
_Avoid_: post, entry, item (an item is the feed's raw record, before it becomes an article)

**Link**:
One user's copy of an article in their library, carrying their read, favorite, archive and progress state.
_Avoid_: bookmark, saved article

### Feeds

**Feed**:
An RSS or Atom source, stored once per URL and shared by every user subscribed to it.
_Avoid_: channel, source

**Item**:
One entry in a feed document, as the feed publishes it; it becomes an article once imported.
_Avoid_: entry, post

**Subscription**:
A user's relationship to a feed, optionally filed in one of their folders.
_Avoid_: follow

**Feed sync**:
One pass over a single feed: fetch it, turn its new items into articles, give every subscriber a link to each, and record the outcome on the feed.
_Avoid_: import, refresh, poll, fetch (fetch is only the HTTP step)

**Claim**:
A sync's temporary exclusive hold on a feed, so no two syncs run on the same feed at once.
_Avoid_: lock, lease

**Due feed**:
A feed whose next sync is owed: its refresh interval has elapsed, or it was just marked due for a new subscriber.
_Avoid_: stale feed, pending feed

**Deactivated feed**:
A feed that failed too many syncs in a row and is now retried only rarely, until a sync succeeds or someone subscribes to it again.
_Avoid_: dead feed, disabled feed
