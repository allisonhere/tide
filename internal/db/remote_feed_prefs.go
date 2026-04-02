package db

func (db *DB) ListRemoteFeedFolders() (map[int64]int64, error) {
	rows, err := db.Query(`
		SELECT remote_feed_id, folder_id
		FROM remote_feed_prefs
		WHERE folder_id IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	assignments := map[int64]int64{}
	for rows.Next() {
		var remoteFeedID int64
		var folderID int64
		if err := rows.Scan(&remoteFeedID, &folderID); err != nil {
			return nil, err
		}
		assignments[remoteFeedID] = folderID
	}
	return assignments, rows.Err()
}

func (db *DB) SetRemoteFeedFolder(remoteFeedID, folderID int64) error {
	if folderID == 0 {
		_, err := db.Exec(`DELETE FROM remote_feed_prefs WHERE remote_feed_id = ?`, remoteFeedID)
		return err
	}

	_, err := db.Exec(`
		INSERT INTO remote_feed_prefs (remote_feed_id, folder_id)
		VALUES (?, ?)
		ON CONFLICT(remote_feed_id) DO UPDATE SET folder_id = excluded.folder_id
	`, remoteFeedID, folderID)
	return err
}
