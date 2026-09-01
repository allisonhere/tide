package image

// Req identifies an in-flight image fetch so a result that arrives after the
// user has moved on can be discarded instead of drawn over the wrong article.
type Req struct {
	ArticleID int64
	Gen       uint64
}

// Fresh reports whether this request still matches the model's current article
// and generation counter. A fetch result whose Req is not Fresh must be dropped.
func (r Req) Fresh(curArticleID int64, curGen uint64) bool {
	return r.ArticleID == curArticleID && r.Gen == curGen
}
