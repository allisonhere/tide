package db

import (
	"strings"
	"time"
)

// SearchResult is an article matched by a search, annotated with the feed it
// came from and a snippet of the matching text.
type SearchResult struct {
	Article
	FeedTitle string
	// Snippet is the matching excerpt. Matched terms are wrapped in
	// SnippetOpen/SnippetClose so the UI can style them without re-running the
	// match. Empty when the LIKE fallback is in use.
	Snippet string
	// Rank is the bm25 score. SQLite returns negative values where a smaller
	// (more negative) number is a better match, so results sort ascending.
	Rank float64
}

// Snippet delimiters. ASCII STX/ETX are used instead of visible characters like
// brackets so literal brackets in article text can never be mistaken for
// highlight markers.
const (
	SnippetOpen  = "\x02"
	SnippetClose = "\x03"
)

const defaultSearchLimit = 200

// SearchArticles runs a full-text search across every article in every local
// feed, ranked by relevance.
//
// Remote (Google Reader) articles are not covered: they are never persisted, so
// there is nothing to index. Callers should fall back to filtering the loaded
// slice for remote feeds.
func (db *DB) SearchArticles(query string, unreadOnly bool, limit int) ([]SearchResult, error) {
	match := buildMatchQuery(query)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if !db.ftsAvailable {
		return db.searchArticlesLike(query, unreadOnly, limit)
	}

	unread := 0
	if unreadOnly {
		unread = 1
	}
	rows, err := db.Query(`
		SELECT a.id, a.feed_id, a.guid, a.title, a.link, a.content, a.summary,
		       a.published_at, a.read,
		       COALESCE(NULLIF(f.custom_title, ''), f.title) AS feed_title,
		       snippet(articles_fts, -1, ?, ?, '…', 12) AS snip,
		       bm25(articles_fts, 10.0, 1.0, 5.0) AS rank
		FROM articles_fts
		JOIN articles a ON a.id = articles_fts.rowid
		JOIN feeds    f ON f.id = a.feed_id
		WHERE articles_fts MATCH ?
		  AND (? = 0 OR a.read = 0)
		ORDER BY rank
		LIMIT ?
	`, SnippetOpen, SnippetClose, match, unread, limit)
	if err != nil {
		// A malformed MATCH expression should degrade to a substring scan
		// rather than surface a SQL error on every keystroke.
		return db.searchArticlesLike(query, unreadOnly, limit)
	}
	defer rows.Close()
	return scanSearchResults(rows, true)
}

// searchArticlesLike is the fallback for SQLite builds without FTS5. It is a
// full scan with multi-term AND semantics: correct, but unranked and without
// snippets.
func (db *DB) searchArticlesLike(query string, unreadOnly bool, limit int) ([]SearchResult, error) {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil, nil
	}

	var clauses []string
	args := make([]any, 0, len(terms)*3+2)
	for _, t := range terms {
		clauses = append(clauses, `(LOWER(a.title) LIKE ? ESCAPE '\' OR LOWER(a.content) LIKE ? ESCAPE '\' OR LOWER(a.summary) LIKE ? ESCAPE '\')`)
		pat := "%" + escapeLike(t) + "%"
		args = append(args, pat, pat, pat)
	}
	where := strings.Join(clauses, " AND ")
	if unreadOnly {
		where += " AND a.read = 0"
	}
	args = append(args, limit)

	rows, err := db.Query(`
		SELECT a.id, a.feed_id, a.guid, a.title, a.link, a.content, a.summary,
		       a.published_at, a.read,
		       COALESCE(NULLIF(f.custom_title, ''), f.title) AS feed_title
		FROM articles a
		JOIN feeds f ON f.id = a.feed_id
		WHERE `+where+`
		ORDER BY a.published_at DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearchResults(rows, false)
}

func scanSearchResults(rows scannerRows, withRank bool) ([]SearchResult, error) {
	var out []SearchResult
	for rows.Next() {
		var (
			r       SearchResult
			ts      int64
			read    int
			snippet string
			rank    float64
		)
		dest := []any{
			&r.ID, &r.FeedID, &r.GUID, &r.Title, &r.Link, &r.Content, &r.Summary,
			&ts, &read, &r.FeedTitle,
		}
		if withRank {
			dest = append(dest, &snippet, &rank)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		r.PublishedAt = time.Unix(ts, 0)
		r.Read = read != 0
		r.Snippet = snippet
		r.Rank = rank
		out = append(out, r)
	}
	return out, rows.Err()
}

type scannerRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// buildMatchQuery converts free-form user input into a valid FTS5 MATCH
// expression.
//
// This is not cosmetic. Raw input is not valid FTS5 syntax: bare `-`, `*`, `^`,
// `:` and unbalanced `"` are all operators or syntax errors, so typing an
// ordinary query like "c++ 20" or a lone "-" would otherwise error on every
// keystroke. Every token is quoted (which makes its contents literal), and the
// final token gets a `*` suffix so results narrow as the user types.
//
//	rust async  ->  "rust" "async"*
//	c++ 20      ->  "c++" "20"*
func buildMatchQuery(raw string) string {
	fields := strings.Fields(raw)
	tokens := make([]string, 0, len(fields))
	for _, f := range fields {
		// Inside a quoted FTS5 string a literal " is escaped by doubling it.
		f = strings.ReplaceAll(f, `"`, `""`)
		if strings.TrimSpace(f) == "" {
			continue
		}
		tokens = append(tokens, `"`+f+`"`)
	}
	if len(tokens) == 0 {
		return ""
	}
	// Prefix-match the last token for search-as-you-type.
	tokens[len(tokens)-1] += "*"
	return strings.Join(tokens, " ")
}

// escapeLike neutralises LIKE wildcards in user input for the fallback path.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
