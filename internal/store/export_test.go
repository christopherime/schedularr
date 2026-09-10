package store

// ExecForTest runs a raw statement against the store's database.
//
// It lives in an _test.go file, so it is compiled only for tests and never
// ships in the binary -- CLAUDE.md rule 1 forbids production code whose
// only caller is a test. It exists so a test can put the database into a
// state the store's own API deliberately cannot produce: corrupt JSON in a
// snapshot row, to prove that a failure partway through RemoveShow leaves
// nothing removed.
func (s *Store) ExecForTest(query string, args ...any) error {
	_, err := s.db.Exec(query, args...)
	return err
}
