package data

import (
	"context"
	"errors"
	"testing"
)

// TestFolderGetOrCreate pins the property the OPML import relies on: calling it
// repeatedly for the same name yields the same folder, and two users may hold
// folders of the same name.
func TestFolderGetOrCreate(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := FolderModel{DB: db}
	alice := seedUser(t, db)
	bob := seedUser(t, db)

	first, err := model.GetOrCreate(ctx, alice, "Tech")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if first.ID == 0 || first.Name != "Tech" || first.UserID != alice {
		t.Fatalf("unexpected folder: %+v", first)
	}
	if first.CreatedAt.IsZero() {
		t.Error("created_at was not returned")
	}

	again, err := model.GetOrCreate(ctx, alice, "Tech")
	if err != nil {
		t.Fatalf("GetOrCreate (second call): %v", err)
	}
	if again.ID != first.ID {
		t.Errorf("got a new folder %d on the second call, want %d", again.ID, first.ID)
	}

	other, err := model.GetOrCreate(ctx, bob, "Tech")
	if err != nil {
		t.Fatalf("GetOrCreate for another user: %v", err)
	}
	if other.ID == first.ID {
		t.Error("two users ended up sharing one folder")
	}
}

func TestFolderGetAllForUser(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := FolderModel{DB: db}
	userID := seedUser(t, db)
	other := seedUser(t, db)

	seedFolder(t, db, userID, "Tech")
	seedFolder(t, db, userID, "Dev")
	seedFolder(t, db, other, "Not mine")

	folders, err := model.GetAllForUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetAllForUser: %v", err)
	}

	if len(folders) != 2 {
		t.Fatalf("got %d folders, want 2", len(folders))
	}
	// Alphabetical, and scoped to the user.
	if folders[0].Name != "Dev" || folders[1].Name != "Tech" {
		t.Errorf("unexpected folders: %s, %s", folders[0].Name, folders[1].Name)
	}
}

// TestSubscriptionsGetAllForUserFolder covers the LEFT JOIN added for folders:
// a filed subscription carries its folder, an unfiled one carries nil rather
// than an empty struct.
func TestSubscriptionsGetAllForUserFolder(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := SubscriptionModel{DB: db}
	userID := seedUser(t, db)

	folderID := seedFolder(t, db, userID, "Tech")
	filedFeed := seedFeed(t, db, "Korben", "")
	unfiledFeed := seedFeed(t, db, "Dan Luu", "")

	seedSubscription(t, db, userID, filedFeed, &folderID)
	seedSubscription(t, db, userID, unfiledFeed, nil)

	subs, err := model.GetAllForUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetAllForUser: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("got %d subscriptions, want 2", len(subs))
	}

	byFeed := map[int64]*Subscription{}
	for _, s := range subs {
		byFeed[s.FeedID] = s
	}

	filed := byFeed[filedFeed]
	if filed.Folder == nil {
		t.Fatal("filed subscription came back without its folder")
	}
	if filed.Folder.Name != "Tech" || filed.Folder.ID != folderID {
		t.Errorf("unexpected folder: %+v", filed.Folder)
	}
	if filed.FolderID == nil || *filed.FolderID != folderID {
		t.Errorf("FolderID = %v, want %d", filed.FolderID, folderID)
	}

	if unfiled := byFeed[unfiledFeed]; unfiled.Folder != nil || unfiled.FolderID != nil {
		t.Errorf("unfiled subscription should carry no folder, got %+v", unfiled.Folder)
	}
}

// TestSubscriptionInsertFolder makes sure Insert actually persists folder_id —
// a column missing from the INSERT would be invisible to the compiler.
func TestSubscriptionInsertFolder(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := SubscriptionModel{DB: db}
	userID := seedUser(t, db)
	folderID := seedFolder(t, db, userID, "Dev")
	feedID := seedFeed(t, db, "The Go Blog", "")

	sub := &Subscription{UserID: userID, FeedID: feedID, FolderID: &folderID}
	if err := model.Insert(ctx, sub); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	var stored *int64
	err := db.QueryRowContext(ctx, `SELECT folder_id FROM subscriptions WHERE id = $1`, sub.ID).Scan(&stored)
	if err != nil {
		t.Fatalf("reading back the subscription: %v", err)
	}
	if stored == nil || *stored != folderID {
		t.Errorf("folder_id = %v, want %d", stored, folderID)
	}
}

// TestListForUserFolderFilter covers the folder filter, including the part that
// matters for privacy: another user's folder id must not select anything.
func TestListForUserFolderFilter(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := LinkModel{DB: db}
	userID := seedUser(t, db)
	other := seedUser(t, db)

	folderID := seedFolder(t, db, userID, "Tech")
	otherFolderID := seedFolder(t, db, other, "Theirs")

	inFolder := seedFeed(t, db, "Korben", "")
	outOfFolder := seedFeed(t, db, "Dan Luu", "")
	seedSubscription(t, db, userID, inFolder, &folderID)
	seedSubscription(t, db, userID, outOfFolder, nil)

	seedLink(t, db, userID, seedArticle(t, db, articleFixture{}), linkFixture{feedID: &inFolder})
	seedLink(t, db, userID, seedArticle(t, db, articleFixture{}), linkFixture{feedID: &outOfFolder})

	links, _, err := model.ListForUser(ctx, userID, LinkFilters{FolderID: &folderID}, 20, 0)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("got %d links, want only the one in the folder", len(links))
	}
	if links[0].FeedID == nil || *links[0].FeedID != inFolder {
		t.Errorf("got a link from feed %v, want %d", links[0].FeedID, inFolder)
	}

	// Someone else's folder id selects nothing, even though the folder exists.
	links, _, err = model.ListForUser(ctx, userID, LinkFilters{FolderID: &otherFolderID}, 20, 0)
	if err != nil {
		t.Fatalf("ListForUser with another user's folder: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("got %d links for another user's folder, want none", len(links))
	}
}

func TestFolderGet(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := FolderModel{DB: db}
	userID := seedUser(t, db)
	other := seedUser(t, db)

	folderID := seedFolder(t, db, userID, "Tech")

	folder, err := model.Get(ctx, userID, folderID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if folder.Name != "Tech" || folder.UserID != userID || folder.CreatedAt.IsZero() {
		t.Errorf("unexpected folder: %+v", folder)
	}

	// The folder exists, but not for this user.
	if _, err := model.Get(ctx, other, folderID); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("Get for another user: got %v, want ErrRecordNotFound", err)
	}
}

func TestFolderInsert(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := FolderModel{DB: db}
	userID := seedUser(t, db)
	other := seedUser(t, db)

	folder := &Folder{UserID: userID, Name: "Tech"}
	if err := model.Insert(ctx, folder); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if folder.ID == 0 || folder.CreatedAt.IsZero() {
		t.Errorf("Insert did not fill in the generated columns: %+v", folder)
	}

	dup := &Folder{UserID: userID, Name: "Tech"}
	if err := model.Insert(ctx, dup); !errors.Is(err, ErrDuplicateFolder) {
		t.Errorf("inserting a duplicate: got %v, want ErrDuplicateFolder", err)
	}

	// Uniqueness is per user, not global.
	sameName := &Folder{UserID: other, Name: "Tech"}
	if err := model.Insert(ctx, sameName); err != nil {
		t.Errorf("another user could not reuse the name: %v", err)
	}
}

func TestFolderUpdate(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := FolderModel{DB: db}
	userID := seedUser(t, db)
	other := seedUser(t, db)

	folderID := seedFolder(t, db, userID, "Tech")
	seedFolder(t, db, userID, "Dev")

	if err := model.Update(ctx, userID, folderID, "Technology"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	folder, err := model.Get(ctx, userID, folderID)
	if err != nil {
		t.Fatalf("Get after Update: %v", err)
	}
	if folder.Name != "Technology" {
		t.Errorf("name = %q, want %q", folder.Name, "Technology")
	}

	if err := model.Update(ctx, userID, folderID, "Dev"); !errors.Is(err, ErrDuplicateFolder) {
		t.Errorf("renaming onto an existing name: got %v, want ErrDuplicateFolder", err)
	}

	if err := model.Update(ctx, other, folderID, "Mine now"); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("renaming another user's folder: got %v, want ErrRecordNotFound", err)
	}
}

// TestFolderDelete pins the behaviour the ON DELETE SET NULL was chosen for:
// deleting a folder unfiles its subscriptions rather than unsubscribing.
func TestFolderDelete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := FolderModel{DB: db}
	userID := seedUser(t, db)
	other := seedUser(t, db)

	folderID := seedFolder(t, db, userID, "Tech")
	feedID := seedFeed(t, db, "Korben", "")
	subID := seedSubscription(t, db, userID, feedID, &folderID)

	if err := model.Delete(ctx, other, folderID); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("deleting another user's folder: got %v, want ErrRecordNotFound", err)
	}

	if err := model.Delete(ctx, userID, folderID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var storedFolder *int64
	err := db.QueryRowContext(ctx, `SELECT folder_id FROM subscriptions WHERE id = $1`, subID).Scan(&storedFolder)
	if err != nil {
		t.Fatalf("the subscription did not survive its folder: %v", err)
	}
	if storedFolder != nil {
		t.Errorf("folder_id = %d, want NULL", *storedFolder)
	}

	if err := model.Delete(ctx, userID, folderID); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("deleting twice: got %v, want ErrRecordNotFound", err)
	}
}

func TestSubscriptionSetFolder(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := SubscriptionModel{DB: db}
	userID := seedUser(t, db)
	other := seedUser(t, db)

	folderID := seedFolder(t, db, userID, "Tech")
	feedID := seedFeed(t, db, "Korben", "")
	subID := seedSubscription(t, db, userID, feedID, nil)

	readBack := func() *int64 {
		t.Helper()
		var stored *int64
		if err := db.QueryRowContext(ctx, `SELECT folder_id FROM subscriptions WHERE id = $1`, subID).Scan(&stored); err != nil {
			t.Fatalf("reading back the subscription: %v", err)
		}
		return stored
	}

	if err := model.SetFolder(ctx, userID, subID, &folderID); err != nil {
		t.Fatalf("SetFolder: %v", err)
	}
	if stored := readBack(); stored == nil || *stored != folderID {
		t.Errorf("folder_id = %v, want %d", stored, folderID)
	}

	if err := model.SetFolder(ctx, userID, subID, nil); err != nil {
		t.Fatalf("SetFolder(nil): %v", err)
	}
	if stored := readBack(); stored != nil {
		t.Errorf("folder_id = %d, want NULL", *stored)
	}

	if err := model.SetFolder(ctx, other, subID, &folderID); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("filing another user's subscription: got %v, want ErrRecordNotFound", err)
	}
}
